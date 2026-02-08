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

func TestDistancesFrom_Basic(t *testing.T) {
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

	dist := g.DistancesFrom("A", DistanceOptions{})
	if dist == nil {
		t.Fatal("expected non-nil distances")
	}

	// Source should have cost 0
	if dist["A"] != 0 {
		t.Errorf("expected dist[A]=0, got %v", dist["A"])
	}

	// B: cost = 1/1.0 = 1.0
	if math.Abs(dist["B"]-1.0) > 1e-9 {
		t.Errorf("expected dist[B]=1.0, got %v", dist["B"])
	}

	// C: cost = 1.0 + 1.0 = 2.0
	if math.Abs(dist["C"]-2.0) > 1e-9 {
		t.Errorf("expected dist[C]=2.0, got %v", dist["C"])
	}
}

func TestDistancesFrom_CrossValidateWithShortestPath(t *testing.T) {
	// Build a graph with mixed weights and verify DistancesFrom matches ShortestPath
	g := makeTestGraph(
		[]struct{ id, kind string }{
			{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}, {"D", "symbol"},
		},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
			{"A", "C", "invokes", 0.5},
			{"B", "D", "invokes", 0.8},
			{"C", "D", "invokes", 1.0},
		},
	)

	dist := g.DistancesFrom("A", DistanceOptions{})

	targets := []string{"B", "C", "D"}
	for _, target := range targets {
		_, _, spCost := g.ShortestPath("A", target, nil)
		dfCost, ok := dist[target]
		if !ok {
			t.Errorf("target %s not in DistancesFrom result", target)
			continue
		}
		if math.Abs(spCost-dfCost) > 1e-9 {
			t.Errorf("mismatch for %s: ShortestPath=%v, DistancesFrom=%v", target, spCost, dfCost)
		}
	}
}

func TestDistancesFrom_MaxCost(t *testing.T) {
	// A -> B (cost=1.0) -> C (cost=2.0) -> D (cost=3.0)
	g := makeTestGraph(
		[]struct{ id, kind string }{
			{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}, {"D", "symbol"},
		},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
			{"B", "C", "invokes", 1.0},
			{"C", "D", "invokes", 1.0},
		},
	)

	dist := g.DistancesFrom("A", DistanceOptions{MaxCost: 2.5})

	// A (0), B (1.0), C (2.0) should be present; D (3.0) should be pruned
	if _, ok := dist["A"]; !ok {
		t.Error("expected A in result")
	}
	if _, ok := dist["B"]; !ok {
		t.Error("expected B in result")
	}
	if _, ok := dist["C"]; !ok {
		t.Error("expected C in result")
	}
	if _, ok := dist["D"]; ok {
		t.Errorf("expected D to be pruned (cost=3.0 > MaxCost=2.5), got %v", dist["D"])
	}
}

func TestDistancesFrom_EdgeTypeFilter(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
			{"A", "C", "contains", 1.0},
		},
	)

	// Filter to invokes only
	dist := g.DistancesFrom("A", DistanceOptions{
		EdgeTypes: map[EdgeType]bool{EdgeInvokes: true},
	})

	if _, ok := dist["B"]; !ok {
		t.Error("expected B reachable via invokes")
	}
	if _, ok := dist["C"]; ok {
		t.Error("expected C unreachable when filtering to invokes only")
	}
}

func TestDistancesFrom_Bidirectional(t *testing.T) {
	// Edge A->B only, but DistancesFrom traverses bidirectionally
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
		},
	)

	// From B, should still reach A via reverse traversal
	dist := g.DistancesFrom("B", DistanceOptions{})
	if _, ok := dist["A"]; !ok {
		t.Error("expected A reachable from B via bidirectional traversal")
	}
}

func TestDistancesFrom_NonexistentSource(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}},
		nil,
	)

	dist := g.DistancesFrom("X", DistanceOptions{})
	if dist != nil {
		t.Errorf("expected nil for nonexistent source, got %v", dist)
	}
}

func TestShortestPath_ForwardOnly(t *testing.T) {
	// Edge A->B only. Forward from A should reach B, forward from B should NOT reach A.
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
		},
	)

	// Forward: A->B reachable
	nodeIDs, _, cost := g.ShortestPathOpts("A", "B", DistanceOptions{Direction: DirForward})
	if len(nodeIDs) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %v", len(nodeIDs), nodeIDs)
	}
	if math.Abs(cost-1.0) > 1e-9 {
		t.Errorf("expected cost 1.0, got %v", cost)
	}

	// Forward: B->A unreachable (no reverse traversal)
	nodeIDs2, _, cost2 := g.ShortestPathOpts("B", "A", DistanceOptions{Direction: DirForward})
	if nodeIDs2 != nil {
		t.Errorf("expected nil path for forward B->A, got %v", nodeIDs2)
	}
	if cost2 != -1 {
		t.Errorf("expected cost -1, got %v", cost2)
	}
}

