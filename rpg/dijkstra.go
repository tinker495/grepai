package rpg

import "container/heap"

// edgeCost converts an edge weight (similarity score) to a traversal cost.
// Higher similarity → lower cost (closer nodes).
//   - weight=1.0 → cost=1.0 (structural/direct edge)
//   - weight=0.8 → cost=1.25
//   - weight=0.5 → cost=2.0
//   - weight≤0   → cost=10.0 (penalty for unweighted/zero edges)
func edgeCost(weight float64) float64 {
	if weight <= 0 {
		return 10.0
	}
	return 1.0 / weight
}

// ShortestPath computes Dijkstra's shortest path between source and target.
// It traverses edges bidirectionally (both adjForward and adjReverse).
// If edgeTypeSet is non-empty, only edges of those types are followed.
// Returns (nodeIDs along path, edges along path, total cost).
// Returns (nil, nil, -1) if the target is unreachable.
// Returns ([sourceID], nil, 0) if source == target.
func (g *Graph) ShortestPath(source, target string, edgeTypeSet map[EdgeType]bool) ([]string, []*Edge, float64) {
	return g.ShortestPathOpts(source, target, DistanceOptions{EdgeTypes: edgeTypeSet})
}

// ShortestPathOpts computes Dijkstra's shortest path with full options control.
func (g *Graph) ShortestPathOpts(source, target string, opts DistanceOptions) ([]string, []*Edge, float64) {
	if source == target {
		if g.GetNode(source) == nil {
			return nil, nil, -1
		}
		return []string{source}, nil, 0
	}
	if g.GetNode(source) == nil || g.GetNode(target) == nil {
		return nil, nil, -1
	}

	filterEdges := len(opts.EdgeTypes) > 0
	dir := opts.Direction
	metric := opts.Metric

	dist := map[string]float64{source: 0}
	prev := map[string]string{}    // prev node in shortest path
	prevEdge := map[string]*Edge{} // edge used to reach node

	pq := &priorityQueue{}
	heap.Init(pq)
	heap.Push(pq, &pqItem{nodeID: source, cost: 0})

	for pq.Len() > 0 {
		cur, _ := heap.Pop(pq).(*pqItem) //nolint:errcheck // heap.Pop always returns *pqItem

		if cur.nodeID == target {
			break
		}

		if cur.cost > dist[cur.nodeID] {
			continue // stale entry
		}

		edges := collectEdges(g, cur.nodeID, dir)

		for _, e := range edges {
			if filterEdges && !opts.EdgeTypes[e.Type] {
				continue
			}

			// Determine neighbor: the other end of the edge
			neighbor := e.To
			if neighbor == cur.nodeID {
				neighbor = e.From
			}

			if g.GetNode(neighbor) == nil {
				continue
			}

			newCost := cur.cost + edgeCostWithMetric(e.Weight, metric)
			if oldCost, ok := dist[neighbor]; !ok || newCost < oldCost {
				dist[neighbor] = newCost
				prev[neighbor] = cur.nodeID
				prevEdge[neighbor] = e
				heap.Push(pq, &pqItem{nodeID: neighbor, cost: newCost})
			}
		}
	}

	// Check if target was reached
	if _, ok := dist[target]; !ok {
		return nil, nil, -1
	}

	// Reconstruct path
	var path []string
	var pathEdges []*Edge
	for cur := target; cur != ""; cur = prev[cur] {
		path = append(path, cur)
		if e, ok := prevEdge[cur]; ok {
			pathEdges = append(pathEdges, e)
		}
	}

	// Reverse path and edges
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	for i, j := 0, len(pathEdges)-1; i < j; i, j = i+1, j-1 {
		pathEdges[i], pathEdges[j] = pathEdges[j], pathEdges[i]
	}

	return path, pathEdges, dist[target]
}

// DijkstraDirection controls which edges are traversed.
type DijkstraDirection string

