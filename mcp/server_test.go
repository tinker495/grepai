package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/rpg"
)

// TestServerCreateEmbedder_AppliesConfiguredDimensions verifies that createEmbedder
// passes configured dimension into each embedder constructor.
func TestServerCreateEmbedder_AppliesConfiguredDimensions(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		dimensions int
		apiKey     string
	}{
		{name: "ollama", provider: "ollama", dimensions: 768},
		{name: "lmstudio", provider: "lmstudio", dimensions: 768},
		{name: "openai-1536", provider: "openai", dimensions: 1536, apiKey: "sk-test"},
		{name: "openai-3072", provider: "openai", dimensions: 3072, apiKey: "sk-test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			cfg := config.DefaultConfig()
			cfg.Embedder.Provider = tt.provider
			cfg.Embedder.Dimensions = &tt.dimensions
			cfg.Embedder.APIKey = tt.apiKey

			emb, err := s.createEmbedder(cfg)
			if err != nil {
				t.Fatalf("createEmbedder returned error: %v", err)
			}

			if emb.Dimensions() != tt.dimensions {
				t.Fatalf("expected dimensions %d, got %d", tt.dimensions, emb.Dimensions())
			}
		})
	}
}

// TestCompactStructDefinitions verifies compact struct definitions.
func TestCompactStructDefinitions(t *testing.T) {
	t.Run("SearchResultCompact has no Content field", func(t *testing.T) {
		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
		}

		jsonBytes, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "content") {
			t.Errorf("Compact struct should not contain 'content' field, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "file_path") {
			t.Errorf("Compact struct should contain 'file_path' field, got: %s", jsonStr)
		}
	})

	t.Run("CallSiteCompact has no Context field", func(t *testing.T) {
		compact := CallSiteCompact{
			File: "test.go",
			Line: 10,
		}

		jsonBytes, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(jsonBytes)
		if strings.Contains(jsonStr, "context") {
			t.Errorf("Compact struct should not contain 'context' field, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "file") {
			t.Errorf("Compact struct should contain 'file' field, got: %s", jsonStr)
		}
	})
}

// TestCompactStructMarshaling verifies JSON marshaling of compact structs.
func TestCompactStructMarshaling(t *testing.T) {
	t.Run("SearchResult vs SearchResultCompact", func(t *testing.T) {
		full := SearchResult{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
			Content:   "line 1\nline 2\nline 3",
		}

		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 10,
			EndLine:   20,
			Score:     0.95,
		}

		fullJSON, err := json.Marshal(full)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		compactJSON, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if len(compactJSON) >= len(fullJSON) {
			t.Errorf("Compact JSON should be shorter than full JSON, got compact=%d, full=%d", len(compactJSON), len(fullJSON))
		}

		if !strings.Contains(string(fullJSON), "content") {
			t.Errorf("Full JSON should contain 'content' field")
		}

		if strings.Contains(string(compactJSON), "content") {
			t.Errorf("Compact JSON should not contain 'content' field")
		}
	})
}

// TestNonCompactSearchResult verifies that the full SearchResult struct
// includes all expected fields when NOT in compact mode.
func TestNonCompactSearchResult(t *testing.T) {
	result := SearchResult{
		FilePath:  "example/test.go",
		StartLine: 42,
		EndLine:   50,
		Score:     0.87,
		Content:   "func example() {\n\treturn true\n}",
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify all required fields are present
	expectedFields := []string{
		"file_path",
		"start_line",
		"end_line",
		"score",
		"content",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Non-compact JSON should contain '%s' field, got: %s", field, jsonStr)
		}
	}

	// Verify content field has correct value
	if !strings.Contains(jsonStr, "func example()") {
		t.Errorf("Non-compact JSON should contain full content, got: %s", jsonStr)
	}

	// Verify score is present and non-zero
	if !strings.Contains(jsonStr, "0.87") {
		t.Errorf("Non-compact JSON should contain score value, got: %s", jsonStr)
	}
}

// TestServerCreateStore_GOBBackend tests createStore with gob backend
func TestServerCreateStore_GOBBackend(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "gob"

	ctx := context.Background()
	store, err := s.createStore(ctx, cfg)

	if err != nil {
		t.Fatalf("createStore returned error: %v", err)
	}

	if store == nil {
		t.Error("expected non-nil store")
	}

	_ = store.Close()
}

