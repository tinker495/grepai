package rpg

import (
	"context"
	"sort"
	"strings"
)

// SearchNodeRequest is the input for SearchNode.
type SearchNodeRequest struct {
	Query string     `json:"query"`           // natural language or feature query
	Scope string     `json:"scope,omitempty"` // area/category path to narrow search
	Kinds []NodeKind `json:"kinds,omitempty"` // filter by node kind (default: symbol)
	Limit int        `json:"limit,omitempty"` // max results (default: 10)
}

// SearchNodeResult is a single search result.
type SearchNodeResult struct {
	Node        *Node   `json:"node"`
	Score       float64 `json:"score"`
	FeaturePath string  `json:"feature_path"` // area/category/subcategory path
}

// FetchNodeRequest is the input for FetchNode.
type FetchNodeRequest struct {
	NodeID string `json:"node_id"`
}

// FetchNodeResult contains detailed node info with context.
type FetchNodeResult struct {
	Node        *Node   `json:"node"`
	FeaturePath string  `json:"feature_path"`
	Parents     []*Node `json:"parents,omitempty"`  // hierarchy chain
	Children    []*Node `json:"children,omitempty"` // contained nodes
	Incoming    []*Edge `json:"incoming,omitempty"` // incoming edges
	Outgoing    []*Edge `json:"outgoing,omitempty"` // outgoing edges
}

// ExploreRequest is the input for Explore.
type ExploreRequest struct {
	StartNodeID string     `json:"start_node_id"`
	Direction   string     `json:"direction"`            // forward, reverse, both
	Depth       int        `json:"depth,omitempty"`      // max depth (default: 2)
	EdgeTypes   []EdgeType `json:"edge_types,omitempty"` // filter by edge type
	Limit       int        `json:"limit,omitempty"`      // max nodes returned
}

// ExploreResult contains the explored subgraph.
type ExploreResult struct {
	StartNode *Node            `json:"start_node"`
	Nodes     map[string]*Node `json:"nodes"`
	Edges     []*Edge          `json:"edges"`
	Depth     int              `json:"depth"`
}

// QueryEngine provides the 3 RPG query operations.
type QueryEngine struct {
	graph *Graph
}

// NewQueryEngine creates a QueryEngine backed by the given graph.
func NewQueryEngine(graph *Graph) *QueryEngine {
	return &QueryEngine{graph: graph}
}

