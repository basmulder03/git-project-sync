package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
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
				return fmt.Errorf("required flag: --manifest-url")
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
	var binaryPath string
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Download, verify, and apply update artifact",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if manifestURL == "" {
				return fmt.Errorf("required flag: --manifest-url")
			}

			target := binaryPath
			if target == "" {
				target = outputPath
			}
			if target == "" {
				target = filepath.Join(".", "syncctl")
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

			api, cleanup, _ := loadUpdateRecorder(*configPath)
			defer cleanup()
			traceID := fmt.Sprintf("update-%d", time.Now().UTC().UnixNano())
			recordUpdateEvent(api, traceID, "info", "update_started", "update apply started")

			applyResult, err := updater.Apply(context.Background(), result.Artifact, target, result.Manifest.Version)
			if err != nil {
				if applyErr, ok := err.(update.ApplyError); ok && applyErr.RollbackPerformed {
					recordUpdateEvent(api, traceID, "warn", "update_rollback", "update failed and rollback completed")
				}
				recordUpdateEvent(api, traceID, "error", "update_failed", err.Error())
				return err
			}

			recordUpdateEvent(api, traceID, "info", "update_succeeded", "update applied successfully")

			cmd.Printf("applied verified update %s to %s\n", applyResult.Version, applyResult.TargetPath)
			return nil
		},
	}

	for _, command := range []*cobra.Command{checkCmd, applyCmd} {
		command.Flags().StringVar(&manifestURL, "manifest-url", "", "Update manifest URL")
		command.Flags().StringVar(&channel, "channel", "stable", "Update channel")
		command.Flags().StringVar(&currentVersion, "current-version", "dev", "Current binary version")
	}
	applyCmd.Flags().StringVar(&outputPath, "output", "", "Output artifact path")
	applyCmd.Flags().StringVar(&binaryPath, "binary-path", "", "Target binary path for atomic replacement")

	updateCmd.AddCommand(checkCmd, applyCmd)
	return updateCmd
}

func loadUpdateRecorder(configPath string) (*daemon.ServiceAPI, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, func() {}, err
	}

	store, err := state.NewSQLiteStore(cfg.State.DBPath)
	if err != nil {
		return nil, func() {}, err
	}

	return daemon.NewServiceAPI(store), func() { _ = store.Close() }, nil
}

func recordUpdateEvent(api *daemon.ServiceAPI, traceID, level, reasonCode, message string) {
	if api == nil {
		return
	}
	_ = api.RecordEvent(context.Background(), telemetry.Event{
		TraceID:    traceID,
		RepoPath:   "update",
		Level:      level,
		ReasonCode: reasonCode,
		Message:    message,
		CreatedAt:  time.Now().UTC(),
	})
}
