package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
	"github.com/yoanbernabeu/grepai/git"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktree groups for shared indexing",
	Long: `Manage git worktree groups for cross-worktree indexing and search.

When working with git worktrees, grepai can share a single index across all
worktrees of the same repository, avoiding redundant embedding costs.

Requirements:
  - Must be inside a git repository
  - Shared backend required: PostgreSQL or Qdrant (GOB not supported)

Commands:
  grepai worktree list      List detected git worktrees
  grepai worktree status    Show worktree group status
  grepai worktree init      Initialize a worktree group config
  grepai worktree watch     Start shared watcher for all worktrees`,
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List detected git worktrees",
	RunE:  runWorktreeList,
}

var worktreeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show worktree group status",
	RunE:  runWorktreeStatus,
}

var worktreeInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a worktree group configuration",
	Long: `Initialize shared configuration for all worktrees of the current repository.

This creates a grepai-worktrees.yaml file in the git common directory,
which is shared by all worktrees. A shared backend (PostgreSQL or Qdrant)
is required.`,
	RunE: runWorktreeInit,
}

var worktreeWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Start shared watcher for all worktrees",
	Long: `Start a workspace-style watcher that indexes all worktrees in the group.

This reuses the workspace watch infrastructure to monitor all worktrees
with a single daemon process and shared vector store.`,
	RunE: runWorktreeWatch,
}

func init() {
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeStatusCmd)
	worktreeCmd.AddCommand(worktreeInitCmd)
	worktreeCmd.AddCommand(worktreeWatchCmd)

	worktreeWatchCmd.Flags().Bool("background", false, "Run in background")
	worktreeWatchCmd.Flags().String("log-dir", "", "Custom log directory")
}

func detectGitInfo() (*git.DetectInfo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	info, err := git.Detect(cwd)
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}

	return info, nil
}

func runWorktreeList(cmd *cobra.Command, args []string) error {
	gitInfo, err := detectGitInfo()
	if err != nil {
		return err
	}

	worktrees, err := config.DiscoverWorktrees(gitInfo.GitCommonDir)
	if err != nil {
		return fmt.Errorf("failed to discover worktrees: %w", err)
	}

	if len(worktrees) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	fmt.Printf("Git worktrees (%d):\n\n", len(worktrees))
	for _, wt := range worktrees {
		marker := " "
		if wt.Path == gitInfo.GitRoot {
			marker = "*" // Current worktree
		}
		fmt.Printf("  %s %s\n", marker, wt.Path)
		fmt.Printf("    Branch: %s\n", wt.Branch)
		if wt.Name != "" {
			fmt.Printf("    Name: %s\n", wt.Name)
		}
	}

	// Show group config status
	cfg, err := config.LoadWorktreeGroupConfig(gitInfo.GitCommonDir)
	if err != nil {
		return nil // Don't fail, just skip
	}
	if cfg != nil {
		fmt.Printf("\nWorktree group: configured (ID: %s)\n", cfg.GroupID)
		fmt.Printf("  Backend: %s\n", cfg.Store.Backend)
		fmt.Printf("  Embedder: %s (%s)\n", cfg.Embedder.Provider, cfg.Embedder.Model)
	} else {
		fmt.Printf("\nWorktree group: not configured\n")
		fmt.Println("  Run 'grepai worktree init' to set up shared indexing")
	}

	return nil
}

