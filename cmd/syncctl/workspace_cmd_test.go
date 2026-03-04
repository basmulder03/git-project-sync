package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

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
