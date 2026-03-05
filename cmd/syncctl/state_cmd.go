package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func newStateCommand(configPath *string) *cobra.Command {
	stateCmd := &cobra.Command{
		Use:   "state",
		Short: "State DB backup, restore, and integrity checks",
	}

	stateCmd.AddCommand(
		newStateCheckCommand(configPath),
		newStateBackupCommand(configPath),
		newStateRestoreCommand(configPath),
	)

	return stateCmd
}

func newStateCheckCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run sqlite integrity check",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			if _, err := os.Stat(cfg.State.DBPath); err != nil {
				return fmt.Errorf("state db unavailable: %w", err)
			}

			store, err := state.NewSQLiteStore(cfg.State.DBPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			if err := store.IntegrityCheck(); err != nil {
				return err
			}
			cmd.Printf("integrity: ok (%s)\n", cfg.State.DBPath)
			return nil
		},
	}
}

func newStateBackupCommand(configPath *string) *cobra.Command {
	var outputPath string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a consistent backup of the state DB",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if outputPath == "" {
				return fmt.Errorf("required flag: --output")
			}

			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			store, err := state.NewSQLiteStore(cfg.State.DBPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			if err := store.BackupTo(outputPath, overwrite); err != nil {
				return err
			}
			cmd.Printf("state backup created: %s\n", outputPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "Backup sqlite file path")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing backup file")
	return cmd
}

func newStateRestoreCommand(configPath *string) *cobra.Command {
	var inputPath string

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore state DB from a backup file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if inputPath == "" {
				return fmt.Errorf("required flag: --input")
			}

			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			if err := state.RestoreSQLiteDB(cfg.State.DBPath, inputPath); err != nil {
				return err
			}

			store, err := state.NewSQLiteStore(cfg.State.DBPath)
			if err != nil {
				return fmt.Errorf("open restored db: %w", err)
			}
			defer func() { _ = store.Close() }()

			if err := store.IntegrityCheck(); err != nil {
				return fmt.Errorf("restored db failed integrity check: %w", err)
			}

			cmd.Printf("state backup restored: %s -> %s\n", inputPath, cfg.State.DBPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&inputPath, "input", "", "Input backup sqlite file path")
	return cmd
}
