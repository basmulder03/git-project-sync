package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	var repo string
	var channel string
	var includePrerelease bool
	var currentVersion string

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Check and apply updates",
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Check for updates",
		RunE: func(cmd *cobra.Command, _ []string) error {
			updater := update.NewUpdater(currentVersion)
			return runUpdateCheck(cmd, updater, manifestURL, repo, channel, includePrerelease)
		},
	}

	var outputPath string
	var binaryPath string
	var desiredVersion string
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Download, verify, and apply update artifact",
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := binaryPath
			if target == "" {
				target = outputPath
			}
			if target == "" {
				execPath, err := os.Executable()
				if err == nil {
					target = execPath
				} else {
					target = filepath.Join(".", "syncctl")
				}
			}

			updater := update.NewUpdater(currentVersion)

			artifact := update.Artifact{}
			targetVersion := ""
			if strings.TrimSpace(manifestURL) != "" {
				result, err := updater.Check(context.Background(), manifestURL, channel)
				if err != nil {
					return err
				}
				if !result.Available {
					cmd.Printf("no update available (current=%s latest=%s channel=%s)\n", currentVersion, result.Manifest.Version, result.Manifest.Channel)
					return nil
				}
				artifact = result.Artifact
				targetVersion = result.Manifest.Version
			} else {
				candidates, err := updater.ListCandidates(context.Background(), repo, channel, includePrerelease)
				if err != nil {
					return err
				}
				newer := updater.FilterNewer(candidates)
				if len(newer) == 0 {
					cmd.Printf("no update available (current=%s channel=%s)\n", currentVersion, channel)
					return nil
				}

				cmd.Printf("available versions:\n")
				for _, candidate := range newer {
					tag := "stable"
					if candidate.Prerelease {
						tag = "prerelease"
					}
					cmd.Printf("- %s (%s)\n", candidate.Version, tag)
				}

				selected, err := updater.SelectCandidate(newer, desiredVersion)
				if err != nil {
					return err
				}
				if strings.TrimSpace(desiredVersion) == "" {
					cmd.Printf("applying latest available version %s (use --version to choose a specific version)\n", selected.Version)
				}
				artifact = selected.Artifact
				targetVersion = selected.Version
			}

			api, cleanup, _ := loadUpdateRecorder(*configPath)
			defer cleanup()
			traceID := fmt.Sprintf("update-%d", time.Now().UTC().UnixNano())
			recordUpdateEvent(api, traceID, "info", "update_started", "update apply started")

			applyResult, err := updater.Apply(context.Background(), artifact, target, targetVersion)
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
		command.Flags().StringVar(&manifestURL, "manifest-url", "", "Update manifest URL override")
		command.Flags().StringVar(&repo, "repo", "basmulder03/git-project-sync", "GitHub repository owner/name for release lookup")
		command.Flags().StringVar(&channel, "channel", "stable", "Update channel")
		command.Flags().BoolVar(&includePrerelease, "include-prerelease", false, "Include prerelease versions")
		command.Flags().StringVar(&currentVersion, "current-version", version, "Current binary version")
	}
	applyCmd.Flags().StringVar(&outputPath, "output", "", "Output artifact path")
	applyCmd.Flags().StringVar(&binaryPath, "binary-path", "", "Target binary path for atomic replacement")
	applyCmd.Flags().StringVar(&desiredVersion, "version", "", "Specific target version to apply")

	updateCmd.AddCommand(checkCmd, applyCmd)
	return updateCmd
}

func runUpdateCheck(cmd *cobra.Command, updater *update.Updater, manifestURL, repo, channel string, includePrerelease bool) error {
	if strings.TrimSpace(manifestURL) != "" {
		result, err := updater.Check(context.Background(), manifestURL, channel)
		if err != nil {
			return err
		}
		if !result.Available {
			cmd.Printf("no update available (current=%s latest=%s channel=%s)\n", updater.CurrentVersion, result.Manifest.Version, result.Manifest.Channel)
			return nil
		}
		cmd.Printf("update available: %s -> %s\n", updater.CurrentVersion, result.Manifest.Version)
		cmd.Printf("artifact: %s/%s %s\n", result.Artifact.OS, result.Artifact.Arch, result.Artifact.URL)
		return nil
	}

	candidates, err := updater.ListCandidates(context.Background(), repo, channel, includePrerelease)
	if err != nil {
		return err
	}
	newer := updater.FilterNewer(candidates)
	if len(newer) == 0 {
		cmd.Printf("no update available (current=%s channel=%s)\n", updater.CurrentVersion, channel)
		return nil
	}

	cmd.Printf("updates available for %s:\n", updater.CurrentVersion)
	for _, candidate := range newer {
		tag := "stable"
		if candidate.Prerelease {
			tag = "prerelease"
		}
		cmd.Printf("- %s (%s) %s\n", candidate.Version, tag, candidate.Artifact.URL)
	}
	cmd.Printf("latest: %s\n", newer[0].Version)
	return nil
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
