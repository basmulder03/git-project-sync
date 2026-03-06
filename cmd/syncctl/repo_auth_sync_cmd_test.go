package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestRepoAddListShowRemove(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Sources = []config.SourceConfig{{
		ID:       "gh-personal",
		Provider: "github",
		Account:  "jane",
		Host:     "github.com",
		Enabled:  true,
	}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if output, err := executeSyncctl("--config", configPath, "repo", "add", "/repos/dotfiles"); err != nil {
		t.Fatalf("repo add failed: %v output=%s", err, output)
	}

	listOut, err := executeSyncctl("--config", configPath, "repo", "list")
	if err != nil {
		t.Fatalf("repo list failed: %v", err)
	}
	if !strings.Contains(listOut, "/repos/dotfiles") {
		t.Fatalf("repo list output missing repo path: %s", listOut)
	}

	showOut, err := executeSyncctl("--config", configPath, "repo", "show", "/repos/dotfiles")
	if err != nil {
		t.Fatalf("repo show failed: %v", err)
	}
	if !strings.Contains(showOut, "source_id: gh-personal") {
		t.Fatalf("repo show output missing source id: %s", showOut)
	}

	if output, err := executeSyncctl("--config", configPath, "repo", "remove", "/repos/dotfiles"); err != nil {
		t.Fatalf("repo remove failed: %v output=%s", err, output)
	}
}

func TestAuthLoginRequiresToken(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Sources = []config.SourceConfig{{
		ID:       "gh-personal",
		Provider: "github",
		Account:  "jane",
		Host:     "github.com",
		Enabled:  true,
	}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := executeSyncctl("--config", configPath, "auth", "login", "gh-personal")
	if err == nil {
		t.Fatalf("expected login command to fail without token, output=%s", out)
	}
	if !strings.Contains(out, "token is required") {
		t.Fatalf("unexpected auth login error output: %s", out)
	}
}

func TestSyncAllNoEnabledRepos(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := executeSyncctl("--config", configPath, "sync", "all", "--dry-run")
	if err != nil {
		t.Fatalf("sync all failed: %v output=%s", err, out)
	}
	if !strings.Contains(out, "no enabled repos configured") {
		t.Fatalf("unexpected sync all output: %s", out)
	}
}

func TestRepoCloneRegistersRepo(t *testing.T) {
	prevClone := runGitClone
	runGitClone = func(repoURL, destination string) error {
		return os.MkdirAll(destination, 0o755)
	}
	t.Cleanup(func() {
		runGitClone = prevClone
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Default()
	cfg.Workspace.Root = filepath.Join(t.TempDir(), "workspace")
	cfg.Sources = []config.SourceConfig{{
		ID:           "gh-work",
		Provider:     "github",
		Account:      "jane",
		Organization: "acme",
		Host:         "github.com",
		Enabled:      true,
	}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := executeSyncctl("--config", configPath, "repo", "clone", "gh-work", "acme/project", "--into", "managed")
	if err != nil {
		t.Fatalf("repo clone failed: %v output=%s", err, out)
	}
	if !strings.Contains(out, "cloned acme/project") {
		t.Fatalf("unexpected clone output: %s", out)
	}

	listOut, err := executeSyncctl("--config", configPath, "repo", "list")
	if err != nil {
		t.Fatalf("repo list failed: %v output=%s", err, listOut)
	}
	if !strings.Contains(listOut, "project") {
		t.Fatalf("expected cloned repo in list output: %s", listOut)
	}
}