// SearchNode finds nodes matching a query within optional scope.
// Scoring uses Jaccard similarity between query words and the union of a
// node's Feature label words and SymbolName words.
func (qe *QueryEngine) SearchNode(_ context.Context, req SearchNodeRequest) ([]SearchNodeResult, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if len(req.Kinds) == 0 {
		req.Kinds = []NodeKind{KindSymbol}
	}

	queryWords := normalizeWords(req.Query)
	if len(queryWords) == 0 {
		return nil, nil
	}

	// Build the set of allowed kinds for fast lookup.
	kindSet := make(map[NodeKind]bool, len(req.Kinds))
	for _, k := range req.Kinds {
		kindSet[k] = true
	}

	// Collect candidate nodes.
	var candidates []*Node
	for _, kind := range req.Kinds {
		candidates = append(candidates, qe.graph.GetNodesByKind(kind)...)
	}

	// Filter by scope if set. Scope is matched as a prefix of the node's
	// feature path (e.g. "cli" or "cli/commands").
	scopeFilter := ""
	if req.Scope != "" {
		scopeFilter = strings.ToLower(req.Scope)
	}

	type scored struct {
		node  *Node
		score float64
		path  string
	}
	var results []scored

	for _, n := range candidates {
		if !kindSet[n.Kind] {
			continue
		}

		featurePath := qe.getFeaturePath(n.ID)

		// Apply scope filter.
		if scopeFilter != "" {
			if !strings.HasPrefix(strings.ToLower(featurePath), scopeFilter) {
				continue
			}
		}

		score := scoreMatch(queryWords, n)
		if score > 0 {
			results = append(results, scored{node: n, score: score, path: featurePath})
		}
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Apply limit.
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	out := make([]SearchNodeResult, len(results))
	for i, r := range results {
		out[i] = SearchNodeResult{
			Node:        r.node,
			Score:       r.score,
			FeaturePath: r.path,
		}
	}
	return out, nil
}

// FetchNode retrieves detailed information about a specific node.
func (qe *QueryEngine) FetchNode(_ context.Context, req FetchNodeRequest) (*FetchNodeResult, error) {
	node := qe.graph.GetNode(req.NodeID)
	if node == nil {
		return nil, nil
	}

	result := &FetchNodeResult{
		Node:        node,
		FeaturePath: qe.getFeaturePath(node.ID),
		Incoming:    qe.graph.GetIncoming(node.ID),
		Outgoing:    qe.graph.GetOutgoing(node.ID),
	}

	// Build hierarchy chain by walking upward through parent links.
	visited := make(map[string]bool)
	current := node.ID
	for {
		parentID := findParentID(qe.graph, current)
		if parentID == "" || visited[parentID] {
			break
		}
		visited[parentID] = true
		parentNode := qe.graph.GetNode(parentID)
		if parentNode == nil {
			break
		}
		result.Parents = append(result.Parents, parentNode)
		current = parentID
	}

	// Collect direct children via outgoing hierarchy/containment edges.
	for _, e := range qe.graph.GetOutgoing(node.ID) {
		if e.Type == EdgeContains || e.Type == EdgeFeatureParent {
			if child := qe.graph.GetNode(e.To); child != nil {
				result.Children = append(result.Children, child)
			}
		}
	}

	return result, nil
}

// Explore traverses the graph from a start node using BFS.
func (qe *QueryEngine) Explore(_ context.Context, req ExploreRequest) (*ExploreResult, error) {
	if req.Depth <= 0 {
		req.Depth = 2
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}

	startNode := qe.graph.GetNode(req.StartNodeID)
	if startNode == nil {
		return nil, nil
	}

	// Build edge type filter set.
	edgeTypeSet := make(map[EdgeType]bool, len(req.EdgeTypes))
	for _, et := range req.EdgeTypes {
		edgeTypeSet[et] = true
	}
	filterEdges := len(edgeTypeSet) > 0

	result := &ExploreResult{
		StartNode: startNode,
		Nodes:     make(map[string]*Node),
		Edges:     make([]*Edge, 0),
		Depth:     0,
	}
	result.Nodes[startNode.ID] = startNode

	// BFS state.
	type bfsEntry struct {
		nodeID string
		depth  int
	}
	queue := []bfsEntry{{nodeID: startNode.ID, depth: 0}}
	visited := map[string]bool{startNode.ID: true}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		if entry.depth >= req.Depth {
			continue
		}

		if len(result.Nodes) >= req.Limit {
			break
		}

		var edges []*Edge

		switch req.Direction {
		case "forward":
			edges = qe.graph.GetOutgoing(entry.nodeID)
		case "reverse":
			edges = qe.graph.GetIncoming(entry.nodeID)
		default: // "both"
			edges = append(edges, qe.graph.GetOutgoing(entry.nodeID)...)
			edges = append(edges, qe.graph.GetIncoming(entry.nodeID)...)
		}

		for _, e := range edges {
			// Apply edge type filter.
			if filterEdges && !edgeTypeSet[e.Type] {
				continue
			}

			result.Edges = append(result.Edges, e)

			// Determine the neighbor ID.
			neighborID := e.To
			if neighborID == entry.nodeID {
				neighborID = e.From
			}

			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighborNode := qe.graph.GetNode(neighborID)
			if neighborNode == nil {
				continue
			}

			result.Nodes[neighborID] = neighborNode

			if len(result.Nodes) >= req.Limit {
				break
			}

			// Track max depth reached.
			nextDepth := entry.depth + 1
			if nextDepth > result.Depth {
				result.Depth = nextDepth
			}

			queue = append(queue, bfsEntry{nodeID: neighborID, depth: nextDepth})
		}
	}

	return result, nil
}

