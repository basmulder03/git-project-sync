package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	var syncdPath string
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Download, verify, and apply update for all binaries (syncctl + syncd)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolve the syncctl target binary path.
			syncctlTarget := binaryPath
			if syncctlTarget == "" {
				syncctlTarget = outputPath
			}
			if syncctlTarget == "" {
				execPath, err := os.Executable()
				if err == nil {
					syncctlTarget = execPath
				} else {
					syncctlTarget = filepath.Join(".", syncctlBinaryName())
				}
			}

			// Resolve the syncd target binary path.
			// Default: sibling of syncctl in the same directory.
			syncdTarget := syncdPath
			if syncdTarget == "" {
				syncdTarget = filepath.Join(filepath.Dir(syncctlTarget), syncdBinaryName())
			}

			updater := update.NewUpdater(currentVersion)

			var manifest update.Manifest
			var targetVersion string

			if strings.TrimSpace(manifestURL) != "" {
				result, err := updater.Check(context.Background(), manifestURL, channel)
				if err != nil {
					return err
				}
				if !result.Available {
					cmd.Printf("no update available (current=%s latest=%s channel=%s)\n", currentVersion, result.Manifest.Version, result.Manifest.Channel)
					return nil
				}
				manifest = result.Manifest
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
				manifest = selected.Manifest
				targetVersion = selected.Version
			}

			api, cleanup, _ := loadUpdateRecorder(*configPath)
			defer cleanup()
			traceID := fmt.Sprintf("update-%d", time.Now().UTC().UnixNano())
			recordUpdateEvent(api, traceID, "info", "update_started", "update apply started")

			// Build the component → target-path map.
			// We only include syncd if the binary currently exists on disk;
			// this avoids failing an update just because syncd was never
			// installed in this particular environment.
			componentPaths := map[string]string{
				"syncctl": syncctlTarget,
			}
			if _, err := os.Stat(syncdTarget); err == nil {
				componentPaths["syncd"] = syncdTarget
			}

			results := updater.ApplyAll(context.Background(), manifest, componentPaths, targetVersion)

			anyErr := false
			for _, r := range results {
				if r.Err != nil {
					anyErr = true
					if applyErr, ok := r.Err.(update.ApplyError); ok && applyErr.RollbackPerformed {
						recordUpdateEvent(api, traceID, "warn", "update_rollback",
							fmt.Sprintf("component %s update failed and rollback completed", r.Component))
						cmd.Printf("rollback performed for %s: %v\n", r.Component, r.Err)
					} else {
						recordUpdateEvent(api, traceID, "error", "update_failed",
							fmt.Sprintf("component %s: %v", r.Component, r.Err))
						cmd.Printf("error updating %s: %v\n", r.Component, r.Err)
					}
				} else {
					cmd.Printf("applied verified update %s to %s\n", r.Version, r.TargetPath)
				}
			}

			if len(results) == 0 {
				cmd.Printf("no matching artifacts found in manifest for this platform\n")
			}

			if anyErr {
				recordUpdateEvent(api, traceID, "error", "update_failed", "one or more component updates failed")
				return fmt.Errorf("one or more component updates failed")
			}

			recordUpdateEvent(api, traceID, "info", "update_succeeded", "all component updates applied successfully")
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
	applyCmd.Flags().StringVar(&outputPath, "output", "", "Output path for the syncctl artifact (deprecated, use --binary-path)")
	applyCmd.Flags().StringVar(&binaryPath, "binary-path", "", "Target path for syncctl binary (defaults to the running executable)")
	applyCmd.Flags().StringVar(&syncdPath, "syncd-path", "", "Target path for syncd binary (defaults to syncd sibling of syncctl)")
	applyCmd.Flags().StringVar(&desiredVersion, "version", "", "Specific target version to apply")

	updateCmd.AddCommand(checkCmd, applyCmd)
	return updateCmd
}

// syncctlBinaryName returns the OS-appropriate name for the syncctl binary.
func syncctlBinaryName() string {
	if runtime.GOOS == "windows" {
		return "syncctl.exe"
	}
	return "syncctl"
}

// syncdBinaryName returns the OS-appropriate name for the syncd binary.
func syncdBinaryName() string {
	if runtime.GOOS == "windows" {
		return "syncd.exe"
	}
	return "syncd"
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
