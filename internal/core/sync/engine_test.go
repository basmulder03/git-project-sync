package sync

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/git"
)

func TestEngineRunRepoFastForwardsBehindBranch(t *testing.T) {
	t.Parallel()

	remote, repo := setupRemoteAndCloneEngine(t, "main")
	writer := cloneRepoEngine(t, remote)
	runGitEngine(t, writer, "checkout", "main")

	if err := os.WriteFile(filepath.Join(writer, "remote.txt"), []byte("change\n"), 0o600); err != nil {
		t.Fatalf("write remote file: %v", err)
	}
	runGitEngine(t, writer, "add", "remote.txt")
	runGitEngine(t, writer, "commit", "-m", "remote update")
	runGitEngine(t, writer, "push", "origin", "main")

	engine := NewEngine(git.NewClient(), testEngineLogger())
	result, err := engine.RunRepo(context.Background(), "trace-1", config.SourceConfig{Provider: "github"}, config.RepoConfig{
		Path:        repo,
		Remote:      "origin",
		Provider:    "github",
		SkipIfDirty: true,
	}, false)
	if err != nil {
		t.Fatalf("run repo sync engine: %v", err)
	}

	if !result.Mutated {
		t.Fatal("expected fast-forward mutation")
	}
}

func TestEngineRunRepoSkipsNonFastForwardDivergence(t *testing.T) {
	t.Parallel()

	remote, repo := setupRemoteAndCloneEngine(t, "main")

	local := repo
	runGitEngine(t, local, "checkout", "main")
	if err := os.WriteFile(filepath.Join(local, "local.txt"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	runGitEngine(t, local, "add", "local.txt")
	runGitEngine(t, local, "commit", "-m", "local-only")

	writer := cloneRepoEngine(t, remote)
	runGitEngine(t, writer, "checkout", "main")
	if err := os.WriteFile(filepath.Join(writer, "remote.txt"), []byte("remote\n"), 0o600); err != nil {
		t.Fatalf("write remote file: %v", err)
	}
	runGitEngine(t, writer, "add", "remote.txt")
	runGitEngine(t, writer, "commit", "-m", "remote-only")
	runGitEngine(t, writer, "push", "origin", "main")

	engine := NewEngine(git.NewClient(), testEngineLogger())
	result, err := engine.RunRepo(context.Background(), "trace-2", config.SourceConfig{Provider: "github"}, config.RepoConfig{
		Path:        repo,
		Remote:      "origin",
		Provider:    "github",
		SkipIfDirty: true,
	}, false)
	if err != nil {
		t.Fatalf("run repo sync engine: %v", err)
	}

	if !result.Skipped {
		t.Fatal("expected non-fast-forward repo to be skipped")
	}
	if result.ReasonCode != "non_fast_forward" {
		t.Fatalf("reason_code=%q want non_fast_forward", result.ReasonCode)
	}
}

func TestEngineRunRepoDeletesCheckedOutMergedBranch(t *testing.T) {
	t.Parallel()

	_, repo := setupRemoteAndCloneEngine(t, "main")
	runGitEngine(t, repo, "checkout", "-b", "feature/cleanup")

	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o600); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGitEngine(t, repo, "add", "feature.txt")
	runGitEngine(t, repo, "commit", "-m", "feature")
	runGitEngine(t, repo, "checkout", "main")
	runGitEngine(t, repo, "merge", "--ff-only", "feature/cleanup")
	runGitEngine(t, repo, "checkout", "feature/cleanup")

	engine := NewEngine(git.NewClient(), testEngineLogger())
	result, err := engine.RunRepo(context.Background(), "trace-3", config.SourceConfig{Provider: "github"}, config.RepoConfig{
		Path:                       repo,
		Remote:                     "origin",
		Provider:                   "github",
		SkipIfDirty:                true,
		CleanupMergedLocalBranches: true,
	}, false)
	if err != nil {
		t.Fatalf("run repo sync engine: %v", err)
	}

	if !result.Mutated {
		t.Fatal("expected cleanup mutation")
	}

	currentBranchOut := runGitOutputEngine(t, repo, "branch", "--show-current")
	if currentBranchOut != "main" {
		t.Fatalf("current branch=%q want main", currentBranchOut)
	}

	branchesOut := runGitOutputEngine(t, repo, "branch", "--list", "feature/cleanup")
	if branchesOut != "" {
		t.Fatalf("feature branch should be deleted, got output=%q", branchesOut)
	}
}

func testEngineLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func setupRemoteAndCloneEngine(t *testing.T, defaultBranch string) (string, string) {
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
	runGitEngine(t, seed, "init")
	runGitEngine(t, seed, "config", "user.email", "tests@example.com")
	runGitEngine(t, seed, "config", "user.name", "Test Runner")
	runGitEngine(t, seed, "checkout", "-b", defaultBranch)

	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGitEngine(t, seed, "add", "README.md")
	runGitEngine(t, seed, "commit", "-m", "initial")

	runGitEngine(t, base, "init", "--bare", remote)
	runGitEngine(t, seed, "remote", "add", "origin", remote)
	runGitEngine(t, seed, "push", "-u", "origin", defaultBranch)
	runGitEngine(t, base, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+defaultBranch)
	runGitEngine(t, base, "clone", remote, clone)
	runGitEngine(t, clone, "config", "user.email", "tests@example.com")
	runGitEngine(t, clone, "config", "user.name", "Test Runner")

	return remote, clone
}

func cloneRepoEngine(t *testing.T, remote string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	runGitEngine(t, t.TempDir(), "clone", remote, clone)
	runGitEngine(t, clone, "config", "user.email", "tests@example.com")
	runGitEngine(t, clone, "config", "user.name", "Test Runner")
	return clone
}

func runGitEngine(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}

func runGitOutputEngine(t *testing.T, repo string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
	return strings.TrimSpace(string(output))
}
