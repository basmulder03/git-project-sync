package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveDefaultBranchFromRemoteHEAD(t *testing.T) {
	t.Parallel()

	repo := setupClonedRepoBranches(t, "main")
	client := NewClient()

	branch, err := client.ResolveDefaultBranchFromRemoteHEAD(context.Background(), repo, "origin")
	if err != nil {
		t.Fatalf("resolve default branch: %v", err)
	}

	if branch != "main" {
		t.Fatalf("default branch = %q, want main", branch)
	}
}

func TestAheadBehindAndFastForward(t *testing.T) {
	t.Parallel()

	remote, repo := setupRemoteAndCloneBranches(t, "main")
	writer := cloneRepoBranches(t, remote)
	runGitBranches(t, writer, "checkout", "main")

	if err := os.WriteFile(filepath.Join(writer, "change.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitBranches(t, writer, "add", "change.txt")
	runGitBranches(t, writer, "commit", "-m", "remote change")
	runGitBranches(t, writer, "push", "origin", "main")

	client := NewClient()
	if err := client.FetchAndPrune(context.Background(), repo, "origin"); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	ahead, behind, err := client.AheadBehind(context.Background(), repo, "main", "origin/main")
	if err != nil {
		t.Fatalf("ahead/behind: %v", err)
	}
	if ahead != 0 || behind == 0 {
		t.Fatalf("ahead=%d behind=%d, expected local behind remote", ahead, behind)
	}

	if err := client.FastForwardTo(context.Background(), repo, "origin/main"); err != nil {
		t.Fatalf("fast-forward: %v", err)
	}

	ahead, behind, err = client.AheadBehind(context.Background(), repo, "main", "origin/main")
	if err != nil {
		t.Fatalf("ahead/behind after ff: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Fatalf("ahead=%d behind=%d after ff, want 0/0", ahead, behind)
	}
}

func setupClonedRepoBranches(t *testing.T, defaultBranch string) string {
	t.Helper()

	_, clone := setupRemoteAndCloneBranches(t, defaultBranch)
	return clone
}

func setupRemoteAndCloneBranches(t *testing.T, defaultBranch string) (string, string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	base := t.TempDir()
	seed := filepath.Join(base, "seed")
	remote := filepath.Join(base, "remote.git")
	clone := filepath.Join(base, "clone")

	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatalf("create seed dir: %v", err)
	}
	runGitBranches(t, seed, "init")
	runGitBranches(t, seed, "config", "user.email", "tests@example.com")
	runGitBranches(t, seed, "config", "user.name", "Test Runner")
	runGitBranches(t, seed, "checkout", "-b", defaultBranch)

	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGitBranches(t, seed, "add", "README.md")
	runGitBranches(t, seed, "commit", "-m", "initial")

	runGitBranches(t, base, "init", "--bare", remote)
	runGitBranches(t, seed, "remote", "add", "origin", remote)
	runGitBranches(t, seed, "push", "-u", "origin", defaultBranch)
	runGitBranches(t, base, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+defaultBranch)

	runGitBranches(t, base, "clone", remote, clone)
	return remote, clone
}

func cloneRepoBranches(t *testing.T, remote string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	runGitBranches(t, t.TempDir(), "clone", remote, clone)
	runGitBranches(t, clone, "config", "user.email", "tests@example.com")
	runGitBranches(t, clone, "config", "user.name", "Test Runner")
	return clone
}

func runGitBranches(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}
