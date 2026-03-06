package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/providers/api"
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

func TestResolveRunReposMergesConfiguredAndDiscoveredWithoutDuplicates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configuredPath := filepath.Join(root, "github", "acme", "repo-configured")
	discoveredPath := filepath.Join(root, "github", "acme", "repo-discovered")
	for _, repoPath := range []string{configuredPath, discoveredPath} {
		if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
			t.Fatalf("create test repo: %v", err)
		}
	}

	cfg := config.Default()
	cfg.Workspace.Root = root
	cfg.Sources = []config.SourceConfig{{ID: "gh-acme", Provider: "github", Account: "acme", Enabled: true}}
	cfg.Repos = []config.RepoConfig{{Path: configuredPath, SourceID: "gh-acme", Enabled: false}}

	result, err := ResolveRunRepos(cfg)
	if err != nil {
		t.Fatalf("ResolveRunRepos failed: %v", err)
	}
	if len(result.Repos) != 2 {
		t.Fatalf("expected two repos (configured+discovered), got %d", len(result.Repos))
	}

	byPath := map[string]config.RepoConfig{}
	for _, repo := range result.Repos {
		byPath[filepath.Clean(repo.Path)] = repo
	}
	if len(byPath) != 2 {
		t.Fatalf("expected no duplicate paths, got %d unique", len(byPath))
	}
	if byPath[filepath.Clean(configuredPath)].Enabled {
		t.Fatal("expected configured repo settings to remain unchanged")
	}
	if !byPath[filepath.Clean(discoveredPath)].Enabled {
		t.Fatal("expected discovered repo to default to enabled")
	}
}

func TestIsIgnorableWalkError(t *testing.T) {
	t.Parallel()

	if !isIgnorableWalkError(os.ErrPermission) {
		t.Fatal("expected os.ErrPermission to be ignorable")
	}
	wrapped := fmt.Errorf("open D:/System Volume Information: %w", os.ErrPermission)
	if !isIgnorableWalkError(wrapped) {
		t.Fatal("expected wrapped permission error to be ignorable")
	}
}

func TestIdentifyReposToClone(t *testing.T) {
	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root: "/workspace",
		},
	}

	remoteRepos := []api.RemoteRepository{
		{
			Provider: "github",
			Owner:    "testuser",
			Name:     "repo1",
			SourceID: "source1",
		},
		{
			Provider: "github",
			Owner:    "testuser",
			Name:     "repo2",
			SourceID: "source1",
		},
		{
			Provider: "github",
			Owner:    "testuser",
			Name:     "repo3",
			SourceID: "source1",
		},
	}

	localRepos := []config.RepoConfig{
		{
			Path:     "/workspace/github/testuser/repo1",
			SourceID: "source1",
		},
	}

	reposToClone := IdentifyReposToClone(cfg, remoteRepos, localRepos)

	// Should identify repo2 and repo3 as needing to be cloned
	if len(reposToClone) != 2 {
		t.Errorf("expected 2 repos to clone, got %d", len(reposToClone))
	}

	// Verify it's the right repos
	found := make(map[string]bool)
	for _, repo := range reposToClone {
		found[repo.Name] = true
	}

	if !found["repo2"] || !found["repo3"] {
		t.Errorf("wrong repos identified for cloning: %+v", reposToClone)
	}
}

func TestBuildListOptions(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.Config
		sourceID        string
		wantIncArchived bool
		wantMaxSizeKB   int64
	}{
		{
			name: "default policy settings",
			cfg: config.Config{
				Governance: config.GovernanceConfig{
					DefaultPolicy: config.SyncPolicyConfig{
						AutoCloneEnabled:         boolPtr(true),
						AutoCloneMaxSizeMB:       1024,
						AutoCloneIncludeArchived: false,
					},
				},
			},
			sourceID:        "test-source",
			wantIncArchived: false,
			wantMaxSizeKB:   1024 * 1024,
		},
		{
			name: "source-specific override",
			cfg: config.Config{
				Governance: config.GovernanceConfig{
					DefaultPolicy: config.SyncPolicyConfig{
						AutoCloneEnabled:         boolPtr(true),
						AutoCloneMaxSizeMB:       2048,
						AutoCloneIncludeArchived: false,
					},
					SourcePolicies: map[string]config.SyncPolicyConfig{
						"test-source": {
							AutoCloneMaxSizeMB: 512,
						},
					},
				},
			},
			sourceID:        "test-source",
			wantIncArchived: false,
			wantMaxSizeKB:   512 * 1024,
		},
		{
			name: "auto-clone disabled for source",
			cfg: config.Config{
				Governance: config.GovernanceConfig{
					DefaultPolicy: config.SyncPolicyConfig{
						AutoCloneEnabled:         boolPtr(true),
						AutoCloneMaxSizeMB:       2048,
						AutoCloneIncludeArchived: false,
					},
					SourcePolicies: map[string]config.SyncPolicyConfig{
						"test-source": {
							AutoCloneEnabled: boolPtr(false),
						},
					},
				},
			},
			sourceID:        "test-source",
			wantIncArchived: false,
			wantMaxSizeKB:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildListOptions(tt.cfg, tt.sourceID)

			if opts.IncludeArchived != tt.wantIncArchived {
				t.Errorf("IncludeArchived = %v, want %v", opts.IncludeArchived, tt.wantIncArchived)
			}
			if opts.MaxSizeKB != tt.wantMaxSizeKB {
				t.Errorf("MaxSizeKB = %d, want %d", opts.MaxSizeKB, tt.wantMaxSizeKB)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
