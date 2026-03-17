package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/update"
)

// postUpdateVersionSync is called on every CLI startup (via PersistentPreRunE).
// It detects whether syncctl was updated since the last run and, if so:
//  1. Probes the version of each companion binary (syncd, synctui).
//  2. Prints a warning for any binary that does not match the CLI version.
//  3. Attempts to bring out-of-sync binaries in line with the CLI version by
//     downloading the matching release artifacts.
//
// Non-fatal: all errors are printed as warnings and execution continues.
// The function also updates the persisted last-version stamp on exit so the
// next invocation knows the current version.
func postUpdateVersionSync(ctx context.Context, w io.Writer, cliVersion, configPath string) {
	// "dev" builds are never stamped; skip the check in development.
	if cliVersion == "" || cliVersion == "dev" {
		return
	}

	dataDir := dataDirectoryForConfig(configPath)

	// Detect whether the CLI version changed since the last run.
	updated, err := update.WasUpdated(dataDir, cliVersion)
	if err != nil {
		// Non-fatal: we simply skip the check this time.
		return
	}

	// Always refresh the stamp so subsequent runs see the correct baseline.
	defer func() {
		_ = update.WriteLastVersion(dataDir, cliVersion)
	}()

	if !updated {
		return
	}

	fmt.Fprintf(w, "info: syncctl updated to %s – checking companion binary versions…\n", cliVersion)

	// Locate the directory that holds sibling binaries (syncd, synctui).
	siblingDir := siblingBinaryDir()

	syncer := update.NewVersionSyncer(cliVersion, siblingDir)

	// Step 1: probe all companions.
	report, err := syncer.Check(ctx)
	if err != nil {
		fmt.Fprintf(w, "warn: version sync check failed: %v\n", err)
		return
	}

	if report.InSync() {
		fmt.Fprintf(w, "info: all companion binaries are already at %s\n", cliVersion)
		return
	}

	// Step 2: report which binaries are out of sync.
	for _, comp := range report.OutOfSync {
		compVer := report.Components[comp]
		if compVer == "" {
			fmt.Fprintf(w, "warn: companion binary %q not found or version unknown (expected %s)\n", comp, cliVersion)
		} else {
			fmt.Fprintf(w, "warn: companion binary %q is at %s, expected %s\n", comp, compVer, cliVersion)
		}
	}

	// Step 3: attempt to sync them using the configured update repo.
	repo := updateRepo(configPath)
	fmt.Fprintf(w, "info: attempting to sync %d companion binary(ies) to %s…\n", len(report.OutOfSync), cliVersion)

	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	results := syncer.Sync(syncCtx, repo)
	if len(results) == 0 {
		fmt.Fprintf(w, "info: no companion binaries required update\n")
		return
	}

	allOK := true
	for comp, syncErr := range results {
		if syncErr != nil {
			allOK = false
			fmt.Fprintf(w, "warn: failed to sync %q to %s: %v\n", comp, cliVersion, syncErr)
		} else {
			fmt.Fprintf(w, "info: synced %q to %s\n", comp, cliVersion)
		}
	}

	if !allOK {
		fmt.Fprintf(w, "hint: run 'syncctl update apply' to retry syncing companion binaries manually\n")
	}
}

// siblingBinaryDir returns the directory that contains the running syncctl
// binary.  Falls back to the current working directory on error.
func siblingBinaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// dataDirectoryForConfig derives the application data directory from the
// config file path.  The data dir sits next to the config directory; we use
// config.DefaultDataDir() for the canonical location.
func dataDirectoryForConfig(_ string) string {
	return config.DefaultDataDir()
}

// updateRepo reads the GitHub repository slug used for update downloads from
// the config.  Falls back to the canonical repository if the config cannot be
// loaded.
func updateRepo(configPath string) string {
	cfg, err := config.Load(configPath)
	if err != nil {
		return defaultUpdateRepo
	}
	_ = cfg // UpdateConfig does not yet expose a repo field; use the default.
	return defaultUpdateRepo
}

// defaultUpdateRepo is the canonical GitHub repository for release artifacts.
const defaultUpdateRepo = "basmulder03/git-project-sync"

// formatVersionSyncStatus returns a short human-readable summary of an out-of-
// sync report, suitable for compact inline display.
func formatVersionSyncStatus(report update.VersionSyncReport) string {
	if report.InSync() {
		return fmt.Sprintf("all binaries at %s", report.CLIVersion)
	}
	parts := make([]string, 0, len(report.OutOfSync))
	for _, comp := range report.OutOfSync {
		ver := report.Components[comp]
		if ver == "" {
			ver = "?"
		}
		parts = append(parts, fmt.Sprintf("%s@%s", comp, ver))
	}
	return fmt.Sprintf("out of sync: %s (expected %s)", strings.Join(parts, ", "), report.CLIVersion)
}