// TestServerCreateStore_UnknownBackend tests that createStore returns error for unknown backend
func TestServerCreateStore_UnknownBackend(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	cfg := config.DefaultConfig()
	cfg.Store.Backend = "unknown-backend"

	ctx := context.Background()
	_, err := s.createStore(ctx, cfg)

	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}

	expected := "unknown storage backend: unknown-backend"
	if err.Error() != expected {
		t.Errorf("expected error message %s, got %s", expected, err.Error())
	}
}

// TestRegisterTools_IndexStatusSchema verifies that the grepai_index_status tool
// is registered with a non-empty schema (regression for empty schema error).
func TestRegisterTools_IndexStatusSchema(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}

	// initialize minimal MCP server like NewServer would
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")

	s.registerTools()

	tools := s.mcpServer.ListTools()
	indexStatus, ok := tools["grepai_index_status"]
	if !ok {
		t.Fatalf("grepai_index_status tool not registered")
	}

	schema := indexStatus.Tool.InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %q", schema.Type)
	}

	prop, ok := schema.Properties["verbose"]
	if !ok {
		t.Fatalf("expected verbose property in schema")
	}

	propMap, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("verbose property is not an object, got %T", prop)
	}

	if propMap["type"] != "boolean" {
		t.Fatalf("expected verbose type boolean, got %v", propMap["type"])
	}
}

func TestNewServerWithWorkspace(t *testing.T) {
	t.Run("creates_server_with_workspace", func(t *testing.T) {
		srv, err := NewServerWithWorkspace("", "orbix")
		if err != nil {
			t.Fatalf("NewServerWithWorkspace error: %v", err)
		}
		if srv.workspaceName != "orbix" {
			t.Errorf("expected workspace orbix, got %s", srv.workspaceName)
		}
		if srv.projectRoot != "" {
			t.Errorf("expected empty projectRoot, got %s", srv.projectRoot)
		}
	})

	t.Run("creates_server_with_project_and_workspace", func(t *testing.T) {
		srv, err := NewServerWithWorkspace("/tmp/project", "orbix")
		if err != nil {
			t.Fatalf("NewServerWithWorkspace error: %v", err)
		}
		if srv.workspaceName != "orbix" {
			t.Errorf("expected workspace orbix, got %s", srv.workspaceName)
		}
		if srv.projectRoot != "/tmp/project" {
			t.Errorf("expected project /tmp/project, got %s", srv.projectRoot)
		}
	})
}

func TestRegisterTools_TracePathSchema(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")
	s.registerTools()

	tools := s.mcpServer.ListTools()
	tracePath, ok := tools["grepai_trace_path"]
	if !ok {
		t.Fatalf("grepai_trace_path tool not registered")
	}

	schema := tracePath.Tool.InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected schema type object, got %q", schema.Type)
	}

	// Verify required params exist
	for _, param := range []string{"source_id", "target_id"} {
		if _, exists := schema.Properties[param]; !exists {
			t.Errorf("expected %s property in schema", param)
		}
	}

	// Verify optional params exist
	for _, param := range []string{"edge_types", "format"} {
		if _, exists := schema.Properties[param]; !exists {
			t.Errorf("expected %s property in schema", param)
		}
	}

	// Verify source_id and target_id are required
	required := make(map[string]bool)
	for _, r := range schema.Required {
		required[r] = true
	}
	if !required["source_id"] {
		t.Error("source_id should be required")
	}
	if !required["target_id"] {
		t.Error("target_id should be required")
	}
}

