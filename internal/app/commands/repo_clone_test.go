package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestBuildCloneURLGitHub(t *testing.T) {
	source := config.SourceConfig{Provider: "github", Account: "jane", Organization: "acme", Host: "github.com"}
	got := BuildCloneURL(source, "acme/project")
	if got != "https://github.com/acme/project.git" {
		t.Fatalf("unexpected github clone URL %q", got)
	}
}

func TestBuildCloneURLAzure(t *testing.T) {
	source := config.SourceConfig{Provider: "azure", Account: "contoso", Organization: "platform", Host: "dev.azure.com"}
	got := BuildCloneURL(source, "repo-a")
	if got != "https://dev.azure.com/contoso/platform/_git/repo-a" {
		t.Fatalf("unexpected azure clone URL %q", got)
	}
}

func TestResolveCloneDestinationManaged(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace.Root = filepath.Join(t.TempDir(), "ws")
	source := config.SourceConfig{ID: "gh", Provider: "github", Account: "jane", Organization: "acme"}

	got, err := ResolveCloneDestination(cfg, source, "project", "managed")
	if err != nil {
		t.Fatalf("resolve destination failed: %v", err)
	}
	if !strings.Contains(got, filepath.Join("github", "acme", "project")) {
		t.Fatalf("unexpected managed path %q", got)
	}
}
