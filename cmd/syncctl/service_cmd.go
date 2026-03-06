package main

import (
	"github.com/spf13/cobra"
)

func newServiceCommand(configPath *string) *cobra.Command {
	_ = configPath

	var userFlag bool
	var systemFlag bool
	var binaryPath string
	var explicitConfigPath string

	cmd := &cobra.Command{
		Use:   "service",
		Short: "Service registration controls",
	}

	registerCmd := &cobra.Command{
		Use:   "register",
		Short: "Register and start service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, err := resolveInstallMode(userFlag, systemFlag)
			if err != nil {
				return err
			}

			resolvedBinaryPath, resolvedConfigPath, err := resolveInstallPaths(mode, binaryPath, explicitConfigPath)
			if err != nil {
				return err
			}
			if err := ensureConfigExists(resolvedConfigPath); err != nil {
				return err
			}

			installer, err := newServiceInstaller(resolvedBinaryPath, resolvedConfigPath)
			if err != nil {
				return err
			}
			if err := installer.Install(mode); err != nil {
				return err
			}

			cmd.Printf("service registered in %s mode\n", mode)
			return nil
		},
	}

	unregisterCmd := &cobra.Command{
		Use:   "unregister",
		Short: "Unregister and stop service",
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			cmd.Printf("service unregistered in %s mode\n", mode)
			return nil
		},
	}

	addInstallCommonFlags(registerCmd, &userFlag, &systemFlag, &binaryPath, &explicitConfigPath)
	addInstallCommonFlags(unregisterCmd, &userFlag, &systemFlag, &binaryPath, &explicitConfigPath)
	cmd.AddCommand(registerCmd, unregisterCmd)

	return cmd
}
