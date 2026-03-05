package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, formatCLIError(err))
		os.Exit(classifyExitCode(err))
	}
}

func newRootCommand() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   "syncctl",
		Short: "Control git-project-sync",
	}
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.CompletionOptions.DisableDefaultCmd = true

	root.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath(), "Path to config file")

	root.Version = version
	root.SetVersionTemplate("syncctl {{.Version}}\n")

	root.AddCommand(
		newDoctorCommand(&configPath),
		newSourceCommand(&configPath),
		newRepoCommand(&configPath),
		newWorkspaceCommand(&configPath),
		newSyncCommand(&configPath),
		newStubCommand("daemon", "Control the daemon"),
		newConfigCommand(&configPath),
		newAuthCommand(&configPath),
		newCacheCommand(&configPath),
		newStatsCommand(&configPath),
		newEventsCommand(&configPath),
		newTraceCommand(&configPath),
		newStateCommand(&configPath),
		newStubCommand("install", "Install and register services"),
		newStubCommand("service", "Service registration controls"),
		newUpdateCommand(&configPath),
	)

	return root
}

func formatCLIError(err error) string {
	if err == nil {
		return ""
	}
	return "error: " + strings.TrimSpace(err.Error())
}

func classifyExitCode(err error) int {
	if err == nil {
		return 0
	}
	msg := strings.ToLower(err.Error())
	usageHints := []string{
		"unknown command",
		"unknown shorthand flag",
		"unknown flag",
		"required flag",
		"required argument",
		"accepts ",
		"invalid argument",
		"invalid value",
		"help requested",
	}
	for _, hint := range usageHints {
		if strings.Contains(msg, hint) {
			return 2
		}
	}

	var targetError interface{ Unwrap() error }
	if errors.As(err, &targetError) {
		return classifyExitCode(targetError.Unwrap())
	}
	return 1
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
