package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/auth"
	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/logging"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func newDiscoverCommand(configPath *string) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover and clone remote repositories",
		Long: `Queries provider APIs to discover accessible repositories and clones missing ones.

This command respects the governance configuration (include/exclude patterns) and
auto-clone settings (max_size, include_archived) from your config file.

By default, repositories are cloned into the workspace layout specified in config.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			// Check if auto-clone is enabled
			if cfg.Governance.DefaultPolicy.AutoCloneEnabled != nil && !*cfg.Governance.DefaultPolicy.AutoCloneEnabled {
				cmd.Println("auto-clone is disabled in configuration")
				return nil
			}

			// Initialize logger
			logger, err := logging.New(logging.Options{
				Level:  cfg.Logging.Level,
				Format: cfg.Logging.Format,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}

			// Initialize token store for API authentication
			secretsPath := filepath.Join(filepath.Dir(*configPath), "secrets", "tokens.enc")
			tokenStore, err := auth.NewTokenStore(auth.Options{
				ServiceName:    "git-project-sync",
				FallbackPath:   secretsPath,
				FallbackKeyEnv: "GIT_PROJECT_SYNC_FALLBACK_KEY",
			})
			if err != nil {
				return fmt.Errorf("failed to initialize token store: %w", err)
			}

			// Load state store directly (orchestrator needs the Store interface)
			cfg2, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			stateStore, err := state.NewSQLiteStore(cfg2.State.DBPath)
			if err != nil {
				return fmt.Errorf("failed to initialize state store: %w", err)
			}
			defer func() { _ = stateStore.Close() }()

			// Create orchestrator
			orchestrator := daemon.NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, stateStore)

			// Run discovery
			traceID := fmt.Sprintf("discover-%d", time.Now().UTC().UnixNano())
			cmd.Printf("starting discovery (trace_id=%s)\n", traceID)

			if dryRun {
				cmd.Println("dry-run mode: discovery would run but no repos would be cloned")
				// TODO: Add dry-run support to orchestrator
				return fmt.Errorf("dry-run mode not yet implemented for discover command")
			}

			if err := orchestrator.Run(context.Background(), traceID); err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			cmd.Println("discovery completed successfully")
			cmd.Printf("check events with: syncctl trace show %s\n", traceID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be discovered and cloned")
	return cmd
}
