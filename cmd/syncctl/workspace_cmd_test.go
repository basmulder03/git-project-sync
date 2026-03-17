package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestWorkspaceShowCommand(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Workspace.Root = "/tmp/myworkspace"
	cfg.Workspace.Layout = "provider/account/repo"
	cfg.Workspace.CreateMissingPaths = true
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	output, err := executeSyncctl("--config", configPath, "workspace", "show")
	if err != nil {
		t.Fatalf("workspace show failed: %v output=%s", err, output)
	}
	if !strings.Contains(output, "root:") {
		t.Fatalf("workspace show output missing 'root:': %s", output)
	}
	if !strings.Contains(output, "layout:") {
		t.Fatalf("workspace show output missing 'layout:': %s", output)
	}
	if !strings.Contains(output, "create_missing_paths: true") {
		t.Fatalf("workspace show output missing 'create_missing_paths:': %s", output)
	}
}

func TestWorkspaceSetRootCommand(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	newRoot := filepath.Join(t.TempDir(), "newroot")
	output, err := executeSyncctl("--config", configPath, "workspace", "set-root", newRoot)
	if err != nil {
		t.Fatalf("workspace set-root failed: %v output=%s", err, output)
	}
	if !strings.Contains(output, "workspace root set to") {
		t.Fatalf("workspace set-root output unexpected: %s", output)
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Workspace.Root != newRoot {
		t.Fatalf("workspace root not persisted: got %q want %q", reloaded.Workspace.Root, newRoot)
	}
}

func TestWorkspaceLayoutCheckCommand_Clean(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Workspace.Root = t.TempDir()
	cfg.Repos = nil // no repos → no drift
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	output, err := executeSyncctl("--config", configPath, "workspace", "layout", "check")
	if err != nil {
		t.Fatalf("workspace layout check failed: %v output=%s", err, output)
	}
	if !strings.Contains(output, "clean") {
		t.Fatalf("expected 'clean' in output, got: %s", output)
	}
}

func TestWorkspaceLayoutFixDryRunAndApply(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Workspace.Root = filepath.Join(t.TempDir(), "workspace")
	cfg.Workspace.CreateMissingPaths = true
	cfg.Sources = []config.SourceConfig{{
		ID:       "gh-personal",
		Provider: "github",
		Account:  "jane-doe",
		Enabled:  true,
	}}
	cfg.Repos = []config.RepoConfig{{
		Path:     filepath.Join("/tmp", "wrong", "dotfiles"),
		SourceID: "gh-personal",
		Enabled:  true,
	}}

	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if output, err := executeSyncctl("--config", configPath, "workspace", "layout", "fix", "--dry-run"); err != nil {
		t.Fatalf("workspace layout fix dry-run failed: %v output=%s", err, output)
	} else if !strings.Contains(output, "would set") {
		t.Fatalf("dry-run output missing planned fix: %s", output)
	}

	if output, err := executeSyncctl("--config", configPath, "workspace", "layout", "fix"); err != nil {
		t.Fatalf("workspace layout fix failed: %v output=%s", err, output)
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	if !strings.Contains(reloaded.Repos[0].Path, filepath.Join("github", "jane-doe", "dotfiles")) {
		t.Fatalf("repo path not updated to managed layout: %s", reloaded.Repos[0].Path)
	}
}
