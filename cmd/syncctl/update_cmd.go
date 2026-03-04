package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/update"
)

func newUpdateCommand(configPath *string) *cobra.Command {
	_ = configPath

	var manifestURL string
	var channel string
	var currentVersion string

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Check and apply updates",
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Check for updates",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if manifestURL == "" {
				return fmt.Errorf("--manifest-url is required")
			}

			updater := update.NewUpdater(currentVersion)
			result, err := updater.Check(context.Background(), manifestURL, channel)
			if err != nil {
				return err
			}

			if !result.Available {
				cmd.Printf("no update available (current=%s latest=%s channel=%s)\n", currentVersion, result.Manifest.Version, result.Manifest.Channel)
				return nil
			}

			cmd.Printf("update available: %s -> %s\n", currentVersion, result.Manifest.Version)
			cmd.Printf("artifact: %s/%s %s\n", result.Artifact.OS, result.Artifact.Arch, result.Artifact.URL)
			return nil
		},
	}

	var outputPath string
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Download and verify update artifact",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if manifestURL == "" {
				return fmt.Errorf("--manifest-url is required")
			}

			updater := update.NewUpdater(currentVersion)
			result, err := updater.Check(context.Background(), manifestURL, channel)
			if err != nil {
				return err
			}
			if !result.Available {
				cmd.Printf("no update available (current=%s latest=%s channel=%s)\n", currentVersion, result.Manifest.Version, result.Manifest.Channel)
				return nil
			}

			out := outputPath
			if out == "" {
				out = filepath.Join(".", "update-"+result.Manifest.Version)
			}

			if err := updater.DownloadAndVerify(context.Background(), result.Artifact, out); err != nil {
				return err
			}

			cmd.Printf("downloaded verified update artifact to %s\n", out)
			return nil
		},
	}

	for _, command := range []*cobra.Command{checkCmd, applyCmd} {
		command.Flags().StringVar(&manifestURL, "manifest-url", "", "Update manifest URL")
		command.Flags().StringVar(&channel, "channel", "stable", "Update channel")
		command.Flags().StringVar(&currentVersion, "current-version", "dev", "Current binary version")
	}
	applyCmd.Flags().StringVar(&outputPath, "output", "", "Output artifact path")

	updateCmd.AddCommand(checkCmd, applyCmd)
	return updateCmd
}
