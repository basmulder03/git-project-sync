package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func newSyncCommand(configPath *string) *cobra.Command {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Trigger sync operations",
	}

	var allDryRun bool
	allCmd := &cobra.Command{
		Use:   "all",
		Short: "Run one-shot sync for all enabled repos",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			runCount := 0
			for _, repo := range cfg.Repos {
				if !repo.Enabled {
					continue
				}
				runCount++
				if err := runRepoSync(cmd, cfg, repo, allDryRun); err != nil {
					cmd.Printf("error\tpath=%s\terr=%v\n", repo.Path, err)
				}
			}

			if runCount == 0 {
				cmd.Println("no enabled repos configured")
			}
			return nil
		},
	}
	allCmd.Flags().BoolVar(&allDryRun, "dry-run", false, "Preview actions only")

	var repoDryRun bool
	repoCmd := &cobra.Command{
		Use:   "repo <path>",
		Short: "Run one-shot sync for one repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			repo, ok := findRepoByPath(cfg.Repos, args[0])
			if !ok {
				return fmt.Errorf("repo %q not found", args[0])
			}

			return runRepoSync(cmd, cfg, repo, repoDryRun)
		},
	}
	repoCmd.Flags().BoolVar(&repoDryRun, "dry-run", false, "Preview actions only")

	syncCmd.AddCommand(allCmd, repoCmd)
	return syncCmd
}
