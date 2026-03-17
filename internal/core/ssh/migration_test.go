package ssh_test

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	coressh "github.com/basmulder03/git-project-sync/internal/core/ssh"
)

// Tests for exported migration helpers.

func TestMigrateRepoToSSH_AlreadySSH(t *testing.T) {
	dir := newGitRepoWithRemote(t, "git@github.com:acme/repo.git")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	result := coressh.MigrateRepoToSSH(context.Background(), dir, "github-acme", "github", logger)

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Changed {
		t.Error("expected Changed=false for repo already using SSH")
	}
	if result.SkipReason != "already_ssh" {
		t.Errorf("skip reason = %q, want %q", result.SkipReason, "already_ssh")
	}
}

func TestMigrateRepoToSSH_HTTPS(t *testing.T) {
	dir := newGitRepoWithRemote(t, "https://github.com/acme/repo.git")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	result := coressh.MigrateRepoToSSH(context.Background(), dir, "github-acme", "github", logger)

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !result.Changed {
		t.Errorf("expected Changed=true; skip reason: %s", result.SkipReason)
	}
	if !coressh.IsSSHURL(result.NewURL) {
		t.Errorf("new URL is not SSH: %s", result.NewURL)
	}
	// The new URL should use the per-source alias.
	alias := coressh.AliasForSource("github-acme")
	if !containsMigrationStr(result.NewURL, alias) {
		t.Errorf("alias %q not found in new URL %q", alias, result.NewURL)
	}
}

func TestMigrateRepoToSSH_NoOrigin(t *testing.T) {
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-b", "main")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	result := coressh.MigrateRepoToSSH(context.Background(), dir, "src", "github", logger)

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true for repo with no origin")
	}
}

func TestMigrateWorkspaceToSSH(t *testing.T) {
	wsRoot := t.TempDir()

	// Repo with HTTPS origin.
	httpRepo := filepath.Join(wsRoot, "repo-https")
	if err := os.MkdirAll(httpRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, httpRepo, "init", "-b", "main")
	mustRunGit(t, httpRepo, "remote", "add", "origin", "https://github.com/acme/http-repo.git")

	// Repo already using SSH.
	sshRepo := filepath.Join(wsRoot, "repo-ssh")
	if err := os.MkdirAll(sshRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, sshRepo, "init", "-b", "main")
	mustRunGit(t, sshRepo, "remote", "add", "origin", "git@github.com:acme/ssh-repo.git")

	sources := []coressh.MigrationSource{
		{
			SourceID:   "github-acme",
			Provider:   "github",
			MatchHosts: []string{"github.com"},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	results := coressh.MigrateWorkspaceToSSH(context.Background(), wsRoot, sources, logger)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}

	var changed, skipped int
	for _, r := range results {
		if r.Error != nil {
			t.Errorf("repo %s error: %v", r.RepoPath, r.Error)
		}
		if r.Changed {
			changed++
		}
		if r.Skipped {
			skipped++
		}
	}

	if changed != 1 {
		t.Errorf("expected 1 changed, got %d", changed)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", skipped)
	}
}

// --- test helpers ---

func newGitRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-b", "main")
	mustRunGit(t, dir, "remote", "add", "origin", remoteURL)
	return dir
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func containsMigrationStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
