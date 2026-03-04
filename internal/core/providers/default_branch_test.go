package providers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/git"
)

func TestGitHubResolverFallsBackToMaster(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithDefaultBranch(t, "master")
	resolver := NewGitHubResolver(git.NewClient())

	branch, err := resolver.ResolveDefaultBranch(context.Background(), repo, "origin")
	if err != nil {
		t.Fatalf("resolve default branch: %v", err)
	}
	if branch != "master" {
		t.Fatalf("branch = %q, want master", branch)
	}
}

func TestAzureResolverResolvesMain(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithDefaultBranch(t, "main")
	resolver := NewAzureDevOpsResolver(git.NewClient())

	branch, err := resolver.ResolveDefaultBranch(context.Background(), repo, "origin")
	if err != nil {
		t.Fatalf("resolve default branch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
}

func setupRepoWithDefaultBranch(t *testing.T, defaultBranch string) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	base := t.TempDir()
	seed := filepath.Join(base, "seed")
	remote := filepath.Join(base, "remote.git")
	clone := filepath.Join(base, "clone")

	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatalf("mkdir seed: %v", err)
	}

	runGit(t, seed, "init")
	runGit(t, seed, "config", "user.email", "tests@example.com")
	runGit(t, seed, "config", "user.name", "Test Runner")
	runGit(t, seed, "checkout", "-b", defaultBranch)

	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o600); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "initial")

	runGit(t, base, "init", "--bare", remote)
	runGit(t, seed, "remote", "add", "origin", remote)
	runGit(t, seed, "push", "-u", "origin", defaultBranch)
	runGit(t, base, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+defaultBranch)
	runGit(t, base, "clone", remote, clone)

	return clone
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}