func TestSearchResultDistanceField(t *testing.T) {
	t.Run("distance_present_when_set", func(t *testing.T) {
		d := 2.5
		result := SearchResult{
			FilePath:  "test.go",
			StartLine: 1,
			EndLine:   10,
			Score:     0.9,
			Content:   "func Foo() {}",
			Distance:  &d,
		}

		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(data)
		if !strings.Contains(jsonStr, "\"distance\"") {
			t.Errorf("expected distance field in JSON, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "2.5") {
			t.Errorf("expected distance value 2.5, got: %s", jsonStr)
		}
	})

	t.Run("distance_omitted_when_nil", func(t *testing.T) {
		result := SearchResult{
			FilePath:  "test.go",
			StartLine: 1,
			EndLine:   10,
			Score:     0.9,
			Content:   "func Foo() {}",
		}

		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if strings.Contains(string(data), "distance") {
			t.Errorf("expected no distance field when nil, got: %s", string(data))
		}
	})

	t.Run("compact_distance_present_when_set", func(t *testing.T) {
		d := -1.0
		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 1,
			EndLine:   10,
			Score:     0.9,
			Distance:  &d,
		}

		data, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		jsonStr := string(data)
		if !strings.Contains(jsonStr, "\"distance\"") {
			t.Errorf("expected distance field in compact JSON, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "-1") {
			t.Errorf("expected distance value -1, got: %s", jsonStr)
		}
	})

	t.Run("compact_distance_omitted_when_nil", func(t *testing.T) {
		compact := SearchResultCompact{
			FilePath:  "test.go",
			StartLine: 1,
			EndLine:   10,
			Score:     0.9,
		}

		data, err := json.Marshal(compact)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if strings.Contains(string(data), "distance") {
			t.Errorf("expected no distance field when nil, got: %s", string(data))
		}
	})
}

func TestRegisterTools_SearchDistanceFromParam(t *testing.T) {
	s := &Server{
		projectRoot: "/tmp/test-project",
	}
	s.mcpServer = server.NewMCPServer("grepai-test", "1.0.0")
	s.registerTools()

	tools := s.mcpServer.ListTools()
	searchTool, ok := tools["grepai_search"]
	if !ok {
		t.Fatalf("grepai_search tool not registered")
	}

	schema := searchTool.Tool.InputSchema
	if _, exists := schema.Properties["distance_from"]; !exists {
		t.Error("expected distance_from property in grepai_search schema")
	}
}

func TestHandleSearch_DistanceFromValidation(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func(t *testing.T) *Server
		args          map[string]any
		expectedError string
	}{
		{
			name: "distance_from with workspace error",
			setupServer: func(t *testing.T) *Server {
				return &Server{projectRoot: "/tmp/test-project"}
			},
			args: map[string]any{
				"query":         "test",
				"distance_from": "sym:test:Func",
				"workspace":     "my-workspace",
			},
			expectedError: "not supported with workspace",
		},
		{
			name: "distance_from with RPG disabled error",
			setupServer: func(t *testing.T) *Server {
				tmpDir := t.TempDir()
				// Create .grepai/config.yaml with RPG disabled
				configDir := config.GetConfigDir(tmpDir)
				if err := os.MkdirAll(configDir, 0755); err != nil {
					t.Fatalf("failed to create config dir: %v", err)
				}
				configPath := filepath.Join(configDir, "config.yaml")
				configContent := "version: 1\nrpg:\n  enabled: false\n"
				if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
				return &Server{projectRoot: tmpDir}
			},
			args: map[string]any{
				"query":         "test",
				"distance_from": "sym:test:Func",
			},
			expectedError: "RPG to be enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setupServer(t)
			ctx := context.Background()

			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.args

			result, err := s.handleSearch(ctx, req)
			if err != nil {
				t.Fatalf("handleSearch returned error: %v", err)
			}

			// Check that result is an error result
			if !result.IsError {
				t.Fatal("expected error result")
			}

			// Extract error message from result content
			if len(result.Content) == 0 {
				t.Fatal("expected error content")
			}

			content, ok := result.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("expected TextContent, got %T", result.Content[0])
			}

			if !strings.Contains(content.Text, tt.expectedError) {
				t.Errorf("expected error containing %q, got: %s", tt.expectedError, content.Text)
			}
		})
	}
}

