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
		newDaemonCommand(&configPath),
		newConfigCommand(&configPath),
		newAuthCommand(&configPath),
		newCacheCommand(&configPath),
		newStatsCommand(&configPath),
		newEventsCommand(&configPath),
		newTraceCommand(&configPath),
		newStateCommand(&configPath),
		newInstallCommand(&configPath),
		newUninstallCommand(&configPath),
		newServiceCommand(&configPath),
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
