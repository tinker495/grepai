package rpg

import (
	"testing"
)

func TestClassifyFile(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	tests := []struct {
		name         string
		filePath     string
		expectedArea string
		expectedCat  string
	}{
		{
			name:         "two-level path",
			filePath:     "cli/watch.go",
			expectedArea: "cli",
			expectedCat:  "watch",
		},
		{
			name:         "store path",
			filePath:     "store/gob.go",
			expectedArea: "store",
			expectedCat:  "gob",
		},
		{
			name:         "root file",
			filePath:     "main.go",
			expectedArea: "root",
			expectedCat:  "main",
		},
		{
			name:         "three-level path",
			filePath:     "internal/foo/bar.go",
			expectedArea: "internal",
			expectedCat:  "foo",
		},
		{
			name:         "deep path",
			filePath:     "a/b/c/d/deep.go",
			expectedArea: "a",
			expectedCat:  "b",
		},
		{
			name:         "with leading slash",
			filePath:     "/cli/watch.go",
			expectedArea: "cli",
			expectedCat:  "watch",
		},
		{
			name:         "with dot slash",
			filePath:     "./store/gob.go",
			expectedArea: "store",
			expectedCat:  "gob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			area, cat := h.ClassifyFile(tt.filePath)
			if area != tt.expectedArea {
				t.Errorf("Expected area %s, got %s", tt.expectedArea, area)
			}
			if cat != tt.expectedCat {
				t.Errorf("Expected category %s, got %s", tt.expectedCat, cat)
			}
		})
	}
}

