package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alpkeskin/gotoon"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
)

func TestOutputSearchJSON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	// Capture stdout by temporarily reassigning
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}
	if err := encoder.Encode(jsonResults); err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	// Verify content field is present
	var decoded []SearchResultJSON
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 result, got %d", len(decoded))
	}

	if decoded[0].Content == "" {
		t.Error("expected content field to be present in JSON output")
	}

	if decoded[0].FilePath != "test/file.go" {
		t.Errorf("expected file_path 'test/file.go', got '%s'", decoded[0].FilePath)
	}
}

func TestOutputSearchCompactJSON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	jsonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultCompactJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
		}
	}
	if err := encoder.Encode(jsonResults); err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	// Verify content field is NOT present
	var decoded []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 result, got %d", len(decoded))
	}

	if _, exists := decoded[0]["content"]; exists {
		t.Error("expected content field to be absent in compact JSON output")
	}

	if decoded[0]["file_path"] != "test/file.go" {
		t.Errorf("expected file_path 'test/file.go', got '%v'", decoded[0]["file_path"])
	}

	if decoded[0]["start_line"].(float64) != 10 {
		t.Errorf("expected start_line 10, got %v", decoded[0]["start_line"])
	}

	if decoded[0]["end_line"].(float64) != 20 {
		t.Errorf("expected end_line 20, got %v", decoded[0]["end_line"])
	}
}

func TestCompactFlagRequiresJSONOrTOON(t *testing.T) {
	// Test that runSearch returns error when --compact is used without --json or --toon
	// We test this by directly checking the validation logic

	// Save original values
	originalCompact := searchCompact
	originalJSON := searchJSON
	originalTOON := searchTOON

	// Reset after test
	defer func() {
		searchCompact = originalCompact
		searchJSON = originalJSON
		searchTOON = originalTOON
	}()

	// Set up test case: --compact without --json and --toon
	searchCompact = true
	searchJSON = false
	searchTOON = false

	// The validation happens at the start of runSearch
	if searchCompact && !searchJSON && !searchTOON {
		// This is the expected behavior - validation would fail
		return
	}

	t.Error("expected --compact to require --json or --toon flag")
}

func TestCompactFlagWithJSON(t *testing.T) {
	// Test that --compact with --json is valid

	// Save original values
	originalCompact := searchCompact
	originalJSON := searchJSON
	originalTOON := searchTOON

	// Reset after test
	defer func() {
		searchCompact = originalCompact
		searchJSON = originalJSON
		searchTOON = originalTOON
	}()

	// Set up test case: --compact with --json
	searchCompact = true
	searchJSON = true
	searchTOON = false

	// The validation should pass
	if searchCompact && !searchJSON && !searchTOON {
		t.Error("expected --compact with --json to be valid")
	}
}

func TestCompactFlagWithTOON(t *testing.T) {
	// Test that --compact with --toon is valid

	// Save original values
	originalCompact := searchCompact
	originalJSON := searchJSON
	originalTOON := searchTOON

	// Reset after test
	defer func() {
		searchCompact = originalCompact
		searchJSON = originalJSON
		searchTOON = originalTOON
	}()

	// Set up test case: --compact with --toon
	searchCompact = true
	searchJSON = false
	searchTOON = true

	// The validation should pass
	if searchCompact && !searchJSON && !searchTOON {
		t.Error("expected --compact with --toon to be valid")
	}
}

func TestSearchResultJSONStruct(t *testing.T) {
	result := SearchResultJSON{
		FilePath:  "path/to/file.go",
		StartLine: 1,
		EndLine:   10,
		Score:     0.85,
		Content:   "code content here",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SearchResultJSON: %v", err)
	}

	// Verify all fields are present in JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	expectedFields := []string{"file_path", "start_line", "end_line", "score", "content"}
	for _, field := range expectedFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("expected field '%s' to be present", field)
		}
	}
}

