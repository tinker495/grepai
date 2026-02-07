package rpg

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestEdgeCost(t *testing.T) {
	tests := []struct {
		name     string
		weight   float64
		expected float64
	}{
		{"structural edge weight=1.0", 1.0, 1.0},
		{"strong similarity weight=0.8", 0.8, 1.25},
		{"weak similarity weight=0.5", 0.5, 2.0},
		{"very weak weight=0.25", 0.25, 4.0},
		{"zero weight", 0.0, 10.0},
		{"negative weight", -0.5, 10.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := edgeCost(tt.weight)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("edgeCost(%v) = %v, want %v", tt.weight, got, tt.expected)
			}
		})
	}
}

// helper to build a simple test graph
func makeTestGraph(nodes []struct{ id, kind string }, edges []struct {
	from, to, edgeType string
	weight             float64
}) *Graph {
	g := NewGraph()
	now := time.Now()
	for _, n := range nodes {
		g.AddNode(&Node{
			ID:        n.id,
			Kind:      NodeKind(n.kind),
			UpdatedAt: now,
		})
	}
	for _, e := range edges {
		g.AddEdge(&Edge{
			From:      e.from,
			To:        e.to,
			Type:      EdgeType(e.edgeType),
			Weight:    e.weight,
			UpdatedAt: now,
		})
	}
	return g
}

func TestShortestPath_LinearPath(t *testing.T) {
	// A -> B -> C, all weight=1.0
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
			{"B", "C", "invokes", 1.0},
		},
	)

	nodeIDs, edges, cost := g.ShortestPath("A", "C", nil)
	if len(nodeIDs) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %v", len(nodeIDs), nodeIDs)
	}
	if nodeIDs[0] != "A" || nodeIDs[1] != "B" || nodeIDs[2] != "C" {
		t.Errorf("unexpected path: %v", nodeIDs)
	}
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
	if math.Abs(cost-2.0) > 1e-9 {
		t.Errorf("expected cost 2.0, got %v", cost)
	}
}

func TestShortestPath_SameNode(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}},
		nil,
	)

	nodeIDs, edges, cost := g.ShortestPath("A", "A", nil)
	if len(nodeIDs) != 1 || nodeIDs[0] != "A" {
		t.Errorf("expected [A], got %v", nodeIDs)
	}
	if edges != nil {
		t.Errorf("expected nil edges, got %v", edges)
	}
	if cost != 0 {
		t.Errorf("expected cost 0, got %v", cost)
	}
}

func TestShortestPath_Unreachable(t *testing.T) {
	// A and B are disconnected
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		nil,
	)

	nodeIDs, edges, cost := g.ShortestPath("A", "B", nil)
	if nodeIDs != nil {
		t.Errorf("expected nil path, got %v", nodeIDs)
	}
	if edges != nil {
		t.Errorf("expected nil edges, got %v", edges)
	}
	if cost != -1 {
		t.Errorf("expected cost -1, got %v", cost)
	}
}

func TestShortestPath_WeightedDetour(t *testing.T) {
	// Direct: A->B weight=0.1 (cost=10)
	// Detour: A->C weight=1.0 (cost=1), C->B weight=1.0 (cost=1) → total=2
	// Detour should win
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 0.1},
			{"A", "C", "invokes", 1.0},
			{"C", "B", "invokes", 1.0},
		},
	)

	nodeIDs, _, cost := g.ShortestPath("A", "B", nil)
	if len(nodeIDs) != 3 {
		t.Fatalf("expected detour path of 3 nodes, got %d: %v", len(nodeIDs), nodeIDs)
	}
	if nodeIDs[0] != "A" || nodeIDs[1] != "C" || nodeIDs[2] != "B" {
		t.Errorf("expected path [A C B], got %v", nodeIDs)
	}
	if math.Abs(cost-2.0) > 1e-9 {
		t.Errorf("expected cost 2.0, got %v", cost)
	}
}

func TestShortestPath_Bidirectional(t *testing.T) {
	// Edge A->B only, but traversal is undirected so B->A should work
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
		},
	)

	nodeIDs, edges, cost := g.ShortestPath("B", "A", nil)
	if len(nodeIDs) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %v", len(nodeIDs), nodeIDs)
	}
	if nodeIDs[0] != "B" || nodeIDs[1] != "A" {
		t.Errorf("expected path [B A], got %v", nodeIDs)
	}
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
	if math.Abs(cost-1.0) > 1e-9 {
		t.Errorf("expected cost 1.0, got %v", cost)
	}
}

