package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestResolveRunReposUsesConfiguredReposWhenPresent(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Repos = []config.RepoConfig{{Path: "/repos/a", SourceID: "gh", Enabled: true}}

	result, err := ResolveRunRepos(cfg)
	if err != nil {
		t.Fatalf("ResolveRunRepos failed: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one repo, got %d", len(result.Repos))
	}
}

func TestResolveRunReposDiscoversAndMapsRepos(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoPath := filepath.Join(root, "github", "acme", "repo-a")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatalf("create test repo: %v", err)
	}

	cfg := config.Default()
	cfg.Workspace.Root = root
	cfg.Sources = []config.SourceConfig{{ID: "gh-acme", Provider: "github", Account: "acme", Enabled: true}}

	result, err := ResolveRunRepos(cfg)
	if err != nil {
		t.Fatalf("ResolveRunRepos failed: %v", err)
	}
	if len(result.Repos) != 1 {
		t.Fatalf("expected one discovered repo, got %d", len(result.Repos))
	}
	if result.Repos[0].SourceID != "gh-acme" {
		t.Fatalf("expected mapped source gh-acme, got %q", result.Repos[0].SourceID)
	}
	if !result.Repos[0].Enabled {
		t.Fatal("expected discovered repo to be enabled")
	}
}

func TestResolveRunReposSkipsUnmappedWhenMultipleSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoPath := filepath.Join(root, "github", "other", "repo-a")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatalf("create test repo: %v", err)
	}

	cfg := config.Default()
	cfg.Workspace.Root = root
	cfg.Sources = []config.SourceConfig{
		{ID: "gh-acme", Provider: "github", Account: "acme", Enabled: true},
		{ID: "gh-team", Provider: "github", Account: "team", Enabled: true},
	}

	result, err := ResolveRunRepos(cfg)
	if err != nil {
		t.Fatalf("ResolveRunRepos failed: %v", err)
	}
	if len(result.Repos) != 0 {
		t.Fatalf("expected no mapped repos, got %d", len(result.Repos))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected one skipped repo, got %d", len(result.Skipped))
	}
}