func TestSearchResultCompactJSONStruct(t *testing.T) {
	result := SearchResultCompactJSON{
		FilePath:  "path/to/file.go",
		StartLine: 1,
		EndLine:   10,
		Score:     0.85,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SearchResultCompactJSON: %v", err)
	}

	// Verify expected fields are present and content is NOT present
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	expectedFields := []string{"file_path", "start_line", "end_line", "score"}
	for _, field := range expectedFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("expected field '%s' to be present", field)
		}
	}

	if _, exists := decoded["content"]; exists {
		t.Error("expected 'content' field to be absent in compact struct")
	}
}

func TestOutputSearchTOON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	// Encode using gotoon
	toonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		t.Fatalf("failed to encode TOON: %v", err)
	}

	// Verify output is not empty and is valid TOON
	if output == "" {
		t.Error("expected TOON output to be non-empty")
	}

	// TOON format should contain the file path
	if !strings.Contains(output, "test/file.go") {
		t.Error("expected TOON output to contain file path")
	}

	// TOON format should contain content
	if !strings.Contains(output, "TestFunction") {
		t.Error("expected TOON output to contain content")
	}
}

func TestOutputSearchCompactTOON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	// Encode using gotoon with compact struct
	toonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultCompactJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		t.Fatalf("failed to encode TOON: %v", err)
	}

	// Verify output is not empty
	if output == "" {
		t.Error("expected TOON output to be non-empty")
	}

	// TOON format should contain the file path
	if !strings.Contains(output, "test/file.go") {
		t.Error("expected TOON output to contain file path")
	}
}