const (
	DirBoth    DijkstraDirection = "both"    // traverse adjForward + adjReverse (default)
	DirForward DijkstraDirection = "forward" // adjForward only
	DirReverse DijkstraDirection = "reverse" // adjReverse only
)

// DistanceMetric controls how edge costs are computed.
type DistanceMetric string

const (
	MetricCost DistanceMetric = "cost" // 1.0/weight (default)
	MetricHops DistanceMetric = "hops" // always 1.0
)

// edgeCostWithMetric returns the traversal cost for an edge using the given metric.
func edgeCostWithMetric(weight float64, metric DistanceMetric) float64 {
	if metric == MetricHops {
		return 1.0
	}
	return edgeCost(weight)
}

// collectEdges returns the edges to consider for a node based on direction.
func collectEdges(g *Graph, nodeID string, dir DijkstraDirection) []*Edge {
	switch dir {
	case DirForward:
		return g.adjForward[nodeID]
	case DirReverse:
		return g.adjReverse[nodeID]
	default: // DirBoth
		var edges []*Edge
		edges = append(edges, g.adjForward[nodeID]...)
		edges = append(edges, g.adjReverse[nodeID]...)
		return edges
	}
}

// DistanceOptions configures the DistancesFrom computation.
type DistanceOptions struct {
	EdgeTypes map[EdgeType]bool // nil = all edge types
	MaxCost   float64           // 0 = unlimited; nodes beyond this cost are excluded
	Direction DijkstraDirection // "" or DirBoth = both directions
	Metric    DistanceMetric    // "" or MetricCost = cost metric
}

// DistancesFrom computes shortest-path distances from a single source to all
// reachable nodes using Dijkstra's algorithm. It traverses edges bidirectionally.
// Returns a map of nodeID -> cost. Nodes not in the map are unreachable.
// When MaxCost > 0, nodes with cost exceeding MaxCost are excluded.
func (g *Graph) DistancesFrom(source string, opts DistanceOptions) map[string]float64 {
	if g.GetNode(source) == nil {
		return nil
	}

	filterEdges := len(opts.EdgeTypes) > 0
	dir := opts.Direction
	metric := opts.Metric

	dist := map[string]float64{source: 0}

	pq := &priorityQueue{}
	heap.Init(pq)
	heap.Push(pq, &pqItem{nodeID: source, cost: 0})

	for pq.Len() > 0 {
		cur, _ := heap.Pop(pq).(*pqItem) //nolint:errcheck // heap.Pop always returns *pqItem

		if cur.cost > dist[cur.nodeID] {
			continue // stale entry
		}

		edges := collectEdges(g, cur.nodeID, dir)

		for _, e := range edges {
			if filterEdges && !opts.EdgeTypes[e.Type] {
				continue
			}

			// Determine neighbor: the other end of the edge
			neighbor := e.To
			if neighbor == cur.nodeID {
				neighbor = e.From
			}

			if g.GetNode(neighbor) == nil {
				continue
			}

			newCost := cur.cost + edgeCostWithMetric(e.Weight, metric)

			// Skip if beyond MaxCost
			if opts.MaxCost > 0 && newCost > opts.MaxCost {
				continue
			}

			if oldCost, ok := dist[neighbor]; !ok || newCost < oldCost {
				dist[neighbor] = newCost
				heap.Push(pq, &pqItem{nodeID: neighbor, cost: newCost})
			}
		}
	}

	return dist
}

// pqItem is an entry in the priority queue for Dijkstra.
type pqItem struct {
	nodeID string
	cost   float64
	index  int
}

// priorityQueue implements heap.Interface as a min-heap on cost.
type priorityQueue []*pqItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].cost < pq[j].cost }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x any) {
	item, _ := x.(*pqItem) //nolint:errcheck // heap.Push always passes *pqItem
	item.index = len(*pq)
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[:n-1]
	return item
}
