package main

import (
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/install"
)

func TestReleaseBaseURL(t *testing.T) {
	t.Parallel()

	if got := releaseBaseURL("owner/repo", "latest"); got != "https://github.com/owner/repo/releases/latest/download" {
		t.Fatalf("unexpected latest URL: %s", got)
	}
	if got := releaseBaseURL("owner/repo", "v1.2.3"); got != "https://github.com/owner/repo/releases/download/v1.2.3" {
		t.Fatalf("unexpected tagged URL: %s", got)
	}
}

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()

	if got := normalizeVersion("v1.2.3"); got != "1.2.3" {
		t.Fatalf("unexpected normalized version: %s", got)
	}
	if got := normalizeVersion("2.4"); got != "2.4.0" {
		t.Fatalf("unexpected normalized short version: %s", got)
	}
	if got := normalizeVersion("dev"); got != "" {
		t.Fatalf("expected invalid version to normalize to empty, got %s", got)
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	if cmp, ok := compareVersions("1.2.3", "1.2.4"); !ok || cmp != -1 {
		t.Fatalf("expected 1.2.3 < 1.2.4, got cmp=%d ok=%v", cmp, ok)
	}
	if cmp, ok := compareVersions("2.0.0", "1.9.9"); !ok || cmp != 1 {
		t.Fatalf("expected 2.0.0 > 1.9.9, got cmp=%d ok=%v", cmp, ok)
	}
	if cmp, ok := compareVersions("1.0", "1.0.0"); !ok || cmp != 0 {
		t.Fatalf("expected 1.0 == 1.0.0, got cmp=%d ok=%v", cmp, ok)
	}
	if _, ok := compareVersions("dev", "1.0.0"); ok {
		t.Fatal("expected unknown comparison for non-semver input")
	}
}

func TestParseVersionFromOutput(t *testing.T) {
	t.Parallel()

	if got := parseVersionFromOutput("syncd 1.8.2\n"); got != "1.8.2" {
		t.Fatalf("unexpected parsed version: %s", got)
	}
	if got := parseVersionFromOutput("syncd dev\n"); got != "" {
		t.Fatalf("expected empty version for dev output, got %s", got)
	}
}

func TestDefaultPathsLinux(t *testing.T) {
	t.Parallel()

	binPath, configPath, err := defaultPaths(install.ModeSystem, "linux")
	if err != nil {
		t.Fatalf("defaultPaths(system, linux): %v", err)
	}
	if binPath != "/usr/local/bin/syncd" {
		t.Fatalf("unexpected system bin path: %s", binPath)
	}
	if configPath != "/etc/git-project-sync/config.yaml" {
		t.Fatalf("unexpected system config path: %s", configPath)
	}
}

func TestDefaultPathsUnsupportedOS(t *testing.T) {
	t.Parallel()

	if _, _, err := defaultPaths(install.ModeUser, "darwin"); err == nil {
		t.Fatal("expected unsupported OS error")
	}
}
