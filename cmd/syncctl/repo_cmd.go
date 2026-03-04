package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coregit "github.com/basmulder03/git-project-sync/internal/core/git"
	"github.com/basmulder03/git-project-sync/internal/core/logging"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func newRepoCommand(configPath *string) *cobra.Command {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage repositories",
	}

	repoCmd.AddCommand(
		newRepoAddCommand(configPath),
		newRepoRemoveCommand(configPath),
		newRepoListCommand(configPath),
		newRepoShowCommand(configPath),
		newRepoSyncCommand(configPath),
		newRepoCloneStubCommand(),
	)

	return repoCmd
}

func newRepoAddCommand(configPath *string) *cobra.Command {
	var remote string
	var sourceID string

	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Add repository to config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			for _, repo := range cfg.Repos {
				if repo.Path == args[0] {
					return fmt.Errorf("repo %q already exists", args[0])
				}
			}

			resolvedSourceID, err := resolveSourceID(cfg, sourceID)
			if err != nil {
				return err
			}

			cfg.Repos = append(cfg.Repos, config.RepoConfig{
				Path:                       args[0],
				SourceID:                   resolvedSourceID,
				Remote:                     remote,
				Enabled:                    true,
				Provider:                   "auto",
				CleanupMergedLocalBranches: true,
				SkipIfDirty:                true,
			})

			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("added repo %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&remote, "remote", "origin", "Remote name")
	cmd.Flags().StringVar(&sourceID, "source-id", "", "Source ID (optional if exactly one source exists)")
	return cmd
}

func newRepoCloneStubCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <source-id> <repo-slug>",
		Short: "Clone repository (not implemented yet)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println("repo clone is not implemented yet")
		},
	}
}

func newRepoRemoveCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <path>",
		Short: "Remove repository from config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			updated := make([]config.RepoConfig, 0, len(cfg.Repos))
			removed := false
			for _, repo := range cfg.Repos {
				if repo.Path == args[0] {
					removed = true
					continue
				}
				updated = append(updated, repo)
			}

			if !removed {
				return fmt.Errorf("repo %q not found", args[0])
			}

			cfg.Repos = updated
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("removed repo %s\n", args[0])
			return nil
		},
	}
}

func newRepoListCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List repositories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			if len(cfg.Repos) == 0 {
				cmd.Println("no repos configured")
				return nil
			}

			for _, repo := range cfg.Repos {
				cmd.Printf("%s\tsource=%s\tremote=%s\tenabled=%t\n", repo.Path, repo.SourceID, repo.Remote, repo.Enabled)
			}
			return nil
		},
	}
}

func newRepoShowCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <path>",
		Short: "Show repository details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			for _, repo := range cfg.Repos {
				if repo.Path != args[0] {
					continue
				}

				cmd.Printf("path: %s\n", repo.Path)
				cmd.Printf("source_id: %s\n", repo.SourceID)
				cmd.Printf("remote: %s\n", repo.Remote)
				cmd.Printf("enabled: %t\n", repo.Enabled)
				cmd.Printf("provider: %s\n", repo.Provider)
				cmd.Printf("cleanup_merged_local_branches: %t\n", repo.CleanupMergedLocalBranches)
				cmd.Printf("skip_if_dirty: %t\n", repo.SkipIfDirty)
				return nil
			}

			return fmt.Errorf("repo %q not found", args[0])
		},
	}
}

func newRepoSyncCommand(configPath *string) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync <path>",
		Short: "Run one-shot sync for one repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			for _, repo := range cfg.Repos {
				if repo.Path != args[0] {
					continue
				}
				return runRepoSync(cmd, cfg, repo, dryRun)
			}

			return fmt.Errorf("repo %q not found", args[0])
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview actions only")
	return cmd
}

func runRepoSync(cmd *cobra.Command, cfg config.Config, repo config.RepoConfig, dryRun bool) error {
	logger, err := logging.New(logging.Options{Level: cfg.Logging.Level, Format: cfg.Logging.Format})
	if err != nil {
		return err
	}

	byID := sourceMap(cfg.Sources)
	source, ok := byID[repo.SourceID]
	if !ok {
		return fmt.Errorf("source %q not found or disabled", repo.SourceID)
	}

	engine := coresync.NewEngine(coregit.NewClient(), logger)
	traceID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	result, err := engine.RunRepo(context.Background(), traceID, source, repo, dryRun)
	if err != nil {
		return err
	}

	if result.Skipped {
		cmd.Printf("skipped\treason=%s\t%s\n", result.ReasonCode, result.Reason)
		return nil
	}

	status := "ok"
	if result.Mutated {
		status = "updated"
	}
	cmd.Printf("%s\ttrace=%s\tpath=%s\n", status, traceID, repo.Path)
	return nil
}

func resolveSourceID(cfg config.Config, sourceID string) (string, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID != "" {
		for _, source := range cfg.Sources {
			if source.ID == sourceID {
				return sourceID, nil
			}
		}
		return "", fmt.Errorf("source %q not found", sourceID)
	}

	if len(cfg.Sources) == 1 {
		return cfg.Sources[0].ID, nil
	}

	if len(cfg.Sources) == 0 {
		return "", fmt.Errorf("no sources configured; add one with syncctl source add")
	}

	return "", fmt.Errorf("multiple sources configured; provide --source-id")
}

func findRepoByPath(repos []config.RepoConfig, path string) (config.RepoConfig, bool) {
	for _, repo := range repos {
		if repo.Path == path {
			return repo, true
		}
	}
	return config.RepoConfig{}, false
}
