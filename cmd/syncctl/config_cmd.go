package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/app/commands"
	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func newConfigCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage configuration"}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Show config file path",
			RunE: func(cmd *cobra.Command, _ []string) error {
				cmd.Println(*configPath)
				return nil
			},
		},
		&cobra.Command{
			Use:   "init",
			Short: "Create default config when missing",
			RunE: func(cmd *cobra.Command, _ []string) error {
				if _, err := os.Stat(*configPath); err == nil {
					cmd.Printf("config already exists: %s\n", *configPath)
					return nil
				} else if !os.IsNotExist(err) {
					return err
				}
				if err := config.Save(*configPath, config.Default()); err != nil {
					return err
				}
				cmd.Printf("config created: %s\n", *configPath)
				return nil
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Show resolved config summary",
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg, err := config.Load(*configPath)
				if err != nil {
					return err
				}
				cmd.Printf("schema_version: %d\n", cfg.SchemaVersion)
				cmd.Printf("workspace_root: %s\n", cfg.Workspace.Root)
				cmd.Printf("state_db_path: %s\n", cfg.State.DBPath)
				cmd.Printf("sources: %d\n", len(cfg.Sources))
				cmd.Printf("repos: %d\n", len(cfg.Repos))
				return nil
			},
		},
		&cobra.Command{
			Use:   "validate",
			Short: "Validate current config",
			RunE: func(cmd *cobra.Command, _ []string) error {
				if _, err := config.Load(*configPath); err != nil {
					return err
				}
				cmd.Printf("config valid: %s\n", *configPath)
				return nil
			},
		},
		&cobra.Command{
			Use:   "get <key>",
			Short: "Get a supported config value",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(*configPath)
				if err != nil {
					return err
				}

				value, err := commands.GetConfigValue(cfg, args[0])
				if err != nil {
					return err
				}
				cmd.Println(value)
				return nil
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set a supported config value",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(*configPath)
				if err != nil {
					return err
				}

				if err := commands.SetConfigValue(&cfg, args[0], args[1]); err != nil {
					return err
				}
				if err := config.Save(*configPath, cfg); err != nil {
					return err
				}

				cmd.Printf("updated %s\n", args[0])
				return nil
			},
		},
	)

	return cmd
}