func runWorktreeStatus(cmd *cobra.Command, args []string) error {
	gitInfo, err := detectGitInfo()
	if err != nil {
		return err
	}

	cfg, err := config.LoadWorktreeGroupConfig(gitInfo.GitCommonDir)
	if err != nil {
		return fmt.Errorf("failed to load worktree group config: %w", err)
	}

	if cfg == nil {
		fmt.Println("No worktree group configured.")
		fmt.Println("\nRun 'grepai worktree init' to set up shared indexing.")
		return nil
	}

	fmt.Printf("Worktree Group: %s\n\n", cfg.GroupID)
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Backend: %s\n", cfg.Store.Backend)
	fmt.Printf("  Embedder: %s (%s)\n", cfg.Embedder.Provider, cfg.Embedder.Model)
	fmt.Printf("  Created: %s\n", cfg.CreatedAt.Format("2006-01-02 15:04:05"))

	// Show worktrees and their status
	fmt.Printf("\nWorktrees (%d):\n", len(cfg.Worktrees))

	logDir, _ := daemon.GetDefaultLogDir()
	wsName := "worktree-group-" + cfg.GroupID

	for _, wt := range cfg.Worktrees {
		exists := "ok"
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			exists = "path not found"
		}
		fmt.Printf("  - %s: %s (%s)\n", wt.Name, wt.Path, exists)
	}

	// Check shared daemon status
	if logDir != "" {
		pid, _ := daemon.GetRunningWorkspacePID(logDir, wsName)
		if pid > 0 {
			fmt.Printf("\nShared daemon: running (PID %d)\n", pid)
		} else {
			fmt.Printf("\nShared daemon: not running\n")
			fmt.Println("  Start with: grepai worktree watch")
		}
	}

	return nil
}

func runWorktreeInit(cmd *cobra.Command, args []string) error {
	gitInfo, err := detectGitInfo()
	if err != nil {
		return err
	}

	// Check if already configured
	existingCfg, err := config.LoadWorktreeGroupConfig(gitInfo.GitCommonDir)
	if err != nil {
		return fmt.Errorf("failed to check existing config: %w", err)
	}
	if existingCfg != nil {
		fmt.Printf("Worktree group already configured (ID: %s)\n", existingCfg.GroupID)
		fmt.Println("To reconfigure, delete the file and run init again:")
		fmt.Printf("  rm %s\n", config.GetWorktreeGroupConfigPath(gitInfo.GitCommonDir))
		return nil
	}

	// Discover worktrees
	worktrees, err := config.DiscoverWorktrees(gitInfo.GitCommonDir)
	if err != nil {
		return fmt.Errorf("failed to discover worktrees: %w", err)
	}

	fmt.Printf("Detected %d worktree(s):\n", len(worktrees))
	for _, wt := range worktrees {
		fmt.Printf("  - %s (%s)\n", wt.Path, wt.Branch)
	}

	reader := bufio.NewReader(os.Stdin)

	// Select backend
	fmt.Println("\nSelect shared storage backend (GOB not supported for shared indexing):")
	fmt.Println("  1. PostgreSQL (recommended)")
	fmt.Println("  2. Qdrant")
	fmt.Print("Choice [1]: ")
	backendChoice, _ := reader.ReadString('\n')
	backendChoice = strings.TrimSpace(backendChoice)
	if backendChoice == "" {
		backendChoice = "1"
	}

	var storeConfig config.StoreConfig
	switch backendChoice {
	case "1":
		storeConfig.Backend = "postgres"
		fmt.Print("PostgreSQL DSN [postgres://localhost:5432/grepai]: ")
		dsn, _ := reader.ReadString('\n')
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			dsn = "postgres://localhost:5432/grepai"
		}
		storeConfig.Postgres.DSN = dsn
	case "2":
		storeConfig.Backend = "qdrant"
		fmt.Print("Qdrant endpoint [http://localhost]: ")
		endpoint, _ := reader.ReadString('\n')
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			endpoint = "http://localhost"
		}
		storeConfig.Qdrant.Endpoint = endpoint
		storeConfig.Qdrant.Port = 6334
	default:
		return fmt.Errorf("invalid choice: %s", backendChoice)
	}

	// Select embedder
	fmt.Println("\nSelect embedding provider:")
	fmt.Println("  1. Ollama (local, default)")
	fmt.Println("  2. OpenAI")
	fmt.Print("Choice [1]: ")
	embedderChoice, _ := reader.ReadString('\n')
	embedderChoice = strings.TrimSpace(embedderChoice)
	if embedderChoice == "" {
		embedderChoice = "1"
	}

	var embedderConfig config.EmbedderConfig
	switch embedderChoice {
	case "1":
		embedderConfig.Provider = "ollama"
		embedderConfig.Endpoint = "http://localhost:11434"
		embedderConfig.Model = "nomic-embed-text"
		dim := 768
		embedderConfig.Dimensions = &dim
	case "2":
		embedderConfig.Provider = "openai"
		embedderConfig.Endpoint = "https://api.openai.com/v1"
		fmt.Print("OpenAI API Key: ")
		apiKey, _ := reader.ReadString('\n')
		embedderConfig.APIKey = strings.TrimSpace(apiKey)
		embedderConfig.Model = "text-embedding-3-small"
	default:
		return fmt.Errorf("invalid choice: %s", embedderChoice)
	}

	// Save config
	cfg := &config.WorktreeGroupConfig{
		Version:   1,
		GroupID:   gitInfo.WorktreeID,
		Store:     storeConfig,
		Embedder:  embedderConfig,
		Worktrees: worktrees,
		CreatedAt: time.Now(),
	}

	if err := config.SaveWorktreeGroupConfig(gitInfo.GitCommonDir, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\nWorktree group configured successfully!\n")
	fmt.Printf("  Group ID: %s\n", cfg.GroupID)
	fmt.Printf("  Config: %s\n", config.GetWorktreeGroupConfigPath(gitInfo.GitCommonDir))
	fmt.Printf("\nStart shared indexing with: grepai worktree watch\n")

	return nil
}