func TestClassifySymbol(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	tests := []struct {
		name     string
		feature  string
		expected string
	}{
		{
			name:     "handle verb",
			feature:  "handle-request",
			expected: "handle",
		},
		{
			name:     "validate verb",
			feature:  "validate-token",
			expected: "validate",
		},
		{
			name:     "with receiver",
			feature:  "validate-token@server",
			expected: "validate",
		},
		{
			name:     "empty feature",
			feature:  "",
			expected: "general",
		},
		{
			name:     "single word",
			feature:  "unknown",
			expected: "unknown",
		},
		{
			name:     "operate verb",
			feature:  "operate-server",
			expected: "operate",
		},
		{
			name:     "multi-word object",
			feature:  "get-user-by-id",
			expected: "get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.ClassifySymbol(tt.feature)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestEnsureArea(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	// First call should create the node
	id1 := h.EnsureArea("cli")
	if id1 != "area:cli" {
		t.Errorf("Expected ID 'area:cli', got %s", id1)
	}

	node := g.GetNode(id1)
	if node == nil {
		t.Fatal("Area node should be created")
	}
	if node.Kind != KindArea {
		t.Errorf("Expected kind %s, got %s", KindArea, node.Kind)
	}
	if node.Feature != "cli" {
		t.Errorf("Expected feature 'cli', got %s", node.Feature)
	}

	// Second call should be idempotent
	id2 := h.EnsureArea("cli")
	if id2 != id1 {
		t.Error("EnsureArea should be idempotent")
	}

	// Should only have one node
	areas := g.GetNodesByKind(KindArea)
	if len(areas) != 1 {
		t.Errorf("Expected 1 area node, got %d", len(areas))
	}
}

func TestEnsureCategory(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	areaID := h.EnsureArea("cli")

	// First call should create the node and edge
	catID1 := h.EnsureCategory(areaID, "watch")
	if catID1 != "cat:cli/watch" {
		t.Errorf("Expected ID 'cat:cli/watch', got %s", catID1)
	}

	node := g.GetNode(catID1)
	if node == nil {
		t.Fatal("Category node should be created")
	}
	if node.Kind != KindCategory {
		t.Errorf("Expected kind %s, got %s", KindCategory, node.Kind)
	}
	if node.Feature != "cli/watch" {
		t.Errorf("Expected feature 'cli/watch', got %s", node.Feature)
	}

	// Verify edge from category to area
	outgoing := g.GetOutgoing(catID1)
	if len(outgoing) != 1 {
		t.Fatalf("Expected 1 outgoing edge from category, got %d", len(outgoing))
	}
	if outgoing[0].To != areaID {
		t.Errorf("Category should have edge to area")
	}
	if outgoing[0].Type != EdgeContains {
		t.Errorf("Edge should be of type %s, got %s", EdgeContains, outgoing[0].Type)
	}

	// Second call should be idempotent
	catID2 := h.EnsureCategory(areaID, "watch")
	if catID2 != catID1 {
		t.Error("EnsureCategory should be idempotent")
	}

	// Should still have only one edge
	outgoing = g.GetOutgoing(catID1)
	if len(outgoing) != 1 {
		t.Errorf("Expected 1 outgoing edge after idempotent call, got %d", len(outgoing))
	}
}

func TestEnsureSubcategory(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	areaID := h.EnsureArea("cli")
	catID := h.EnsureCategory(areaID, "watch")

	// First call should create the node and edge
	subcatID1 := h.EnsureSubcategory(catID, "handle")
	if subcatID1 != "subcat:cli/watch/handle" {
		t.Errorf("Expected ID 'subcat:cli/watch/handle', got %s", subcatID1)
	}

	node := g.GetNode(subcatID1)
	if node == nil {
		t.Fatal("Subcategory node should be created")
	}
	if node.Kind != KindSubcategory {
		t.Errorf("Expected kind %s, got %s", KindSubcategory, node.Kind)
	}
	if node.Feature != "cli/watch/handle" {
		t.Errorf("Expected feature 'cli/watch/handle', got %s", node.Feature)
	}

	// Verify edge from subcategory to category
	outgoing := g.GetOutgoing(subcatID1)
	if len(outgoing) != 1 {
		t.Fatalf("Expected 1 outgoing edge from subcategory, got %d", len(outgoing))
	}
	if outgoing[0].To != catID {
		t.Errorf("Subcategory should have edge to category")
	}
	if outgoing[0].Type != EdgeContains {
		t.Errorf("Edge should be of type %s, got %s", EdgeContains, outgoing[0].Type)
	}

	// Second call should be idempotent
	subcatID2 := h.EnsureSubcategory(catID, "handle")
	if subcatID2 != subcatID1 {
		t.Error("EnsureSubcategory should be idempotent")
	}

	// Should still have only one edge
	outgoing = g.GetOutgoing(subcatID1)
	if len(outgoing) != 1 {
		t.Errorf("Expected 1 outgoing edge after idempotent call, got %d", len(outgoing))
	}
}

func TestBuildHierarchy(t *testing.T) {
	g := NewGraph()
	ext := NewLocalExtractor()
	h := NewHierarchyBuilder(g, ext)

	// Add a file node
	fileNode := &Node{
		ID:   "file:cli/watch.go",
		Kind: KindFile,
		Path: "cli/watch.go",
	}
	g.AddNode(fileNode)

	// Add symbol nodes
	sym1 := &Node{
		ID:         "sym:cli/watch.go:StartWatch",
		Kind:       KindSymbol,
		Path:       "cli/watch.go",
		SymbolName: "StartWatch",
		Feature:    "start-watch",
	}
	sym2 := &Node{
		ID:         "sym:cli/watch.go:StopWatch",
		Kind:       KindSymbol,
		Path:       "cli/watch.go",
		SymbolName: "StopWatch",
		Feature:    "stop-watch",
	}
	sym3 := &Node{
		ID:         "sym:cli/watch.go:HandleEvent",
		Kind:       KindSymbol,
		Path:       "cli/watch.go",
		SymbolName: "HandleEvent",
		Feature:    "handle-event",
	}
	g.AddNode(sym1)
	g.AddNode(sym2)
	g.AddNode(sym3)

	// Build hierarchy
	h.BuildHierarchy()

	// Verify area node exists
	areaNode := g.GetNode("area:cli")
	if areaNode == nil {
		t.Fatal("Area node should be created")
	}

	// Verify category node exists
	catNode := g.GetNode("cat:cli/watch")
	if catNode == nil {
		t.Fatal("Category node should be created")
	}

	// Verify subcategory nodes exist
	subcatStart := g.GetNode("subcat:cli/watch/start")
	if subcatStart == nil {
		t.Fatal("Subcategory 'start' should be created")
	}

	subcatStop := g.GetNode("subcat:cli/watch/stop")
	if subcatStop == nil {
		t.Fatal("Subcategory 'stop' should be created")
	}

	subcatHandle := g.GetNode("subcat:cli/watch/handle")
	if subcatHandle == nil {
		t.Fatal("Subcategory 'handle' should be created")
	}

	// Verify file -> category edge
	fileEdges := g.GetOutgoing(fileNode.ID)
	found := false
	for _, e := range fileEdges {
		if e.Type == EdgeFeatureParent && e.To == catNode.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("File should have EdgeFeatureParent edge to category")
	}

	// Verify symbol -> subcategory edges
	sym1Edges := g.GetOutgoing(sym1.ID)
	found = false
	for _, e := range sym1Edges {
		if e.Type == EdgeFeatureParent && e.To == subcatStart.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Symbol 'StartWatch' should have EdgeFeatureParent edge to 'start' subcategory")
	}

	// Verify category -> area edge
	catEdges := g.GetOutgoing(catNode.ID)
	found = false
	for _, e := range catEdges {
		if e.Type == EdgeContains && e.To == areaNode.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Category should have EdgeContains edge to area")
	}

	// Verify subcategory -> category edge
	subcatEdges := g.GetOutgoing(subcatStart.ID)
	found = false
	for _, e := range subcatEdges {
		if e.Type == EdgeContains && e.To == catNode.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Subcategory should have EdgeContains edge to category")
	}
}

func TestFileNameStem(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"go file", "chunker.go", "chunker"},
		{"yaml file", "config.yaml", "config"},
		{"no extension", "Makefile", "Makefile"},
		{"multiple dots", "test.spec.ts", "test.spec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fileNameStem(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
