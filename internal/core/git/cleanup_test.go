package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanupCheckedOutStaleBranchDeletesMergedBranch(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommitCleanup(t)
	runGitCleanup(t, repo, "checkout", "-b", "feature/test")

	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o600); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGitCleanup(t, repo, "add", "feature.txt")
	runGitCleanup(t, repo, "commit", "-m", "feature commit")
	runGitCleanup(t, repo, "checkout", "master")
	runGitCleanup(t, repo, "merge", "--ff-only", "feature/test")
	runGitCleanup(t, repo, "checkout", "feature/test")

	client := NewClient()
	result, err := client.CleanupCheckedOutStaleBranch(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("cleanup stale branch: %v", err)
	}

	if result.DeletedBranch != "feature/test" {
		t.Fatalf("deleted branch = %q, want feature/test", result.DeletedBranch)
	}

	current, err := client.CurrentBranch(context.Background(), repo)
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if current != "master" {
		t.Fatalf("current branch = %q, want master", current)
	}
}

func TestCleanupCheckedOutStaleBranchSkipsWhenNotMerged(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommitCleanup(t)
	runGitCleanup(t, repo, "checkout", "-b", "feature/test")

	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o600); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGitCleanup(t, repo, "add", "feature.txt")
	runGitCleanup(t, repo, "commit", "-m", "feature commit")

	client := NewClient()
	result, err := client.CleanupCheckedOutStaleBranch(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("cleanup stale branch: %v", err)
	}

	if !result.Skipped {
		t.Fatal("expected cleanup to be skipped")
	}
	if result.ReasonCode != "cleanup_branch_not_merged" {
		t.Fatalf("reason_code=%q want cleanup_branch_not_merged", result.ReasonCode)
	}
}

func initRepoWithCommitCleanup(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	runGitCleanup(t, repo, "init")
	runGitCleanup(t, repo, "config", "user.email", "tests@example.com")
	runGitCleanup(t, repo, "config", "user.name", "Test Runner")

	tracked := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}

	runGitCleanup(t, repo, "add", "tracked.txt")
	runGitCleanup(t, repo, "commit", "-m", "initial")
	return repo
}

func runGitCleanup(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}
