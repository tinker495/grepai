package rpg

import (
	"context"
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

	rpgStore := &GOBRPGStore{indexPath: "/dev/null", graph: g}
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
