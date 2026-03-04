package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
)

func newWorkspaceCommand(configPath *string) *cobra.Command {
	workspaceCmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspace settings",
	}

	layoutCmd := &cobra.Command{
		Use:   "layout",
		Short: "Validate and fix managed layout",
	}
	layoutCmd.AddCommand(newWorkspaceLayoutCheckCommand(configPath), newWorkspaceLayoutFixCommand(configPath))

	workspaceCmd.AddCommand(
		newWorkspaceShowCommand(configPath),
		newWorkspaceSetRootCommand(configPath),
		layoutCmd,
	)

	return workspaceCmd
}

func newWorkspaceShowCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show workspace configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			cmd.Printf("root: %s\n", cfg.Workspace.Root)
			cmd.Printf("layout: %s\n", cfg.Workspace.Layout)
			cmd.Printf("create_missing_paths: %t\n", cfg.Workspace.CreateMissingPaths)
			return nil
		},
	}
}

func newWorkspaceSetRootCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "set-root <path>",
		Short: "Set workspace root path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			cfg.Workspace.Root = filepath.Clean(args[0])
			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("workspace root set to %s\n", cfg.Workspace.Root)
			return nil
		},
	}
}

func newWorkspaceLayoutCheckCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check for workspace layout drift",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			drifts, err := checkWorkspaceDrift(cfg)
			if err != nil {
				return err
			}

			if len(drifts) == 0 {
				cmd.Println("workspace layout is clean")
				return nil
			}

			for _, drift := range drifts {
				cmd.Printf("repo=%s expected=%s source=%s reason=%s\n", drift.RepoPath, drift.ExpectedPath, drift.SourceID, drift.ReasonCode)
			}

			return fmt.Errorf("workspace layout drift detected in %d repo(s)", len(drifts))
		},
	}
}

func newWorkspaceLayoutFixCommand(configPath *string) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Fix workspace layout drift",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			drifts, err := checkWorkspaceDrift(cfg)
			if err != nil {
				return err
			}

			if len(drifts) == 0 {
				cmd.Println("workspace layout is clean")
				return nil
			}

			for _, drift := range drifts {
				if drift.ReasonCode == "path_mismatch" {
					cmd.Printf("would set %s -> %s\n", drift.RepoPath, drift.ExpectedPath)
				} else {
					cmd.Printf("skipping %s (%s)\n", drift.RepoPath, drift.ReasonMessage)
				}
			}

			if dryRun {
				return nil
			}

			updated, err := workspace.ApplyPathFixes(&cfg, drifts, cfg.Workspace.CreateMissingPaths)
			if err != nil {
				return err
			}

			if err := config.Save(*configPath, cfg); err != nil {
				return err
			}

			cmd.Printf("updated %d repo path(s)\n", updated)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview path fixes without writing config")
	return cmd
}

func checkWorkspaceDrift(cfg config.Config) ([]workspace.Drift, error) {
	validator, err := workspace.NewValidator(cfg)
	if err != nil {
		return nil, err
	}

	return validator.Check(cfg)
}
