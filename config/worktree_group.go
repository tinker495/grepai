package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	WorktreeGroupConfigFileName = "grepai-worktrees.yaml"
)

// WorktreeGroupConfig holds configuration for a group of git worktrees sharing
// a common repository. Stored at <git-common-dir>/grepai-worktrees.yaml.
type WorktreeGroupConfig struct {
	Version   int             `yaml:"version"`
	GroupID   string          `yaml:"group_id"` // WorktreeID from git package
	Store     StoreConfig     `yaml:"store"`
	Embedder  EmbedderConfig  `yaml:"embedder"`
	Worktrees []WorktreeEntry `yaml:"worktrees"`
	CreatedAt time.Time       `yaml:"created_at"`
}

// WorktreeEntry represents a single worktree in the group.
type WorktreeEntry struct {
	Path   string `yaml:"path"`
	Name   string `yaml:"name"`   // Short name (branch name or "main")
	Branch string `yaml:"branch"` // Current branch
}

// GetWorktreeGroupConfigPath returns the path to the worktree group config file.
func GetWorktreeGroupConfigPath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, WorktreeGroupConfigFileName)
}

// LoadWorktreeGroupConfig loads the worktree group config from the git common directory.
// Returns nil, nil if the file doesn't exist.
func LoadWorktreeGroupConfig(gitCommonDir string) (*WorktreeGroupConfig, error) {
	configPath := GetWorktreeGroupConfigPath(gitCommonDir)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read worktree group config: %w", err)
	}

	var cfg WorktreeGroupConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse worktree group config: %w", err)
	}

	return &cfg, nil
}

// SaveWorktreeGroupConfig saves the worktree group config to the git common directory.
func SaveWorktreeGroupConfig(gitCommonDir string, cfg *WorktreeGroupConfig) error {
	configPath := GetWorktreeGroupConfigPath(gitCommonDir)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal worktree group config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write worktree group config: %w", err)
	}

	return nil
}

// DiscoverWorktrees uses `git worktree list --porcelain` to discover all worktrees
// for the repository rooted at gitCommonDir.
func DiscoverWorktrees(gitCommonDir string) ([]WorktreeEntry, error) {
	// Find the main worktree path from git common dir
	// The common dir is typically <main-worktree>/.git or <main-worktree>/.git/worktrees/../
	mainWorktree := resolveMainWorktree(gitCommonDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", mainWorktree, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return parseWorktreeList(string(output)), nil
}

// resolveMainWorktree resolves the main worktree path from the git common directory.
func resolveMainWorktree(gitCommonDir string) string {
	// If gitCommonDir ends with /.git, the main worktree is the parent
	if filepath.Base(gitCommonDir) == ".git" {
		return filepath.Dir(gitCommonDir)
	}
	// Otherwise use it as-is (might be a bare repo or already resolved)
	return gitCommonDir
}

// parseWorktreeList parses the output of `git worktree list --porcelain`.
// Format:
//
//	worktree /path/to/main
//	HEAD abc123
//	branch refs/heads/main
//
//	worktree /path/to/linked
//	HEAD def456
//	branch refs/heads/feature
func parseWorktreeList(output string) []WorktreeEntry {
	var entries []WorktreeEntry
	var current *WorktreeEntry

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current = &WorktreeEntry{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			branch := strings.TrimPrefix(line, "branch ")
			// Extract short branch name from refs/heads/...
			branch = strings.TrimPrefix(branch, "refs/heads/")
			current.Branch = branch
			current.Name = branch
		} else if line == "detached" && current != nil {
			current.Branch = "(detached)"
			current.Name = filepath.Base(current.Path)
		}
	}

	// Don't forget the last entry
	if current != nil {
		entries = append(entries, *current)
	}

	return entries
}

// WorktreeGroupToWorkspace converts a WorktreeGroupConfig to a Workspace
// for reuse with existing workspace infrastructure (watch, search, etc).
func WorktreeGroupToWorkspace(cfg *WorktreeGroupConfig) *Workspace {
	projects := make([]ProjectEntry, len(cfg.Worktrees))
	for i, wt := range cfg.Worktrees {
		projects[i] = ProjectEntry{
			Name: wt.Name,
			Path: wt.Path,
		}
	}

	return &Workspace{
		Name:     "worktree-group-" + cfg.GroupID,
		Store:    cfg.Store,
		Embedder: cfg.Embedder,
		Projects: projects,
	}
}

// FindWorktreeGroupForPath checks if the given path is within a git repo
// that has a worktree group configured.
// Returns the git common dir and config if found.
func FindWorktreeGroupForPath(targetPath string) (string, *WorktreeGroupConfig, error) {
	// Try to detect git info
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", targetPath, "rev-parse", "--git-common-dir")
	output, err := cmd.Output()
	if err != nil {
		return "", nil, nil // Not a git repo, not an error
	}

	gitCommonDir := strings.TrimSpace(string(output))
	if !filepath.IsAbs(gitCommonDir) {
		gitCommonDir = filepath.Join(targetPath, gitCommonDir)
	}
	gitCommonDir = filepath.Clean(gitCommonDir)

	cfg, err := LoadWorktreeGroupConfig(gitCommonDir)
	if err != nil {
		return "", nil, err
	}

	return gitCommonDir, cfg, nil
}
