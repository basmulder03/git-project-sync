package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
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

	// Print a one-time hint when SSH is enabled but the user hasn't yet
	// decided about the SSH remote migration.  Informational only; does not
	// block any command.
	// Also runs a post-update version sync check when the CLI was just updated.
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Only hint in interactive sessions (not when piped/scripted).
		fi, err := os.Stdout.Stat()
		isTTY := err == nil && (fi.Mode()&os.ModeCharDevice) != 0
		if !isTTY {
			return nil
		}
		// Skip startup checks for the migration command itself and the update
		// apply command (which replaces binaries and therefore runs in the
		// middle of the sync process).
		cmdPath := cmd.CommandPath()
		if cmdPath == "syncctl auth ssh migrate" || cmdPath == "syncctl update apply" {
			return nil
		}

		// Post-update companion version sync.  Runs only when syncctl was
		// updated since the last invocation; non-fatal in all cases.
		postUpdateVersionSync(cmd.Context(), os.Stderr, version, configPath)

		cfg, cfgErr := config.Load(configPath)
		if cfgErr != nil {
			return nil // config may not exist yet; don't block
		}
		if cfg.SSH.Enabled && cfg.SSH.MigrationOptIn == "" {
			fmt.Fprintln(os.Stderr, "hint: SSH is enabled. Run 'syncctl auth ssh migrate' to switch existing repos from HTTPS to SSH remotes.")
		}
		return nil
	}

	root.Version = version
	root.SetVersionTemplate("syncctl {{.Version}}\n")

	root.AddCommand(
		newDoctorCommand(&configPath),
		newSourceCommand(&configPath),
		newRepoCommand(&configPath),
		newWorkspaceCommand(&configPath),
		newSyncCommand(&configPath),
		newDiscoverCommand(&configPath),
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
