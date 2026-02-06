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
