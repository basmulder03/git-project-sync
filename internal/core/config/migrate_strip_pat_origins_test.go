package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveEmbeddedCredential(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "github PAT embedded",
			input:   "https://ghp_SECRETTOKEN@github.com/owner/repo.git",
			want:    "https://github.com/owner/repo.git",
			changed: true,
		},
		{
			name:    "clean github URL unchanged",
			input:   "https://github.com/owner/repo.git",
			want:    "https://github.com/owner/repo.git",
			changed: false,
		},
		{
			name:    "azure devops clean URL unchanged",
			input:   "https://dev.azure.com/myorg/myproject/_git/myrepo",
			want:    "https://dev.azure.com/myorg/myproject/_git/myrepo",
			changed: false,
		},
		{
			name:    "SSH URL unchanged",
			input:   "git@github.com:owner/repo.git",
			want:    "git@github.com:owner/repo.git",
			changed: false,
		},
		{
			name:    "github enterprise PAT embedded",
			input:   "https://mytoken@ghe.example.com/owner/repo.git",
			want:    "https://ghe.example.com/owner/repo.git",
			changed: true,
		},
		{
			name:    "URL with no dot in host after @ - not treated as credential",
			input:   "https://org@nondothost/path",
			want:    "https://org@nondothost/path",
			changed: false,
		},
		{
			name:    "empty URL unchanged",
			input:   "",
			want:    "",
			changed: false,
		},
		{
			name:    "http (non-https) URL unchanged",
			input:   "http://token@example.com/repo.git",
			want:    "http://token@example.com/repo.git",
			changed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := removeEmbeddedCredential(tc.input)
			if got != tc.want {
				t.Errorf("removeEmbeddedCredential(%q) url = %q, want %q", tc.input, got, tc.want)
			}
			if changed != tc.changed {
				t.Errorf("removeEmbeddedCredential(%q) changed = %v, want %v", tc.input, changed, tc.changed)
			}
		})
	}
}

func TestFindGitRepos(t *testing.T) {
	root := t.TempDir()

	// Create three fake git repos
	for _, name := range []string{"repo-a", "repo-b", "repo-c"} {
		repoDir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create a non-repo directory
	if err := os.MkdirAll(filepath.Join(root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}

	repos, err := findGitRepos(root)
	if err != nil {
		t.Fatalf("findGitRepos: %v", err)
	}

	if len(repos) != 3 {
		t.Errorf("expected 3 repos, got %d: %v", len(repos), repos)
	}
}

func TestFindGitRepos_EmptyWorkspace(t *testing.T) {
	root := t.TempDir()
	repos, err := findGitRepos(root)
	if err != nil {
		t.Fatalf("findGitRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos in empty workspace, got %d", len(repos))
	}
}

func TestFindGitRepos_NonExistentRoot(t *testing.T) {
	// Walk of a non-existent root should return an error or empty slice,
	// depending on the OS. We just verify it does not panic.
	_, _ = findGitRepos("/path/that/does/not/exist/xyzzy123")
}

// TestMigrateStripPATFromOrigins_Integration uses actual git repos to verify
// the end-to-end migration path.  It is skipped when the "git" binary is not
// on PATH.
func TestMigrateStripPATFromOrigins_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}

	root := t.TempDir()

	// Helper to init a bare remote and clone it with a PAT-embedded URL.
	initRepo := func(name, tokenURL, cleanURL string) string {
		// Bare "remote" repo
		bareDir := filepath.Join(root, name+".git")
		runGit(t, "", "init", "--bare", bareDir)

		// Clone with embedded PAT URL
		repoDir := filepath.Join(root, name)
		runGit(t, "", "clone", bareDir, repoDir)

		// Rewrite origin to the PAT-embedded URL (simulating what the daemon does)
		runGit(t, repoDir, "remote", "set-url", "origin", tokenURL)

		// Verify we actually have the token URL
		gotURL := getRemoteURL(t, repoDir, "origin")
		if gotURL != tokenURL {
			t.Fatalf("setup: expected remote URL %q, got %q", tokenURL, gotURL)
		}

		return repoDir
	}

	// Repo 1 – GitHub with embedded PAT
	repo1 := initRepo(
		"repo1",
		"https://ghp_ABCDEF@github.com/owner/repo1.git",
		"https://github.com/owner/repo1.git",
	)

	// Repo 2 – already clean (should not be changed)
	repo2 := initRepo(
		"repo2",
		"https://github.com/owner/repo2.git", // no token
		"https://github.com/owner/repo2.git",
	)

	cfg := Default()
	cfg.Workspace.Root = root

	if err := migrateStripPATFromOrigins(&cfg); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// repo1 origin should now be clean
	if got := getRemoteURL(t, repo1, "origin"); got != "https://github.com/owner/repo1.git" {
		t.Errorf("repo1 origin: want clean URL, got %q", got)
	}

	// repo2 origin should be unchanged
	if got := getRemoteURL(t, repo2, "origin"); got != "https://github.com/owner/repo2.git" {
		t.Errorf("repo2 origin: want unchanged URL, got %q", got)
	}
}

func TestMigrateStripPATFromOrigins_NoWorkspaceRoot(t *testing.T) {
	cfg := Default()
	cfg.Workspace.Root = ""

	// Should be a no-op, not an error
	if err := migrateStripPATFromOrigins(&cfg); err != nil {
		t.Errorf("expected nil error for empty workspace root, got %v", err)
	}
}

func TestMigrateStripPATFromOrigins_WorkspaceDoesNotExist(t *testing.T) {
	cfg := Default()
	cfg.Workspace.Root = "/path/that/does/not/exist/xyzzy456"

	// Should be a no-op, not an error
	if err := migrateStripPATFromOrigins(&cfg); err != nil {
		t.Errorf("expected nil error for non-existent workspace, got %v", err)
	}
}

// runGit is a helper that runs a git command, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Suppress interactive prompts
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		fmt.Sprintf("HOME=%s", t.TempDir()),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// getRemoteURL returns the current URL for the named remote.
func getRemoteURL(t *testing.T, repoDir, remote string) string {
	t.Helper()
	cmd := exec.Command("git", "remote", "get-url", remote)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get remote URL for %s: %v", repoDir, err)
	}
	return filepath.ToSlash(strings.TrimSpace(string(out)))
}