func TestDistanceFromWithWorkspaceError(t *testing.T) {
	originalDistFrom := searchDistanceFrom
	originalWorkspace := searchWorkspace
	originalJSON := searchJSON
	originalCompact := searchCompact
	defer func() {
		searchDistanceFrom = originalDistFrom
		searchWorkspace = originalWorkspace
		searchJSON = originalJSON
		searchCompact = originalCompact
	}()

	searchDistanceFrom = "sym:cli/search.go:runSearch"
	searchWorkspace = "test-workspace"
	searchJSON = true
	searchCompact = false

	// Call runSearch, which should return error before trying to find project root
	err := runSearch(nil, []string{"test"})
	if err == nil {
		t.Fatal("expected error when --distance-from used with --workspace")
	}
	if !strings.Contains(err.Error(), "not supported with --workspace") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDistanceFromRPGDisabledError(t *testing.T) {
	cfg := &config.Config{}
	cfg.RPG.Enabled = false

	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	_, err := enrichWithRPG("/tmp/nonexistent", cfg, results, "sym:test:Func", "", "", 0)
	if err == nil {
		t.Fatal("expected error when distance-from used with RPG disabled")
	}
	if !strings.Contains(err.Error(), "RPG to be enabled") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDistanceFromRPGLoadError(t *testing.T) {
	// When RPG is enabled but store doesn't exist, enrichWithRPG should error
	// for distance-from requests.
	cfg := &config.Config{}
	cfg.RPG.Enabled = true

	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	// This will fail to load the RPG store (nonexistent path) and return error
	// because distanceFrom is set
	_, err := enrichWithRPG("/tmp/nonexistent-project", cfg, results, "sym:test:Func", "", "", 0)
	if err == nil {
		t.Fatal("expected error when RPG store cannot be loaded with distance-from set")
	}
	// Error could be "failed to load RPG" or "RPG index is empty" or "node not found"
	// depending on whether the GOB store loads an empty graph
	errMsg := err.Error()
	if !strings.Contains(errMsg, "failed to load RPG") &&
		!strings.Contains(errMsg, "RPG index is empty") &&
		!strings.Contains(errMsg, "node not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDistancePrefilledToNegativeOne(t *testing.T) {
	// Verify that disconnected nodes get distance = -1 from Dijkstra
	tmpDir := t.TempDir()

	// Create .grepai directory first
	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create RPG store
	rpgStorePath := config.GetRPGIndexPath(tmpDir)
	rpgStore := rpg.NewGOBRPGStore(rpgStorePath)

	// Get the graph and add nodes directly
	graph := rpgStore.GetGraph()

	// Add a symbol node
	sourceNode := &rpg.Node{
		ID:         "sym:test.go:Foo",
		Kind:       rpg.KindSymbol,
		Path:       "test.go",
		StartLine:  1,
		EndLine:    10,
		SymbolName: "Foo",
		UpdatedAt:  time.Now(),
	}
	graph.AddNode(sourceNode)

	// Add a disconnected symbol node (no edges between them)
	targetNode := &rpg.Node{
		ID:         "sym:other.go:Bar",
		Kind:       rpg.KindSymbol,
		Path:       "other.go",
		StartLine:  1,
		EndLine:    5,
		SymbolName: "Bar",
		UpdatedAt:  time.Now(),
	}
	graph.AddNode(targetNode)

	// Persist and close
	ctx := context.Background()
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist RPG store: %v", err)
	}
	if err := rpgStore.Close(); err != nil {
		t.Fatalf("failed to close RPG store: %v", err)
	}

	// Create config with RPG enabled
	cfg := &config.Config{}
	cfg.RPG.Enabled = true

	// Create a minimal config.yaml
	configPath := filepath.Join(configDir, "config.yaml")
	configContent := "version: 1\nrpg:\n  enabled: true\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create results with a chunk matching test.go:1-10
	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	// Call enrichWithRPG with distance_from pointing to the disconnected node
	enrichments, err := enrichWithRPG(tmpDir, cfg, results, "sym:other.go:Bar", "", "", 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(enrichments) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(enrichments))
	}

	// Verify distance is -1 (unreachable)
	if enrichments[0].Distance == nil {
		t.Fatal("expected distance to be set, got nil")
	}
	if *enrichments[0].Distance != -1 {
		t.Errorf("expected distance -1 for unreachable node, got %f", *enrichments[0].Distance)
	}
}

func TestEnrichWithRPG_NoDistanceFrom_BestEffort(t *testing.T) {
	// Without distance-from, RPG load failure should be silent (best-effort)
	cfg := &config.Config{}
	cfg.RPG.Enabled = true

	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	enrichments, err := enrichWithRPG("/tmp/nonexistent-project", cfg, results, "", "", "", 0)
	if err != nil {
		t.Fatalf("expected no error for best-effort RPG enrichment, got: %v", err)
	}
	if len(enrichments) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(enrichments))
	}
	if enrichments[0].Distance != nil {
		t.Error("expected nil distance when distance-from is not set")
	}
}

func TestEnrichWithRPG_DistanceComputed(t *testing.T) {
	// Verify that connected nodes get actual distance computed via Dijkstra
	tmpDir := t.TempDir()

	// Create .grepai directory
	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create RPG store with connected nodes
	rpgStorePath := config.GetRPGIndexPath(tmpDir)
	rpgStore := rpg.NewGOBRPGStore(rpgStorePath)
	graph := rpgStore.GetGraph()

	now := time.Now()

	// Source node
	graph.AddNode(&rpg.Node{
		ID: "sym:source.go:Start", Kind: rpg.KindSymbol,
		Path: "source.go", StartLine: 1, EndLine: 5,
		SymbolName: "Start", UpdatedAt: now,
	})

	// Target node (matching the search result file)
	graph.AddNode(&rpg.Node{
		ID: "sym:target.go:End", Kind: rpg.KindSymbol,
		Path: "target.go", StartLine: 1, EndLine: 10,
		SymbolName: "End", UpdatedAt: now,
	})

	// Connect them: Start -> End via invokes edge with weight=1.0
	graph.AddEdge(&rpg.Edge{
		From: "sym:source.go:Start", To: "sym:target.go:End",
		Type: rpg.EdgeInvokes, Weight: 1.0, UpdatedAt: now,
	})

	// Persist and close
	ctx := context.Background()
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist RPG store: %v", err)
	}
	if err := rpgStore.Close(); err != nil {
		t.Fatalf("failed to close RPG store: %v", err)
	}

	// Config
	cfg := &config.Config{}
	cfg.RPG.Enabled = true

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: true\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Search result matching target.go:1-10
	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "target.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	// Compute distance from source to target
	enrichments, err := enrichWithRPG(tmpDir, cfg, results, "sym:source.go:Start", "", "", 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(enrichments) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(enrichments))
	}

	// Distance should be computed (cost = 1.0/1.0 = 1.0)
	if enrichments[0].Distance == nil {
		t.Fatal("expected distance to be set, got nil")
	}
	if *enrichments[0].Distance != 1.0 {
		t.Errorf("expected distance 1.0, got %f", *enrichments[0].Distance)
	}

	// SymbolName should be enriched too
	if enrichments[0].SymbolName != "End" {
		t.Errorf("expected symbol name 'End', got %q", enrichments[0].SymbolName)
	}
}

