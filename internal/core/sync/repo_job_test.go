package sync

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/git"
)

func TestRepoJobSkipsDirtyRepoWhenConfigured(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	job := NewRepoJob(git.NewClient(), testLogger())
	result, err := job.Run(context.Background(), "trace-1", config.RepoConfig{
		Path:        repo,
		SkipIfDirty: true,
		Remote:      "origin",
	}, false)
	if err != nil {
		t.Fatalf("run repo job: %v", err)
	}

	if !result.Skipped {
		t.Fatal("expected job to be skipped")
	}
	if result.ReasonCode != "repo_untracked_files" {
		t.Fatalf("reason code = %q, want repo_untracked_files", result.ReasonCode)
	}
}

func TestRepoJobAllowsCleanRepo(t *testing.T) {
	t.Parallel()

	repo := initRepoWithCommit(t)
	job := NewRepoJob(git.NewClient(), testLogger())

	result, err := job.Run(context.Background(), "trace-2", config.RepoConfig{
		Path:        repo,
		SkipIfDirty: true,
		Remote:      "origin",
	}, true)
	if err != nil {
		t.Fatalf("run repo job: %v", err)
	}

	if result.Skipped {
		t.Fatal("did not expect skip for clean repo")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
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
