package integration

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
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func TestPolicyDeniedSyncDoesNotMutateRepository(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	remote, repo := setupRemoteAndClonePolicy(t)
	writer := cloneRepoPolicy(t, remote)
	runGitPolicy(t, writer, "checkout", "main")

	if err := os.WriteFile(filepath.Join(writer, "remote.txt"), []byte("remote\n"), 0o600); err != nil {
		t.Fatalf("write remote file: %v", err)
	}
	runGitPolicy(t, writer, "add", "remote.txt")
	runGitPolicy(t, writer, "commit", "-m", "remote update")
	runGitPolicy(t, writer, "push", "origin", "main")

	before := runGitOutputPolicy(t, repo, "rev-parse", "HEAD")

	engine := coresync.NewEngine(git.NewClient(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	engine.SetGovernance(config.GovernanceConfig{DefaultPolicy: config.SyncPolicyConfig{ProtectedRepoPatterns: []string{`/clone$`}}})
	result, err := engine.RunRepo(context.Background(), "trace-policy-int", config.SourceConfig{Provider: "github", ID: "gh1"}, config.RepoConfig{Path: repo, Remote: "origin", Provider: "github", SkipIfDirty: true}, false)
	if err != nil {
		t.Fatalf("run repo: %v", err)
	}
	if !result.Skipped || !strings.HasPrefix(result.ReasonCode, "policy_") {
		t.Fatalf("expected policy skip, got %+v", result)
	}

	after := runGitOutputPolicy(t, repo, "rev-parse", "HEAD")
	if after != before {
		t.Fatalf("expected no mutation for policy-denied sync, before=%s after=%s", before, after)
	}
}

func setupRemoteAndClonePolicy(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	seed := filepath.Join(base, "seed")
	remote := filepath.Join(base, "remote.git")
	clone := filepath.Join(base, "clone")

	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatalf("create seed dir: %v", err)
	}
	runGitPolicy(t, seed, "init")
	runGitPolicy(t, seed, "config", "user.email", "tests@example.com")
	runGitPolicy(t, seed, "config", "user.name", "Test Runner")
	runGitPolicy(t, seed, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGitPolicy(t, seed, "add", "README.md")
	runGitPolicy(t, seed, "commit", "-m", "initial")

	runGitPolicy(t, base, "init", "--bare", remote)
	runGitPolicy(t, seed, "remote", "add", "origin", remote)
	runGitPolicy(t, seed, "push", "-u", "origin", "main")
	runGitPolicy(t, base, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitPolicy(t, base, "clone", remote, clone)
	runGitPolicy(t, clone, "config", "user.email", "tests@example.com")
	runGitPolicy(t, clone, "config", "user.name", "Test Runner")

	return remote, clone
}

func cloneRepoPolicy(t *testing.T, remote string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	runGitPolicy(t, t.TempDir(), "clone", remote, clone)
	runGitPolicy(t, clone, "config", "user.email", "tests@example.com")
	runGitPolicy(t, clone, "config", "user.name", "Test Runner")
	return clone
}

func runGitPolicy(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}

func runGitOutputPolicy(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}
