package update

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ComponentVersionFunc is a function that returns the version string for a
// named companion binary. It receives the sibling directory of the running
// syncctl binary.
type ComponentVersionFunc func(siblingDir, component string) (string, error)

// VersionSyncReport holds the result of a cross-binary version consistency
// check.
type VersionSyncReport struct {
	// CLIVersion is the authoritative version (the running syncctl binary).
	CLIVersion string

	// Components maps each companion binary name to its probed version.
	Components map[string]string

	// OutOfSync lists the companion binary names whose version differs from
	// CLIVersion.
	OutOfSync []string
}

// InSync returns true when all probed companions match the CLI version.
func (r VersionSyncReport) InSync() bool {
	return len(r.OutOfSync) == 0
}

// VersionSyncer checks and optionally realigns companion binary versions so
// they all match the running syncctl version.
type VersionSyncer struct {
	// CLIVersion is the authoritative version string (e.g. "v1.2.3") taken from
	// the running syncctl binary.
	CLIVersion string

	// SiblingDir is the directory that contains the companion binaries (syncd,
	// synctui). Defaults to the directory of the running syncctl binary when
	// empty.
	SiblingDir string

	// Components is the set of companion binary names to check.  When nil the
	// default set {"syncd", "synctui"} is used.
	Components []string

	// ProbeVersion is the function used to query a companion binary's version.
	// The default implementation shells out to "<binary> --version" and parses
	// the output.  Override in tests.
	ProbeVersion ComponentVersionFunc

	// Updater is used to apply updates when versions are out of sync. When nil
	// the default updater is constructed from CLIVersion.
	Updater *Updater
}

// NewVersionSyncer creates a VersionSyncer for the given CLI version.
func NewVersionSyncer(cliVersion, siblingDir string) *VersionSyncer {
	return &VersionSyncer{
		CLIVersion: cliVersion,
		SiblingDir: siblingDir,
	}
}

// Check probes all companion binaries and returns a report describing which, if
// any, are running a different version than the CLI.
func (s *VersionSyncer) Check(ctx context.Context) (VersionSyncReport, error) {
	components := s.components()
	siblingDir := s.SiblingDir
	probe := s.probeFunc()

	report := VersionSyncReport{
		CLIVersion: s.CLIVersion,
		Components: make(map[string]string, len(components)),
	}

	for _, comp := range components {
		ver, err := probe(siblingDir, comp)
		if err != nil {
			// Binary not found or not executable: treat as out-of-sync so the
			// caller can decide whether to skip or remediate.
			ver = ""
		}
		report.Components[comp] = ver
		if normalizeVersion(ver) != normalizeVersion(s.CLIVersion) {
			report.OutOfSync = append(report.OutOfSync, comp)
		}
	}

	return report, nil
}

// Sync applies updates to any companion binary that is out of sync with the
// CLI version. It fetches the release manifest that matches CLIVersion from
// GitHub Releases and downloads/verifies the relevant artifacts.
//
// The repo parameter identifies the GitHub repository (owner/name) from which
// to download the matching release artifacts.
//
// Returns a map of component name → error (nil means success). Components that
// were already in sync are not present in the map.
func (s *VersionSyncer) Sync(ctx context.Context, repo string) map[string]error {
	report, err := s.Check(ctx)
	if err != nil || report.InSync() {
		return nil
	}

	updater := s.updater()

	// Look for a release whose manifest version exactly matches CLIVersion.
	// We pass includePrerelease=true so we can also pin to RC releases.
	candidates, err := updater.ListCandidates(ctx, repo, "", true)
	if err != nil {
		results := make(map[string]error, len(report.OutOfSync))
		for _, comp := range report.OutOfSync {
			results[comp] = fmt.Errorf("list release candidates: %w", err)
		}
		return results
	}

	target, err := updater.SelectCandidate(candidates, s.CLIVersion)
	if err != nil {
		results := make(map[string]error, len(report.OutOfSync))
		for _, comp := range report.OutOfSync {
			results[comp] = fmt.Errorf("find release for version %s: %w", s.CLIVersion, err)
		}
		return results
	}

	// Build the component → binary-path map for the out-of-sync set only.
	componentPaths := make(map[string]string, len(report.OutOfSync))
	for _, comp := range report.OutOfSync {
		componentPaths[comp] = filepath.Join(s.SiblingDir, binaryName(comp))
	}

	applyResults := updater.ApplyAll(ctx, target.Manifest, componentPaths, s.CLIVersion)

	results := make(map[string]error, len(applyResults))
	for _, r := range applyResults {
		results[r.Component] = r.Err
	}

	// Components that had no matching artifact in the manifest get a
	// descriptive error rather than silently succeeding.
	for _, comp := range report.OutOfSync {
		if _, seen := results[comp]; !seen {
			results[comp] = fmt.Errorf("no artifact found in manifest for component %q on %s/%s", comp, runtime.GOOS, runtime.GOARCH)
		}
	}

	return results
}

// components returns the list of companion binary names to check, falling back
// to the default set.
func (s *VersionSyncer) components() []string {
	if len(s.Components) > 0 {
		return s.Components
	}
	return []string{"syncd", "synctui"}
}

// probeFunc returns the version probing function, falling back to the real
// subprocess implementation.
func (s *VersionSyncer) probeFunc() ComponentVersionFunc {
	if s.ProbeVersion != nil {
		return s.ProbeVersion
	}
	return probeComponentVersion
}

// updater returns the Updater to use, constructing a default one if needed.
func (s *VersionSyncer) updater() *Updater {
	if s.Updater != nil {
		return s.Updater
	}
	return NewUpdater(s.CLIVersion)
}

// probeComponentVersion runs "<siblingDir>/<component> --version" and parses
// the first version token from the output line "component vX.Y.Z".
func probeComponentVersion(siblingDir, component string) (string, error) {
	binaryPath := filepath.Join(siblingDir, binaryName(component))

	cmd := exec.Command(binaryPath, "--version") // #nosec G204 – path is derived from the install dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("probe %s version: %w", component, err)
	}

	// Expected output format: "<component> vX.Y.Z\n"
	line := strings.TrimSpace(string(out))
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", fmt.Errorf("unexpected version output from %s: %q", component, line)
	}
	return fields[len(fields)-1], nil
}

// binaryName returns the OS-appropriate binary filename for a component.
func binaryName(component string) string {
	if runtime.GOOS == "windows" {
		return component + ".exe"
	}
	return component
}
