package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupMergedLocalBranchesDeletesCheckedOutMergedBranch(t *testing.T) {
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
	result, err := client.CleanupMergedLocalBranches(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("cleanup stale branch: %v", err)
	}

	if len(result.DeletedBranches) != 1 || result.DeletedBranches[0] != "feature/test" {
		t.Fatalf("deleted branches = %+v, want [feature/test]", result.DeletedBranches)
	}

	current, err := client.CurrentBranch(context.Background(), repo)
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if current != "master" {
		t.Fatalf("current branch = %q, want master", current)
	}
}

func TestCleanupMergedLocalBranchesSkipsWhenNotMerged(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommitCleanup(t)
	runGitCleanup(t, repo, "checkout", "-b", "feature/test")

	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o600); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGitCleanup(t, repo, "add", "feature.txt")
	runGitCleanup(t, repo, "commit", "-m", "feature commit")

	client := NewClient()
	result, err := client.CleanupMergedLocalBranches(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("cleanup stale branch: %v", err)
	}

	if len(result.DeletedBranches) != 0 {
		t.Fatalf("expected no deleted branches, got %+v", result.DeletedBranches)
	}

	if len(result.Decisions) == 0 {
		t.Fatal("expected at least one cleanup decision")
	}
	if result.Decisions[0].ReasonCode != "cleanup_branch_not_merged" {
		t.Fatalf("reason_code=%q want cleanup_branch_not_merged", result.Decisions[0].ReasonCode)
	}
}

func TestCleanupMergedLocalBranchesCleansAllMergedBranches(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommitCleanup(t)

	createAndMerge := func(branch string) {
		runGitCleanup(t, repo, "checkout", "-b", branch)
		name := strings.ReplaceAll(branch, "/", "-") + ".txt"
		if err := os.WriteFile(filepath.Join(repo, name), []byte(branch+"\n"), 0o600); err != nil {
			t.Fatalf("write branch file: %v", err)
		}
		runGitCleanup(t, repo, "add", name)
		runGitCleanup(t, repo, "commit", "-m", branch)
		runGitCleanup(t, repo, "checkout", "master")
		runGitCleanup(t, repo, "merge", "--ff-only", branch)
	}

	createAndMerge("feature/one")
	createAndMerge("feature/two")

	runGitCleanup(t, repo, "checkout", "-b", "feature/not-merged")
	if err := os.WriteFile(filepath.Join(repo, "feature-not-merged.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write non-merged branch file: %v", err)
	}
	runGitCleanup(t, repo, "add", "feature-not-merged.txt")
	runGitCleanup(t, repo, "commit", "-m", "feature/not-merged")
	runGitCleanup(t, repo, "checkout", "master")

	client := NewClient()
	result, err := client.CleanupMergedLocalBranches(context.Background(), repo, "master")
	if err != nil {
		t.Fatalf("cleanup stale branches: %v", err)
	}

	if len(result.DeletedBranches) != 2 {
		t.Fatalf("deleted branches = %+v, want 2 merged branches", result.DeletedBranches)
	}

	out := runGitCleanupOutput(t, repo, "branch", "--list")
	if !strings.Contains(out, "feature/not-merged") {
		t.Fatalf("expected non-merged branch to remain, branches output: %q", out)
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

func runGitCleanupOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}

	return strings.TrimSpace(string(output))
}
