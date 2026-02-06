package rpg

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

func TestLinkChunksForFile_NoAccumulation(t *testing.T) {
	// Setup: create a graph with a file node and symbol node
	g := NewGraph()
	fileNode := &Node{ID: "file:main.go", Kind: KindFile, Path: "main.go"}
	g.AddNode(fileNode)

	symNode := &Node{
		ID:         "sym:main.go:Foo",
		Kind:       KindSymbol,
		Path:       "main.go",
		SymbolName: "Foo",
		StartLine:  1,
		EndLine:    20,
	}
	g.AddNode(symNode)
	g.AddEdge(&Edge{From: fileNode.ID, To: symNode.ID, Type: EdgeContains, Weight: 1.0})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGIndexer(rpgStore, extractor, "/tmp", RPGIndexerConfig{DriftThreshold: 0.35})

	chunks := []store.Chunk{
		{ID: "chunk-1", FilePath: "main.go", StartLine: 1, EndLine: 10},
		{ID: "chunk-2", FilePath: "main.go", StartLine: 11, EndLine: 20},
	}

	// First call
	if err := indexer.LinkChunksForFile(context.Background(), "main.go", chunks); err != nil {
		t.Fatalf("first LinkChunksForFile failed: %v", err)
	}

	edgeCount1 := countEdgesByType(g, EdgeMapsToChunk)
	chunkCount1 := countNodesByKind(g, KindChunk)

	// Second call (simulates file modification)
	if err := indexer.LinkChunksForFile(context.Background(), "main.go", chunks); err != nil {
		t.Fatalf("second LinkChunksForFile failed: %v", err)
	}

	edgeCount2 := countEdgesByType(g, EdgeMapsToChunk)
	chunkCount2 := countNodesByKind(g, KindChunk)

	// Edge and chunk counts should NOT grow after repeated calls
	if edgeCount2 != edgeCount1 {
		t.Errorf("EdgeMapsToChunk accumulated: first=%d, second=%d", edgeCount1, edgeCount2)
	}
	if chunkCount2 != chunkCount1 {
		t.Errorf("Chunk nodes accumulated: first=%d, second=%d", chunkCount1, chunkCount2)
	}

	// Verify correct counts
	if chunkCount1 != 2 {
		t.Errorf("Expected 2 chunk nodes, got %d", chunkCount1)
	}
	if edgeCount1 != 2 {
		t.Errorf("Expected 2 EdgeMapsToChunk edges, got %d", edgeCount1)
	}
}

func TestNormalizeEndLine(t *testing.T) {
	tests := []struct {
		name      string
		startLine int
		endLine   int
		want      int
	}{
		{"zero endline falls back to startline", 42, 0, 42},
		{"negative endline falls back to startline", 10, -1, 10},
		{"endline before startline falls back", 50, 30, 50},
		{"valid endline preserved", 10, 25, 25},
		{"same start and end preserved", 5, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEndLine(tt.startLine, tt.endLine)
			if got != tt.want {
				t.Errorf("normalizeEndLine(%d, %d) = %d, want %d", tt.startLine, tt.endLine, got, tt.want)
			}
		})
	}
}

func TestLinkChunksForFile_SymbolEndLineZero(t *testing.T) {
	// Regression: regex extractor sets EndLine=0; chunks must still link
	g := NewGraph()
	fileNode := &Node{ID: "file:main.go", Kind: KindFile, Path: "main.go"}
	g.AddNode(fileNode)

	// Symbol with EndLine=0 (as produced by regex extractor)
	symNode := &Node{
		ID:         "sym:main.go:Foo",
		Kind:       KindSymbol,
		Path:       "main.go",
		SymbolName: "Foo",
		StartLine:  10,
		EndLine:    10, // after normalizeEndLine(10, 0) => 10
	}
	g.AddNode(symNode)
	g.AddEdge(&Edge{From: fileNode.ID, To: symNode.ID, Type: EdgeContains, Weight: 1.0})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGIndexer(rpgStore, extractor, "/tmp", RPGIndexerConfig{DriftThreshold: 0.35})

	// Chunk that spans lines 1-20 (covers the symbol at line 10)
	chunks := []store.Chunk{
		{ID: "chunk-1", FilePath: "main.go", StartLine: 1, EndLine: 20},
	}

	if err := indexer.LinkChunksForFile(context.Background(), "main.go", chunks); err != nil {
		t.Fatalf("LinkChunksForFile failed: %v", err)
	}

	edgeCount := countEdgesByType(g, EdgeMapsToChunk)
	if edgeCount != 1 {
		t.Errorf("Expected 1 EdgeMapsToChunk edge for symbol at line 10 within chunk 1-20, got %d", edgeCount)
	}
}

