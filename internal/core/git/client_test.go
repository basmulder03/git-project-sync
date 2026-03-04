package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDirtyStateDetectsCleanRepo(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommit(t)
	client := NewClient()

	state, err := client.DirtyState(context.Background(), repo)
	if err != nil {
		t.Fatalf("dirty state: %v", err)
	}

	if state.IsDirty() {
		t.Fatalf("expected clean repo, got %+v", state)
	}
}

func TestDirtyStateDetectsStagedAndUntracked(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommit(t)

	tracked := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, repo, "add", "tracked.txt")

	untracked := filepath.Join(repo, "new.txt")
	if err := os.WriteFile(untracked, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	client := NewClient()
	state, err := client.DirtyState(context.Background(), repo)
	if err != nil {
		t.Fatalf("dirty state: %v", err)
	}

	if !state.HasStagedChanges {
		t.Fatal("expected staged changes")
	}
	if !state.HasUntrackedFiles {
		t.Fatal("expected untracked files")
	}
}

func TestDirtyStateReasonCodePriority(t *testing.T) {
	t.Parallel()

	state := DirtyState{HasUntrackedFiles: true, HasUnstagedChanges: true, HasStagedChanges: true}
	if state.ReasonCode() != "repo_staged_changes" {
		t.Fatalf("reason code = %q, want repo_staged_changes", state.ReasonCode())
	}
}

func initRepoWithCommit(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "tests@example.com")
	runGit(t, repo, "config", "user.name", "Test Runner")

	tracked := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}

	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
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