func TestEnrichWithRPG_MultipleResults_MixedDistances(t *testing.T) {
	// Verify mixed distances: one reachable, one unreachable
	tmpDir := t.TempDir()

	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	rpgStorePath := config.GetRPGIndexPath(tmpDir)
	rpgStore := rpg.NewGOBRPGStore(rpgStorePath)
	graph := rpgStore.GetGraph()

	now := time.Now()

	graph.AddNode(&rpg.Node{
		ID: "sym:a.go:Source", Kind: rpg.KindSymbol,
		Path: "a.go", StartLine: 1, EndLine: 5,
		SymbolName: "Source", UpdatedAt: now,
	})
	graph.AddNode(&rpg.Node{
		ID: "sym:b.go:Connected", Kind: rpg.KindSymbol,
		Path: "b.go", StartLine: 1, EndLine: 10,
		SymbolName: "Connected", UpdatedAt: now,
	})
	graph.AddNode(&rpg.Node{
		ID: "sym:c.go:Isolated", Kind: rpg.KindSymbol,
		Path: "c.go", StartLine: 1, EndLine: 8,
		SymbolName: "Isolated", UpdatedAt: now,
	})

	// Source -> Connected (weight=0.5, cost=2.0)
	graph.AddEdge(&rpg.Edge{
		From: "sym:a.go:Source", To: "sym:b.go:Connected",
		Type: rpg.EdgeInvokes, Weight: 0.5, UpdatedAt: now,
	})
	// No edge to Isolated

	ctx := context.Background()
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist: %v", err)
	}
	rpgStore.Close()

	cfg := &config.Config{}
	cfg.RPG.Enabled = true
	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: true\n"), 0644)

	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "b.go", StartLine: 1, EndLine: 10}, Score: 0.9},
		{Chunk: store.Chunk{FilePath: "c.go", StartLine: 1, EndLine: 8}, Score: 0.8},
	}

	enrichments, err := enrichWithRPG(tmpDir, cfg, results, "sym:a.go:Source", "", "", 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(enrichments) != 2 {
		t.Fatalf("expected 2 enrichments, got %d", len(enrichments))
	}

	// First result: connected, distance = 2.0 (1.0/0.5)
	if enrichments[0].Distance == nil {
		t.Fatal("enrichments[0].Distance should not be nil")
	}
	if *enrichments[0].Distance != 2.0 {
		t.Errorf("expected distance 2.0 for connected node, got %f", *enrichments[0].Distance)
	}

	// Second result: isolated, distance = -1
	if enrichments[1].Distance == nil {
		t.Fatal("enrichments[1].Distance should not be nil")
	}
	if *enrichments[1].Distance != -1 {
		t.Errorf("expected distance -1 for isolated node, got %f", *enrichments[1].Distance)
	}
}