func TestHandleTracePath_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .grepai/config.yaml with RPG enabled
	configDir := config.GetConfigDir(tmpDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: true\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create RPG store with connected nodes
	rpgStorePath := config.GetRPGIndexPath(tmpDir)
	rpgStore := rpg.NewGOBRPGStore(rpgStorePath)
	graph := rpgStore.GetGraph()

	now := time.Now()
	graph.AddNode(&rpg.Node{
		ID: "sym:a.go:Func1", Kind: rpg.KindSymbol,
		Path: "a.go", StartLine: 1, EndLine: 5,
		SymbolName: "Func1", UpdatedAt: now,
	})
	graph.AddNode(&rpg.Node{
		ID: "sym:b.go:Func2", Kind: rpg.KindSymbol,
		Path: "b.go", StartLine: 1, EndLine: 10,
		SymbolName: "Func2", UpdatedAt: now,
	})
	graph.AddEdge(&rpg.Edge{
		From: "sym:a.go:Func1", To: "sym:b.go:Func2",
		Type: rpg.EdgeInvokes, Weight: 1.0, UpdatedAt: now,
	})

	ctx := context.Background()
	if err := rpgStore.Persist(ctx); err != nil {
		t.Fatalf("failed to persist: %v", err)
	}
	rpgStore.Close()

	// Create server pointing to tmpDir
	s := &Server{projectRoot: tmpDir}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"source_id": "sym:a.go:Func1",
		"target_id": "sym:b.go:Func2",
	}

	result, err := s.handleTracePath(ctx, req)
	if err != nil {
		t.Fatalf("handleTracePath returned error: %v", err)
	}
	if result.IsError {
		content, _ := result.Content[0].(mcp.TextContent)
		t.Fatalf("expected success, got error: %s", content.Text)
	}

	// Verify result contains path data as JSON
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	// Parse JSON and verify distance
	var pathResult struct {
		Distance float64 `json:"distance"`
		Steps    []struct {
			Node struct {
				ID string `json:"id"`
			} `json:"node"`
			Cost float64 `json:"cost"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(content.Text), &pathResult); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	// Distance should be 1.0 (cost = 1.0/1.0)
	if pathResult.Distance != 1.0 {
		t.Errorf("expected distance 1.0, got %f", pathResult.Distance)
	}

	// Should have 2 steps: source and target
	if len(pathResult.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(pathResult.Steps))
	}
	if pathResult.Steps[0].Node.ID != "sym:a.go:Func1" {
		t.Errorf("expected first step Func1, got %s", pathResult.Steps[0].Node.ID)
	}
	if pathResult.Steps[1].Node.ID != "sym:b.go:Func2" {
		t.Errorf("expected second step Func2, got %s", pathResult.Steps[1].Node.ID)
	}
	// Cumulative costs: step0=0.0, step1=1.0
	if pathResult.Steps[0].Cost != 0.0 {
		t.Errorf("expected step[0] cost 0.0, got %f", pathResult.Steps[0].Cost)
	}
	if pathResult.Steps[1].Cost != 1.0 {
		t.Errorf("expected step[1] cost 1.0, got %f", pathResult.Steps[1].Cost)
	}
}

func TestHandleTracePath_ErrorPaths(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func(t *testing.T) *Server
		args          map[string]any
		expectedError string
	}{
		{
			name: "RPG unavailable",
			setupServer: func(t *testing.T) *Server {
				tmpDir := t.TempDir()
				configDir := config.GetConfigDir(tmpDir)
				os.MkdirAll(configDir, 0755)
				configPath := filepath.Join(configDir, "config.yaml")
				os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: false\n"), 0644)
				return &Server{projectRoot: tmpDir}
			},
			args: map[string]any{
				"source_id": "sym:a:Func",
				"target_id": "sym:b:Func",
			},
			expectedError: "RPG is not enabled",
		},
		{
			name: "invalid edge type",
			setupServer: func(t *testing.T) *Server {
				tmpDir := t.TempDir()
				configDir := config.GetConfigDir(tmpDir)
				os.MkdirAll(configDir, 0755)
				configPath := filepath.Join(configDir, "config.yaml")
				os.WriteFile(configPath, []byte("version: 1\nrpg:\n  enabled: true\n"), 0644)
				// Create RPG store with nodes so tryLoadRPG succeeds
				rpgStorePath := config.GetRPGIndexPath(tmpDir)
				rpgStore := rpg.NewGOBRPGStore(rpgStorePath)
				graph := rpgStore.GetGraph()
				now := time.Now()
				graph.AddNode(&rpg.Node{
					ID: "sym:a:Func", Kind: rpg.KindSymbol,
					Path: "a.go", StartLine: 1, EndLine: 5,
					SymbolName: "Func", UpdatedAt: now,
				})
				ctx := context.Background()
				rpgStore.Persist(ctx)
				rpgStore.Close()
				return &Server{projectRoot: tmpDir}
			},
			args: map[string]any{
				"source_id":  "sym:a:Func",
				"target_id":  "sym:b:Func",
				"edge_types": "invalid_type",
			},
			expectedError: "invalid edge type",
		},
		{
			name: "invalid format",
			setupServer: func(t *testing.T) *Server {
				return &Server{projectRoot: "/tmp/test"}
			},
			args: map[string]any{
				"source_id": "sym:a:Func",
				"target_id": "sym:b:Func",
				"format":    "xml",
			},
			expectedError: "format must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setupServer(t)
			ctx := context.Background()

			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.args

			result, err := s.handleTracePath(ctx, req)
			if err != nil {
				t.Fatalf("handleTracePath returned error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error result")
			}
			if len(result.Content) == 0 {
				t.Fatal("expected error content")
			}
			content, ok := result.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("expected TextContent, got %T", result.Content[0])
			}
			if !strings.Contains(content.Text, tt.expectedError) {
				t.Errorf("expected error containing %q, got: %s", tt.expectedError, content.Text)
			}
		})
	}
}