func TestShortestPath_EdgeTypeFilter(t *testing.T) {
	// A->B via "invokes" (weight=1.0) and A->C->B via "contains" (weight=1.0 each)
	// Filter to "contains" only → must use A->C->B
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
			{"A", "C", "contains", 1.0},
			{"C", "B", "contains", 1.0},
		},
	)

	edgeFilter := map[EdgeType]bool{EdgeContains: true}
	nodeIDs, _, cost := g.ShortestPath("A", "B", edgeFilter)
	if len(nodeIDs) != 3 {
		t.Fatalf("expected 3 nodes with filter, got %d: %v", len(nodeIDs), nodeIDs)
	}
	if nodeIDs[1] != "C" {
		t.Errorf("expected path through C, got %v", nodeIDs)
	}
	if math.Abs(cost-2.0) > 1e-9 {
		t.Errorf("expected cost 2.0, got %v", cost)
	}

	// Filter to "invokes" only → direct A->B
	edgeFilter2 := map[EdgeType]bool{EdgeInvokes: true}
	nodeIDs2, _, cost2 := g.ShortestPath("A", "B", edgeFilter2)
	if len(nodeIDs2) != 2 {
		t.Fatalf("expected 2 nodes with invokes filter, got %d: %v", len(nodeIDs2), nodeIDs2)
	}
	if math.Abs(cost2-1.0) > 1e-9 {
		t.Errorf("expected cost 1.0, got %v", cost2)
	}
}

func TestQueryEngine_ShortestPath(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "file"}, {"C", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "contains", 1.0},
			{"B", "C", "contains", 0.8},
		},
	)

	qe := NewQueryEngine(g)
	ctx := context.Background()

	result, err := qe.ShortestPath(ctx, ShortestPathRequest{
		SourceID: "A",
		TargetID: "C",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Distance < 0 {
		t.Fatal("expected reachable path")
	}
	if result.Source.ID != "A" {
		t.Errorf("expected source A, got %s", result.Source.ID)
	}
	if result.Target.ID != "C" {
		t.Errorf("expected target C, got %s", result.Target.ID)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}
	if len(result.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(result.Edges))
	}
	// cost = 1.0/1.0 + 1.0/0.8 = 1.0 + 1.25 = 2.25
	if math.Abs(result.Distance-2.25) > 1e-9 {
		t.Errorf("expected distance 2.25, got %v", result.Distance)
	}

	// Verify step cumulative costs: A=0.0, B=1.0, C=2.25
	if math.Abs(result.Steps[0].Cost-0.0) > 1e-9 {
		t.Errorf("step[0] cost: expected 0.0, got %v", result.Steps[0].Cost)
	}
	if math.Abs(result.Steps[1].Cost-1.0) > 1e-9 {
		t.Errorf("step[1] cost: expected 1.0, got %v", result.Steps[1].Cost)
	}
	if math.Abs(result.Steps[2].Cost-2.25) > 1e-9 {
		t.Errorf("step[2] cost: expected 2.25, got %v", result.Steps[2].Cost)
	}
	// Source step should have no edge
	if result.Steps[0].Edge != nil {
		t.Errorf("step[0] should have nil edge")
	}
	// Non-source steps should have edges
	if result.Steps[1].Edge == nil {
		t.Errorf("step[1] should have an edge")
	}
	if result.Steps[2].Edge == nil {
		t.Errorf("step[2] should have an edge")
	}
}

func TestShortestPath_NonexistentNode(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}},
		nil,
	)

	// Source doesn't exist
	nodeIDs, _, cost := g.ShortestPath("X", "A", nil)
	if nodeIDs != nil {
		t.Errorf("expected nil path for nonexistent source, got %v", nodeIDs)
	}
	if cost != -1 {
		t.Errorf("expected cost -1, got %v", cost)
	}

	// Target doesn't exist
	nodeIDs2, _, cost2 := g.ShortestPath("A", "Y", nil)
	if nodeIDs2 != nil {
		t.Errorf("expected nil path for nonexistent target, got %v", nodeIDs2)
	}
	if cost2 != -1 {
		t.Errorf("expected cost -1, got %v", cost2)
	}
}

func TestQueryEngine_ShortestPath_Unreachable(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		nil,
	)

	qe := NewQueryEngine(g)
	ctx := context.Background()

	result, err := qe.ShortestPath(ctx, ShortestPathRequest{
		SourceID: "A",
		TargetID: "B",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Distance != -1 {
		t.Errorf("expected distance -1 for unreachable, got %v", result.Distance)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(result.Steps))
	}
}
