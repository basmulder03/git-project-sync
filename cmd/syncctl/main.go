package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   "syncctl",
		Short: "Control git-project-sync",
	}

	root.PersistentFlags().StringVar(&configPath, "config", "configs/config.example.yaml", "Path to config file")

	root.Version = "dev"
	root.SetVersionTemplate("syncctl {{.Version}}\n")

	root.AddCommand(
		newDoctorCommand(),
		newSourceCommand(&configPath),
		newRepoCommand(&configPath),
		newWorkspaceCommand(&configPath),
		newSyncCommand(&configPath),
		newStubCommand("daemon", "Control the daemon"),
		newStubCommand("config", "Manage configuration"),
		newAuthCommand(&configPath),
		newCacheCommand(&configPath),
		newStubCommand("stats", "View runtime stats"),
		newEventsCommand(&configPath),
		newTraceCommand(&configPath),
		newStubCommand("install", "Install and register services"),
		newStubCommand("service", "Service registration controls"),
		newStubCommand("update", "Check and apply updates"),
	)

	return root
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostics",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println("doctor checks are not implemented yet")
		},
	}
}

func newStubCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Printf("%s command group is not implemented yet\n", use)
		},
	}
}
