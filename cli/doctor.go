package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
	"github.com/yoanbernabeu/grepai/git"
	"github.com/yoanbernabeu/grepai/store"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose grepai installation and project health",
	Long: `Run diagnostic checks on the current grepai installation and project.

Checks performed:
  - Git repository detection and worktree status
  - grepai configuration (.grepai/config.yaml)
  - Index health (exists, size, freshness)
  - Daemon status (running, stale PID)
  - Worktree group configuration
  - Backend connectivity (if applicable)`,
	RunE: runDoctor,
}

func init() {
	// Will be registered in root.go
}

type checkResult struct {
	name   string
	status string // "ok", "warn", "fail", "info"
	detail string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var results []checkResult

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	fmt.Println("grepai doctor - Diagnostic Report")
	fmt.Println("==================================")
	fmt.Println()

	// 1. Git repository check
	results = append(results, checkGit(cwd)...)

	// 2. grepai config check
	results = append(results, checkConfig(cwd)...)

	// 3. Index health check
	results = append(results, checkIndex(cwd)...)

	// 4. Daemon status check
	results = append(results, checkDaemon(cwd)...)

	// 5. Worktree group check
	results = append(results, checkWorktreeGroup(cwd)...)

	// Print results
	var okCount, warnCount, failCount int
	for _, r := range results {
		icon := "  "
		switch r.status {
		case "ok":
			icon = "[OK]  "
			okCount++
		case "warn":
			icon = "[WARN]"
			warnCount++
		case "fail":
			icon = "[FAIL]"
			failCount++
		case "info":
			icon = "[INFO]"
		}
		fmt.Printf("  %s %s", icon, r.name)
		if r.detail != "" {
			fmt.Printf(": %s", r.detail)
		}
		fmt.Println()
	}

	fmt.Println()
	fmt.Printf("Summary: %d ok, %d warnings, %d failures\n", okCount, warnCount, failCount)

	if failCount > 0 {
		fmt.Println("\nRun 'grepai init' to set up a project, or 'grepai watch' to start indexing.")
	}

	return nil
}

func checkGit(cwd string) []checkResult {
	var results []checkResult

	gitInfo, err := git.Detect(cwd)
	if err != nil {
		results = append(results, checkResult{
			name:   "Git repository",
			status: "info",
			detail: "not a git repository",
		})
		return results
	}

	results = append(results, checkResult{
		name:   "Git repository",
		status: "ok",
		detail: gitInfo.GitRoot,
	})

	if gitInfo.IsWorktree {
		results = append(results, checkResult{
			name:   "Git worktree",
			status: "info",
			detail: fmt.Sprintf("linked worktree (main: %s, ID: %s)", gitInfo.MainWorktree, gitInfo.WorktreeID),
		})
	} else {
		results = append(results, checkResult{
			name:   "Git worktree",
			status: "info",
			detail: fmt.Sprintf("main worktree (ID: %s)", gitInfo.WorktreeID),
		})
	}

	return results
}

func checkConfig(cwd string) []checkResult {
	var results []checkResult

	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		results = append(results, checkResult{
			name:   "grepai config",
			status: "fail",
			detail: "no .grepai/config.yaml found (run 'grepai init')",
		})
		return results
	}

	results = append(results, checkResult{
		name:   "grepai config",
		status: "ok",
		detail: projectRoot,
	})

	// Load and check config
	cfg, err := config.Load(projectRoot)
	if err != nil {
		results = append(results, checkResult{
			name:   "Config validity",
			status: "fail",
			detail: err.Error(),
		})
		return results
	}

	results = append(results, checkResult{
		name:   "Backend",
		status: "ok",
		detail: cfg.Store.Backend,
	})

	results = append(results, checkResult{
		name:   "Embedder",
		status: "ok",
		detail: fmt.Sprintf("%s (%s)", cfg.Embedder.Provider, cfg.Embedder.Model),
	})

	return results
}

