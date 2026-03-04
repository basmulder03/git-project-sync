package integration

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coregit "github.com/basmulder03/git-project-sync/internal/core/git"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func TestDirtyRepoSkipIncludesReasonAndTrace(t *testing.T) {
	t.Parallel()

	repo := initIntegrationRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	engine := coresync.NewEngine(coregit.NewClient(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	traceID := "trace-integration-1"
	result, err := engine.RunRepo(context.Background(), traceID, config.SourceConfig{Provider: "github", Host: "github.com"}, config.RepoConfig{
		Path:        repo,
		Remote:      "origin",
		Provider:    "github",
		SkipIfDirty: true,
	}, true)
	if err != nil {
		t.Fatalf("run repo sync: %v", err)
	}

	if !result.Skipped {
		t.Fatal("expected dirty repo to be skipped")
	}
	if result.ReasonCode != "repo_untracked_files" {
		t.Fatalf("reason_code=%q want repo_untracked_files", result.ReasonCode)
	}
	if result.TraceID != traceID {
		t.Fatalf("trace_id=%q want %q", result.TraceID, traceID)
	}
}

func initIntegrationRepo(t *testing.T) string {
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

	runGitIntegration(t, seed, "init")
	runGitIntegration(t, seed, "config", "user.email", "tests@example.com")
	runGitIntegration(t, seed, "config", "user.name", "Test Runner")
	runGitIntegration(t, seed, "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGitIntegration(t, seed, "add", "README.md")
	runGitIntegration(t, seed, "commit", "-m", "initial")

	runGitIntegration(t, base, "init", "--bare", remote)
	runGitIntegration(t, seed, "remote", "add", "origin", remote)
	runGitIntegration(t, seed, "push", "-u", "origin", "main")
	runGitIntegration(t, base, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/main")

	runGitIntegration(t, base, "clone", remote, clone)
	return clone
}

func runGitIntegration(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}