func TestShortestPath_ReverseOnly(t *testing.T) {
	// Edge A->B only. Reverse from B should reach A, reverse from A should NOT reach B.
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
		},
	)

	// Reverse: B->A reachable (B sees A->B in adjReverse)
	nodeIDs, _, cost := g.ShortestPathOpts("B", "A", DistanceOptions{Direction: DirReverse})
	if len(nodeIDs) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %v", len(nodeIDs), nodeIDs)
	}
	if math.Abs(cost-1.0) > 1e-9 {
		t.Errorf("expected cost 1.0, got %v", cost)
	}

	// Reverse: A->B unreachable (A has no adjReverse entries for this edge)
	nodeIDs2, _, cost2 := g.ShortestPathOpts("A", "B", DistanceOptions{Direction: DirReverse})
	if nodeIDs2 != nil {
		t.Errorf("expected nil path for reverse A->B, got %v", nodeIDs2)
	}
	if cost2 != -1 {
		t.Errorf("expected cost -1, got %v", cost2)
	}
}

func TestDistancesFrom_HopsMetric(t *testing.T) {
	// A->B (weight=0.5, cost=2.0) -> C (weight=0.25, cost=4.0)
	// With hops metric, all edges cost 1.0 regardless of weight
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}, {"C", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 0.5},
			{"B", "C", "invokes", 0.25},
		},
	)

	// Hops metric: all edges = 1.0
	distHops := g.DistancesFrom("A", DistanceOptions{Metric: MetricHops})
	if math.Abs(distHops["B"]-1.0) > 1e-9 {
		t.Errorf("hops: expected dist[B]=1.0, got %v", distHops["B"])
	}
	if math.Abs(distHops["C"]-2.0) > 1e-9 {
		t.Errorf("hops: expected dist[C]=2.0, got %v", distHops["C"])
	}

	// Cost metric: B=2.0, C=6.0
	distCost := g.DistancesFrom("A", DistanceOptions{Metric: MetricCost})
	if math.Abs(distCost["B"]-2.0) > 1e-9 {
		t.Errorf("cost: expected dist[B]=2.0, got %v", distCost["B"])
	}
	if math.Abs(distCost["C"]-6.0) > 1e-9 {
		t.Errorf("cost: expected dist[C]=6.0, got %v", distCost["C"])
	}
}

func TestDistancesFrom_ForwardOnly(t *testing.T) {
	// Edge A->B. Forward from A should reach B, forward from B should NOT reach A.
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		[]struct {
			from, to, edgeType string
			weight             float64
		}{
			{"A", "B", "invokes", 1.0},
		},
	)

	dist := g.DistancesFrom("A", DistanceOptions{Direction: DirForward})
	if _, ok := dist["B"]; !ok {
		t.Error("forward from A: expected B reachable")
	}

	distB := g.DistancesFrom("B", DistanceOptions{Direction: DirForward})
	if _, ok := distB["A"]; ok {
		t.Error("forward from B: expected A unreachable")
	}
}

func TestEdgeCostWithMetric(t *testing.T) {
	// Hops always returns 1.0
	if edgeCostWithMetric(0.5, MetricHops) != 1.0 {
		t.Error("hops metric should always return 1.0")
	}
	if edgeCostWithMetric(0.0, MetricHops) != 1.0 {
		t.Error("hops metric should always return 1.0 even for zero weight")
	}

	// Cost delegates to edgeCost
	if math.Abs(edgeCostWithMetric(0.5, MetricCost)-2.0) > 1e-9 {
		t.Error("cost metric should return 1.0/0.5 = 2.0")
	}

	// Default (empty) uses cost
	if math.Abs(edgeCostWithMetric(0.5, "")-2.0) > 1e-9 {
		t.Error("empty metric should default to cost")
	}
}

func TestDistancesFrom_Disconnected(t *testing.T) {
	g := makeTestGraph(
		[]struct{ id, kind string }{{"A", "symbol"}, {"B", "symbol"}},
		nil,
	)

	dist := g.DistancesFrom("A", DistanceOptions{})
	if _, ok := dist["B"]; ok {
		t.Error("expected B unreachable from disconnected A")
	}
	if dist["A"] != 0 {
		t.Errorf("expected dist[A]=0, got %v", dist["A"])
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
