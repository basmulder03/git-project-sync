package workspace

import (
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestExpectedRepoPathUsesProviderAndContext(t *testing.T) {
	t.Parallel()

	resolver, err := NewLayoutResolver("/workspace")
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	got, err := resolver.ExpectedRepoPath(
		config.SourceConfig{Provider: "github", Account: "jane-doe", Organization: "acme-org"},
		config.RepoConfig{Path: "/tmp/platform-api"},
	)
	if err != nil {
		t.Fatalf("expected repo path: %v", err)
	}

	want := filepath.Join("/workspace", "github", "acme-org", "platform-api")
	if got != want {
		t.Fatalf("expected path = %q, want %q", got, want)
	}
}

func TestValidatorDetectsAndFixesDrift(t *testing.T) {
	t.Parallel()

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
		Path:     filepath.Join("/random", "location", "dotfiles"),
		SourceID: "gh-personal",
		Enabled:  true,
	}}

	validator, err := NewValidator(cfg)
	if err != nil {
		t.Fatalf("new validator: %v", err)
	}

	drifts, err := validator.Check(cfg)
	if err != nil {
		t.Fatalf("check drifts: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("drift count = %d, want 1", len(drifts))
	}

	updated, err := ApplyPathFixes(&cfg, drifts, true)
	if err != nil {
		t.Fatalf("apply fixes: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated count = %d, want 1", updated)
	}

	if cfg.Repos[0].Path != drifts[0].ExpectedPath {
		t.Fatalf("repo path after fix = %q, want %q", cfg.Repos[0].Path, drifts[0].ExpectedPath)
	}
}

func TestRepoPathForAzureDevOps(t *testing.T) {
	t.Parallel()

	layout := NewLayout(config.WorkspaceConfig{
		Root:   "/workspace",
		Layout: "flat",
	})

	// Test Azure DevOps with account/project format
	path := layout.RepoPath("azuredevops", "rr-wfm/Platform", "RR.ApiGateway")
	want := filepath.Join("/workspace", "azuredevops", "rr-wfm", "platform", "rr.apigateway")
	if path != want {
		t.Errorf("Azure DevOps path = %q, want %q", path, want)
	}

	// Test GitHub (should use flat structure)
	path = layout.RepoPath("github", "myorg", "myrepo")
	want = filepath.Join("/workspace", "github", "myorg", "myrepo")
	if path != want {
		t.Errorf("GitHub path = %q, want %q", path, want)
	}

	// Test Azure DevOps with single-part owner (edge case)
	path = layout.RepoPath("azuredevops", "account-only", "repo")
	want = filepath.Join("/workspace", "azuredevops", "account-only", "repo")
	if path != want {
		t.Errorf("Azure DevOps single-part owner path = %q, want %q", path, want)
	}
}