func countEdgesByType(g *Graph, edgeType EdgeType) int {
	count := 0
	for _, e := range g.Edges {
		if e.Type == edgeType {
			count++
		}
	}
	return count
}

func countNodesByKind(g *Graph, kind NodeKind) int {
	count := 0
	for _, n := range g.Nodes {
		if n.Kind == kind {
			count++
		}
	}
	return count
}

func TestFeatureSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "handle-request", "handle-request", 1.0},
		// handle-request: {handle, request}, handle-response: {handle, response}
		// intersection=1, union=3 -> 1/3 ≈ 0.333
		{"partial overlap", "handle-request", "handle-response", 0.333},
		{"different", "handle-request", "parse-config", 0.0},
		{"empty first", "", "handle-request", 0.0},
		{"empty second", "handle-request", "", 0.0},
		// handle-request@server: {handle, request, server}
		// handle-request@client: {handle, request, client}
		// intersection=2, union=4 -> 0.5
		{"above threshold", "handle-request@server", "handle-request@client", 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := featureSimilarity(tt.a, tt.b)
			if got < tt.want-0.01 || got > tt.want+0.01 {
				t.Errorf("featureSimilarity(%q, %q) = %f, want ~%f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFirstWord(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"handle-request", "handle"},
		{"parse-config@server", "parse"},
		{"validate", "validate"},
		{"get_user", "get"},
		{"run/task", "run"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := firstWord(tt.input)
			if got != tt.want {
				t.Errorf("firstWord(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWireFeatureSimilarity(t *testing.T) {
	g := NewGraph()

	// Two symbols in different files with similar features (Jaccard >= 0.5)
	symA := &Node{ID: "sym:a.go:HandleReq", Kind: KindSymbol, Path: "a.go", Feature: "handle-request@server", SymbolName: "HandleReq"}
	symB := &Node{ID: "sym:b.go:HandleReq", Kind: KindSymbol, Path: "b.go", Feature: "handle-request@client", SymbolName: "HandleReq"}
	// Different feature verb
	symC := &Node{ID: "sym:c.go:ParseConfig", Kind: KindSymbol, Path: "c.go", Feature: "parse-config", SymbolName: "ParseConfig"}

	g.AddNode(symA)
	g.AddNode(symB)
	g.AddNode(symC)

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGIndexer(rpgStore, extractor, "/tmp", RPGIndexerConfig{DriftThreshold: 0.35})

	indexer.wireFeatureSimilarity(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 1 {
		t.Errorf("Expected 1 EdgeSemanticSim edge between similar symbols, got %d", simCount)
	}
}

func TestWireFeatureSimilarity_SameFileSkipped(t *testing.T) {
	g := NewGraph()

	// Two symbols in the SAME file - should not get an edge
	symA := &Node{ID: "sym:a.go:HandleReq", Kind: KindSymbol, Path: "a.go", Feature: "handle-request@server", SymbolName: "HandleReq"}
	symB := &Node{ID: "sym:a.go:HandleRes", Kind: KindSymbol, Path: "a.go", Feature: "handle-request@client", SymbolName: "HandleRes"}

	g.AddNode(symA)
	g.AddNode(symB)

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGIndexer(rpgStore, extractor, "/tmp", RPGIndexerConfig{DriftThreshold: 0.35})

	indexer.wireFeatureSimilarity(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 0 {
		t.Errorf("Expected 0 EdgeSemanticSim edges for same-file symbols, got %d", simCount)
	}
}

func TestWireCoCallerAffinity(t *testing.T) {
	g := NewGraph()

	// Create caller and callee symbols
	caller1 := &Node{ID: "sym:main.go:Main", Kind: KindSymbol, Path: "main.go", Feature: "run-main", SymbolName: "Main"}
	caller2 := &Node{ID: "sym:app.go:Start", Kind: KindSymbol, Path: "app.go", Feature: "start-app", SymbolName: "Start"}
	calleeA := &Node{ID: "sym:a.go:FuncA", Kind: KindSymbol, Path: "a.go", Feature: "handle-a", SymbolName: "FuncA"}
	calleeB := &Node{ID: "sym:b.go:FuncB", Kind: KindSymbol, Path: "b.go", Feature: "handle-b", SymbolName: "FuncB"}

	g.AddNode(caller1)
	g.AddNode(caller2)
	g.AddNode(calleeA)
	g.AddNode(calleeB)

	// Both callers invoke both callees -> co-occurrence count = 2
	g.AddEdge(&Edge{From: caller1.ID, To: calleeA.ID, Type: EdgeInvokes, Weight: 1.0})
	g.AddEdge(&Edge{From: caller1.ID, To: calleeB.ID, Type: EdgeInvokes, Weight: 1.0})
	g.AddEdge(&Edge{From: caller2.ID, To: calleeA.ID, Type: EdgeInvokes, Weight: 1.0})
	g.AddEdge(&Edge{From: caller2.ID, To: calleeB.ID, Type: EdgeInvokes, Weight: 1.0})

	rpgStore := &GOBRPGStore{indexPath: filepath.Join(t.TempDir(), "rpg.gob"), graph: g}
	extractor := NewLocalExtractor()
	indexer := NewRPGIndexer(rpgStore, extractor, "/tmp", RPGIndexerConfig{DriftThreshold: 0.35})

	indexer.wireCoCallerAffinity(g)

	simCount := countEdgesByType(g, EdgeSemanticSim)
	if simCount != 1 {
		t.Errorf("Expected 1 EdgeSemanticSim edge for co-caller affinity, got %d", simCount)
	}
}

func TestBuildFull_EdgeImports(t *testing.T) {
	// Test that EdgeImports edges are created for cross-file invocations
	g := NewGraph()

	// Create two file nodes
	fileA := &Node{ID: "file:a.go", Kind: KindFile, Path: "a.go"}
	fileB := &Node{ID: "file:b.go", Kind: KindFile, Path: "b.go"}
	g.AddNode(fileA)
	g.AddNode(fileB)

	// Create symbol nodes in each file
	symA := &Node{
		ID:         "sym:a.go:FuncA",
		Kind:       KindSymbol,
		Path:       "a.go",
		SymbolName: "FuncA",
		StartLine:  1,
		EndLine:    10,
	}
	symB := &Node{
		ID:         "sym:b.go:FuncB",
		Kind:       KindSymbol,
		Path:       "b.go",
		SymbolName: "FuncB",
		StartLine:  1,
		EndLine:    10,
	}
	g.AddNode(symA)
	g.AddNode(symB)

	// Add EdgeContains edges
	g.AddEdge(&Edge{From: fileA.ID, To: symA.ID, Type: EdgeContains, Weight: 1.0})
	g.AddEdge(&Edge{From: fileB.ID, To: symB.ID, Type: EdgeContains, Weight: 1.0})

	// Add cross-file invocation edge (FuncA calls FuncB)
	g.AddEdge(&Edge{From: symA.ID, To: symB.ID, Type: EdgeInvokes, Weight: 1.0})

	// Now simulate Step 3 from BuildFull: generate EdgeImports
	importsSeen := make(map[string]bool)
	for _, e := range g.Edges {
		if e.Type != EdgeInvokes {
			continue
		}
		callerNode := g.GetNode(e.From)
		calleeNode := g.GetNode(e.To)
		if callerNode == nil || calleeNode == nil {
			continue
		}
		if callerNode.Path == "" || calleeNode.Path == "" || callerNode.Path == calleeNode.Path {
			continue
		}
		key := callerNode.Path + "->" + calleeNode.Path
		if importsSeen[key] {
			continue
		}
		importsSeen[key] = true

		fromFileID := MakeNodeID(KindFile, callerNode.Path)
		toFileID := MakeNodeID(KindFile, calleeNode.Path)
		if g.GetNode(fromFileID) != nil && g.GetNode(toFileID) != nil {
			g.AddEdge(&Edge{
				From:      fromFileID,
				To:        toFileID,
				Type:      EdgeImports,
				Weight:    1.0,
			})
		}
	}

	// Verify EdgeImports edge was created
	importsCount := countEdgesByType(g, EdgeImports)
	if importsCount != 1 {
		t.Errorf("Expected 1 EdgeImports edge, got %d", importsCount)
	}

	// Verify the edge is from fileA to fileB
	var foundImport bool
	for _, e := range g.Edges {
		if e.Type == EdgeImports && e.From == fileA.ID && e.To == fileB.ID {
			foundImport = true
			break
		}
	}
	if !foundImport {
		t.Error("Expected EdgeImports edge from a.go to b.go, but not found")
	}
}