func checkIndex(cwd string) []checkResult {
	var results []checkResult

	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return results // Skip if no project
	}

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return results
	}

	if cfg.Store.Backend == "gob" {
		indexPath := config.GetIndexPath(projectRoot)
		info, err := os.Stat(indexPath)
		if err != nil {
			if os.IsNotExist(err) {
				results = append(results, checkResult{
					name:   "Index file",
					status: "warn",
					detail: "not found (run 'grepai watch' to create)",
				})
			} else {
				results = append(results, checkResult{
					name:   "Index file",
					status: "fail",
					detail: err.Error(),
				})
			}
			return results
		}

		results = append(results, checkResult{
			name:   "Index file",
			status: "ok",
			detail: fmt.Sprintf("%s (%.1f MB)", indexPath, float64(info.Size())/(1024*1024)),
		})

		// Check if index is readable
		gobStore := store.NewGOBStore(indexPath)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := gobStore.Load(ctx); err != nil {
			results = append(results, checkResult{
				name:   "Index integrity",
				status: "fail",
				detail: fmt.Sprintf("corrupt or unreadable: %v", err),
			})
		} else {
			stats, _ := gobStore.GetStats(ctx)
			if stats != nil {
				results = append(results, checkResult{
					name:   "Index integrity",
					status: "ok",
					detail: fmt.Sprintf("%d files, %d chunks", stats.TotalFiles, stats.TotalChunks),
				})

				// Check freshness
				if !stats.LastUpdated.IsZero() {
					age := time.Since(stats.LastUpdated)
					if age > 24*time.Hour {
						results = append(results, checkResult{
							name:   "Index freshness",
							status: "warn",
							detail: fmt.Sprintf("last updated %s ago", age.Round(time.Hour)),
						})
					} else {
						results = append(results, checkResult{
							name:   "Index freshness",
							status: "ok",
							detail: fmt.Sprintf("last updated %s ago", age.Round(time.Minute)),
						})
					}
				}
			}
			gobStore.Close()
		}

		// Check for stale lock file
		lockPath := indexPath + ".lock"
		if _, err := os.Stat(lockPath); err == nil {
			results = append(results, checkResult{
				name:   "Index lock file",
				status: "info",
				detail: "present (normal if watcher is running)",
			})
		}
	} else {
		results = append(results, checkResult{
			name:   "Index",
			status: "info",
			detail: fmt.Sprintf("using %s backend (remote storage)", cfg.Store.Backend),
		})
	}

	// Check symbol index
	symbolPath := config.GetSymbolIndexPath(projectRoot)
	if _, err := os.Stat(symbolPath); err == nil {
		results = append(results, checkResult{
			name:   "Symbol index",
			status: "ok",
			detail: symbolPath,
		})
	} else {
		results = append(results, checkResult{
			name:   "Symbol index",
			status: "warn",
			detail: "not found (built during 'grepai watch')",
		})
	}

	return results
}

func checkDaemon(cwd string) []checkResult {
	var results []checkResult

	logDir, err := daemon.GetDefaultLogDir()
	if err != nil {
		results = append(results, checkResult{
			name:   "Log directory",
			status: "warn",
			detail: err.Error(),
		})
		return results
	}

	// Check regular PID
	pid, _ := daemon.GetRunningPID(logDir)
	if pid > 0 {
		results = append(results, checkResult{
			name:   "Watch daemon",
			status: "ok",
			detail: fmt.Sprintf("running (PID %d)", pid),
		})
	} else {
		// Check for stale PID file
		stalePID, _ := daemon.ReadPIDFile(logDir)
		if stalePID > 0 && !daemon.IsProcessRunning(stalePID) {
			results = append(results, checkResult{
				name:   "Watch daemon",
				status: "warn",
				detail: fmt.Sprintf("stale PID file (PID %d not running)", stalePID),
			})
		} else {
			results = append(results, checkResult{
				name:   "Watch daemon",
				status: "info",
				detail: "not running",
			})
		}
	}

	// Check worktree-specific daemon
	gitInfo, gitErr := git.Detect(cwd)
	if gitErr == nil && gitInfo.WorktreeID != "" {
		wtPID, _ := daemon.GetRunningWorktreePID(logDir, gitInfo.WorktreeID)
		if wtPID > 0 {
			results = append(results, checkResult{
				name:   "Worktree daemon",
				status: "ok",
				detail: fmt.Sprintf("running (PID %d, ID: %s)", wtPID, gitInfo.WorktreeID),
			})
		}
	}

	// Check log directory
	logPath := filepath.Join(logDir, "grepai-watch.log")
	if info, err := os.Stat(logPath); err == nil {
		results = append(results, checkResult{
			name:   "Watch log",
			status: "ok",
			detail: fmt.Sprintf("%s (%.1f KB)", logPath, float64(info.Size())/1024),
		})
	}

	return results
}

func checkWorktreeGroup(cwd string) []checkResult {
	var results []checkResult

	gitCommonDir, cfg, err := config.FindWorktreeGroupForPath(cwd)
	if err != nil {
		results = append(results, checkResult{
			name:   "Worktree group",
			status: "warn",
			detail: err.Error(),
		})
		return results
	}

	if cfg == nil {
		results = append(results, checkResult{
			name:   "Worktree group",
			status: "info",
			detail: "not configured (use 'grepai worktree init' to set up)",
		})
		return results
	}

	results = append(results, checkResult{
		name:   "Worktree group",
		status: "ok",
		detail: fmt.Sprintf("ID: %s, %d worktrees", cfg.GroupID, len(cfg.Worktrees)),
	})

	// Check each worktree path
	for _, wt := range cfg.Worktrees {
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			results = append(results, checkResult{
				name:   fmt.Sprintf("Worktree '%s'", wt.Name),
				status: "warn",
				detail: fmt.Sprintf("path not found: %s", wt.Path),
			})
		}
	}

	_ = gitCommonDir

	return results
}
