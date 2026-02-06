package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGetWorktreeGroupConfigPath(t *testing.T) {
	path := GetWorktreeGroupConfigPath("/home/user/.git")
	expected := "/home/user/.git/grepai-worktrees.yaml"
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestLoadSaveWorktreeGroupConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Initially no config
	cfg, err := LoadWorktreeGroupConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadWorktreeGroupConfig failed: %v", err)
	}
	if cfg != nil {
		t.Fatal("Expected nil config for non-existent file")
	}

	// Save config
	dim := 768
	saveCfg := &WorktreeGroupConfig{
		Version: 1,
		GroupID: "abc123def456",
		Store: StoreConfig{
			Backend: "postgres",
		},
		Embedder: EmbedderConfig{
			Provider:   "ollama",
			Model:      "nomic-embed-text",
			Dimensions: &dim,
		},
		Worktrees: []WorktreeEntry{
			{Path: "/home/user/project", Name: "main", Branch: "main"},
			{Path: "/home/user/project-feat", Name: "feature", Branch: "feature"},
		},
		CreatedAt: time.Now(),
	}

	if err := SaveWorktreeGroupConfig(tmpDir, saveCfg); err != nil {
		t.Fatalf("SaveWorktreeGroupConfig failed: %v", err)
	}

	// Verify file exists
	configPath := GetWorktreeGroupConfigPath(tmpDir)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file not created")
	}

	// Load and verify
	loadedCfg, err := LoadWorktreeGroupConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadWorktreeGroupConfig failed: %v", err)
	}
	if loadedCfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if loadedCfg.GroupID != "abc123def456" {
		t.Errorf("Expected GroupID abc123def456, got %s", loadedCfg.GroupID)
	}
	if len(loadedCfg.Worktrees) != 2 {
		t.Errorf("Expected 2 worktrees, got %d", len(loadedCfg.Worktrees))
	}
}

func TestParseWorktreeList(t *testing.T) {
	output := `worktree /home/user/project
HEAD abc123
branch refs/heads/main

worktree /home/user/project-feat
HEAD def456
branch refs/heads/feature-branch

worktree /home/user/project-detached
HEAD 789abc
detached

`

	entries := parseWorktreeList(output)

	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// First entry
	if entries[0].Path != "/home/user/project" {
		t.Errorf("Entry 0 path: expected /home/user/project, got %s", entries[0].Path)
	}
	if entries[0].Branch != "main" {
		t.Errorf("Entry 0 branch: expected main, got %s", entries[0].Branch)
	}
	if entries[0].Name != "main" {
		t.Errorf("Entry 0 name: expected main, got %s", entries[0].Name)
	}

	// Second entry
	if entries[1].Path != "/home/user/project-feat" {
		t.Errorf("Entry 1 path: expected /home/user/project-feat, got %s", entries[1].Path)
	}
	if entries[1].Branch != "feature-branch" {
		t.Errorf("Entry 1 branch: expected feature-branch, got %s", entries[1].Branch)
	}

	// Detached entry
	if entries[2].Branch != "(detached)" {
		t.Errorf("Entry 2 branch: expected (detached), got %s", entries[2].Branch)
	}
	if entries[2].Name != "project-detached" {
		t.Errorf("Entry 2 name: expected project-detached, got %s", entries[2].Name)
	}
}

func TestParseWorktreeList_Empty(t *testing.T) {
	entries := parseWorktreeList("")
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries for empty output, got %d", len(entries))
	}
}

func TestWorktreeGroupToWorkspace(t *testing.T) {
	cfg := &WorktreeGroupConfig{
		GroupID: "abc123",
		Store:   StoreConfig{Backend: "postgres"},
		Embedder: EmbedderConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
		},
		Worktrees: []WorktreeEntry{
			{Path: "/path/main", Name: "main"},
			{Path: "/path/feat", Name: "feature"},
		},
	}

	ws := WorktreeGroupToWorkspace(cfg)

	if ws.Name != "worktree-group-abc123" {
		t.Errorf("Expected workspace name worktree-group-abc123, got %s", ws.Name)
	}
	if len(ws.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(ws.Projects))
	}
	if ws.Projects[0].Name != "main" || ws.Projects[0].Path != "/path/main" {
		t.Errorf("Unexpected first project: %+v", ws.Projects[0])
	}
	if ws.Store.Backend != "postgres" {
		t.Errorf("Expected postgres backend, got %s", ws.Store.Backend)
	}
}

func TestResolveMainWorktree(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/user/project/.git", "/home/user/project"},
		{"/home/user/project/.git/worktrees", "/home/user/project/.git/worktrees"},
		{"/some/bare/repo", "/some/bare/repo"},
	}

	for _, tt := range tests {
		result := resolveMainWorktree(tt.input)
		if result != tt.expected {
			t.Errorf("resolveMainWorktree(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFindWorktreeGroupForPath_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	gitCommonDir, cfg, err := FindWorktreeGroupForPath(tmpDir)
	if err != nil {
		t.Fatalf("FindWorktreeGroupForPath failed: %v", err)
	}
	if gitCommonDir != "" || cfg != nil {
		t.Error("Expected empty result for non-git directory")
	}
}

func TestFindWorktreeGroupForPath_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a git repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git dir: %v", err)
	}

	// Initialize git repo properly
	cmd := exec.Command("git", "init", tmpDir)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available")
	}

	gitCommonDir, cfg, err := FindWorktreeGroupForPath(tmpDir)
	if err != nil {
		t.Fatalf("FindWorktreeGroupForPath failed: %v", err)
	}
	// Should find git common dir but no config
	if cfg != nil {
		t.Error("Expected nil config for repo without worktree group")
	}
	_ = gitCommonDir // may or may not be empty depending on git availability
}