// getFeaturePath returns the full feature path for a node.
// Hierarchy nodes (area/category/subcategory) already store their full path in Feature.
// File nodes use incoming feature-parent links, symbol nodes inherit file path.
func (qe *QueryEngine) getFeaturePath(nodeID string) string {
	node := qe.graph.GetNode(nodeID)
	if node == nil {
		return ""
	}

	// Hierarchy nodes already store the full path in Feature
	if node.Kind == KindArea || node.Kind == KindCategory || node.Kind == KindSubcategory {
		return node.Feature
	}

	if node.Kind == KindFile {
		for _, e := range qe.graph.GetIncoming(nodeID) {
			if e.Type != EdgeFeatureParent {
				continue
			}
			parent := qe.graph.GetNode(e.From)
			if parent != nil {
				return parent.Feature
			}
		}
		return ""
	}

	if node.Kind == KindSymbol {
		for _, e := range qe.graph.GetIncoming(nodeID) {
			if e.Type != EdgeContains {
				continue
			}
			fileNode := qe.graph.GetNode(e.From)
			if fileNode != nil && fileNode.Kind == KindFile {
				return qe.getFeaturePath(fileNode.ID)
			}
		}
		return ""
	}

	if node.Kind == KindChunk {
		for _, e := range qe.graph.GetIncoming(nodeID) {
			if e.Type != EdgeMapsToChunk {
				continue
			}
			symNode := qe.graph.GetNode(e.From)
			if symNode != nil && symNode.Kind == KindSymbol {
				return qe.getFeaturePath(symNode.ID)
			}
		}
	}

	return ""
}

// findParentID finds the hierarchy parent of a node by looking at outgoing
// incoming EdgeFeatureParent and EdgeContains edges.
func findParentID(g *Graph, nodeID string) string {
	// Hierarchy/file parent links.
	for _, e := range g.GetIncoming(nodeID) {
		if e.Type == EdgeFeatureParent {
			return e.From
		}
	}
	// Symbol/file containment parent links.
	for _, e := range g.GetIncoming(nodeID) {
		if e.Type == EdgeContains {
			return e.From
		}
	}
	return ""
}

// scoreMatch computes Jaccard similarity between query words and the combined
// word set from node features and SymbolName.
func scoreMatch(queryWords []string, node *Node) float64 {
	// Build the node word set from primary/atomic features and symbol name.
	nodeWordSet := make(map[string]bool)
	for _, w := range normalizeWords(node.Feature) {
		nodeWordSet[w] = true
	}
	for _, feature := range node.Features {
		for _, w := range normalizeWords(feature) {
			nodeWordSet[w] = true
		}
	}
	for _, w := range normalizeWords(node.SymbolName) {
		nodeWordSet[w] = true
	}

	if len(nodeWordSet) == 0 {
		return 0
	}

	querySet := make(map[string]bool, len(queryWords))
	for _, w := range queryWords {
		querySet[w] = true
	}

	// Jaccard = |intersection| / |union|.
	union := make(map[string]bool)
	for w := range querySet {
		union[w] = true
	}
	for w := range nodeWordSet {
		union[w] = true
	}

	intersectionCount := 0
	for w := range querySet {
		if nodeWordSet[w] {
			intersectionCount++
		}
	}

	if len(union) == 0 {
		return 0
	}

	return float64(intersectionCount) / float64(len(union))
}

// normalizeWords splits a string on common separators and lowercases each
// word. Suitable for feature labels ("handle-request"), symbol names
// ("HandleRequest"), and natural language queries.
func normalizeWords(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ToLower(s)

	// Split on common delimiters: dash, underscore, slash, space, dot, @.
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == '/' || r == ' ' || r == '.' || r == '@'
	})

	// Deduplicate while preserving order.
	seen := make(map[string]bool, len(parts))
	var result []string
	for _, p := range parts {
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
