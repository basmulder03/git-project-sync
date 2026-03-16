package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newInstallCommand(configPath *string) *cobra.Command {
	_ = configPath

	var userFlag bool
	var systemFlag bool
	var binaryPath string
	var explicitConfigPath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and register service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// On Windows, service creation always requires Administrator. Try to
			// auto-elevate before doing anything else.
			if relaunched, elErr := tryElevateForInstall(); elErr != nil {
				return fmt.Errorf("elevation failed: %w", elErr)
			} else if relaunched {
				os.Exit(0)
			}

			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}

			resolvedBinaryPath, resolvedConfigPath, err := resolveInstallPaths(mode, binaryPath, explicitConfigPath)
			if err != nil {
				return err
			}

			if err := ensureConfigExists(resolvedConfigPath); err != nil {
				return fmt.Errorf("ensure config: %w", err)
			}

			installer, err := newServiceInstaller(resolvedBinaryPath, resolvedConfigPath)
			if err != nil {
				return err
			}

			if err := installer.Install(mode); err != nil {
				return err
			}

			cmd.Printf("installed service in %s mode\n", mode)
			cmd.Printf("binary_path: %s\n", resolvedBinaryPath)
			cmd.Printf("config_path: %s\n", resolvedConfigPath)
			return nil
		},
	}

	addInstallCommonFlags(cmd, &userFlag, &systemFlag, &binaryPath, &explicitConfigPath)
	return cmd
}

func newUninstallCommand(configPath *string) *cobra.Command {
	_ = configPath

	var userFlag bool
	var systemFlag bool
	var binaryPath string
	var explicitConfigPath string

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Unregister service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Unregistering a Windows Service also needs Administrator.
			if relaunched, elErr := tryElevateForInstall(); elErr != nil {
				return fmt.Errorf("elevation failed: %w", elErr)
			} else if relaunched {
				os.Exit(0)
			}

			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}

			resolvedBinaryPath, resolvedConfigPath, err := resolveInstallPaths(mode, binaryPath, explicitConfigPath)
			if err != nil {
				return err
			}

			installer, err := newServiceInstaller(resolvedBinaryPath, resolvedConfigPath)
			if err != nil {
				return err
			}

			if err := installer.Uninstall(mode); err != nil {
				return err
			}

			cmd.Printf("uninstalled service in %s mode\n", mode)
			return nil
		},
	}

	addInstallCommonFlags(cmd, &userFlag, &systemFlag, &binaryPath, &explicitConfigPath)
	return cmd
}

func addInstallCommonFlags(cmd *cobra.Command, userFlag, systemFlag *bool, binaryPath, configPath *string) {
	cmd.Flags().BoolVar(userFlag, "user", false, "Use user mode (default)")
	cmd.Flags().BoolVar(systemFlag, "system", false, "Use system mode")
	cmd.Flags().StringVar(binaryPath, "binary-path", "", "Path to syncd binary")
	cmd.Flags().StringVar(configPath, "config-path", "", "Path to config file")
}