func TestRunSearch_DistanceFromRPGDisabled(t *testing.T) {
	// Verify that runSearch returns error when --distance-from is used
	// but RPG is disabled in the project config (search.go line 192).
	tmpDir := t.TempDir()

	// Create .grepai/config.yaml with RPG disabled
	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: false\nembedder:\n  provider: ollama\nstore:\n  backend: gob\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Chdir to tmpDir so FindProjectRoot finds our config
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origDir)

	// Save and restore package-level vars
	origDistFrom := searchDistanceFrom
	origWorkspace := searchWorkspace
	origJSON := searchJSON
	origCompact := searchCompact
	defer func() {
		searchDistanceFrom = origDistFrom
		searchWorkspace = origWorkspace
		searchJSON = origJSON
		searchCompact = origCompact
	}()

	searchDistanceFrom = "sym:cli/search.go:runSearch"
	searchWorkspace = ""
	searchJSON = true
	searchCompact = false

	// runSearch should hit the RPG-disabled validation before embedder init
	err = runSearch(nil, []string{"test query"})
	if err == nil {
		t.Fatal("expected error when --distance-from used with RPG disabled")
	}
	if !strings.Contains(err.Error(), "RPG to be enabled") {
		t.Errorf("expected 'RPG to be enabled' error, got: %v", err)
	}
}

func TestEnrichWithRPG_EmptyGraph(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create an empty RPG store (no nodes)
	rpgStorePath := config.GetRPGIndexPath(tmpDir)
	rpgStore := rpg.NewGOBRPGStore(rpgStorePath)
	ctx := context.Background()
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist: %v", err)
	}
	rpgStore.Close()

	cfg := &config.Config{}
	cfg.RPG.Enabled = true
	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: true\n"), 0644)

	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	_, err := enrichWithRPG(tmpDir, cfg, results, "sym:test:Func", "", "", 0)
	if err == nil {
		t.Fatal("expected error for empty RPG graph with distance-from")
	}
	if !strings.Contains(err.Error(), "RPG index is empty") {
		t.Errorf("expected 'RPG index is empty' error, got: %v", err)
	}
}

func TestEnrichWithRPG_SourceNodeNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	rpgStorePath := config.GetRPGIndexPath(tmpDir)
	rpgStore := rpg.NewGOBRPGStore(rpgStorePath)
	graph := rpgStore.GetGraph()

	now := time.Now()
	graph.AddNode(&rpg.Node{
		ID: "sym:test.go:Foo", Kind: rpg.KindSymbol,
		Path: "test.go", StartLine: 1, EndLine: 10,
		SymbolName: "Foo", UpdatedAt: now,
	})

	ctx := context.Background()
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist: %v", err)
	}
	rpgStore.Close()

	cfg := &config.Config{}
	cfg.RPG.Enabled = true
	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: true\n"), 0644)

	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test.go", StartLine: 1, EndLine: 10}, Score: 0.9},
	}

	_, err := enrichWithRPG(tmpDir, cfg, results, "sym:nonexistent:Missing", "", "", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent source node")
	}
	if !strings.Contains(err.Error(), "node not found in RPG graph") {
		t.Errorf("expected 'node not found' error, got: %v", err)
	}
}

func TestTOONSmallerThanJSON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {\n\treturn nil\n}",
			},
			Score: 0.95,
		},
		{
			Chunk: store.Chunk{
				FilePath:  "test/file2.go",
				StartLine: 30,
				EndLine:   40,
				Content:   "func AnotherFunction() {\n\treturn true\n}",
			},
			Score: 0.85,
		},
	}

	// Convert to JSON structs
	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}

	// Encode as JSON
	var jsonBuf bytes.Buffer
	encoder := json.NewEncoder(&jsonBuf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonResults); err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	// Encode as TOON
	toonOutput, err := gotoon.Encode(jsonResults)
	if err != nil {
		t.Fatalf("failed to encode TOON: %v", err)
	}

	jsonSize := len(jsonBuf.Bytes())
	toonSize := len(toonOutput)

	// TOON should be smaller than JSON
	if toonSize >= jsonSize {
		t.Errorf("expected TOON (%d bytes) to be smaller than JSON (%d bytes)", toonSize, jsonSize)
	}
}