func runWorktreeWatch(cmd *cobra.Command, args []string) error {
	gitInfo, err := detectGitInfo()
	if err != nil {
		return err
	}

	cfg, err := config.LoadWorktreeGroupConfig(gitInfo.GitCommonDir)
	if err != nil {
		return fmt.Errorf("failed to load worktree group config: %w", err)
	}

	if cfg == nil {
		return fmt.Errorf("no worktree group configured; run 'grepai worktree init' first")
	}

	// Refresh worktree list
	worktrees, err := config.DiscoverWorktrees(gitInfo.GitCommonDir)
	if err != nil {
		fmt.Printf("Warning: could not refresh worktree list: %v\n", err)
	} else {
		cfg.Worktrees = worktrees
		// Save updated list
		_ = config.SaveWorktreeGroupConfig(gitInfo.GitCommonDir, cfg)
	}

	// Convert to workspace and use existing workspace watch infrastructure
	ws := config.WorktreeGroupToWorkspace(cfg)

	// Validate backend
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	// Save as workspace config so the watch --workspace flag can find it
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		wsCfg = config.DefaultWorkspaceConfig()
	}

	// Add or update the worktree group workspace
	wsName := ws.Name
	delete(wsCfg.Workspaces, wsName) // Remove if exists
	if err := wsCfg.AddWorkspace(*ws); err != nil {
		return fmt.Errorf("failed to add workspace: %w", err)
	}
	if err := config.SaveWorkspaceConfig(wsCfg); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	fmt.Printf("Worktree group watch: %s\n", wsName)
	fmt.Printf("Worktrees: %d\n", len(cfg.Worktrees))
	for _, wt := range cfg.Worktrees {
		fmt.Printf("  - %s: %s\n", wt.Name, wt.Path)
	}
	fmt.Println()

	// Delegate to workspace watch
	background, _ := cmd.Flags().GetBool("background")
	logDirFlag, _ := cmd.Flags().GetString("log-dir")

	// Set workspace flag and invoke watch
	watchWorkspace = wsName
	watchBackground = background
	if logDirFlag != "" {
		watchLogDir = logDirFlag
	}

	return runWatch(cmd, nil)
}
