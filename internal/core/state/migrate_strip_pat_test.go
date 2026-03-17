package state

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestRemoveEmbeddedCredentialFromURL verifies the URL-cleaning helper used by
// the schema migration.
func TestRemoveEmbeddedCredentialFromURL(t *testing.T) {
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
			name:    "clean github URL",
			input:   "https://github.com/owner/repo.git",
			want:    "https://github.com/owner/repo.git",
			changed: false,
		},
		{
			name:    "azure devops clean URL",
			input:   "https://dev.azure.com/org/project/_git/repo",
			want:    "https://dev.azure.com/org/project/_git/repo",
			changed: false,
		},
		{
			name:    "SSH URL unchanged",
			input:   "git@github.com:owner/repo.git",
			want:    "git@github.com:owner/repo.git",
			changed: false,
		},
		{
			name:    "github enterprise with PAT",
			input:   "https://token123@ghe.corp.com/owner/repo.git",
			want:    "https://ghe.corp.com/owner/repo.git",
			changed: true,
		},
		{
			name:    "empty string unchanged",
			input:   "",
			want:    "",
			changed: false,
		},
		{
			name:    "http (non-https) unchanged",
			input:   "http://pat@example.com/repo.git",
			want:    "http://pat@example.com/repo.git",
			changed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := removeEmbeddedCredentialFromURL(tc.input)
			if got != tc.want {
				t.Errorf("url = %q, want %q", got, tc.want)
			}
			if changed != tc.changed {
				t.Errorf("changed = %v, want %v", changed, tc.changed)
			}
		})
	}
}

// TestSchemaVersion4_StripsPATsFromDiscoveredRepos verifies that upgrading an
// old (version 3) database to version 4 rewrites embedded-credential clone
// URLs in discovered_repos while leaving clean URLs untouched.
func TestSchemaVersion4_StripsPATsFromDiscoveredRepos(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")

	// ── Phase 1: create a v3 database with raw data ─────────────────────────
	store3, err := newSQLiteStoreAtVersion(dbPath, 3)
	if err != nil {
		t.Fatalf("create v3 store: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Insert a row that still has the PAT embedded.
	if err := store3.UpsertDiscoveredRepo(DiscoveredRepo{
		Provider:      "github",
		SourceID:      "gh-1",
		FullName:      "owner/secret-repo",
		CloneURL:      "https://ghp_TOKEN@github.com/owner/secret-repo.git",
		DefaultBranch: "main",
		DiscoveredAt:  now,
	}); err != nil {
		t.Fatalf("upsert dirty repo: %v", err)
	}

	// Insert a row that is already clean.
	if err := store3.UpsertDiscoveredRepo(DiscoveredRepo{
		Provider:      "github",
		SourceID:      "gh-1",
		FullName:      "owner/clean-repo",
		CloneURL:      "https://github.com/owner/clean-repo.git",
		DefaultBranch: "main",
		DiscoveredAt:  now,
	}); err != nil {
		t.Fatalf("upsert clean repo: %v", err)
	}

	if err := store3.Close(); err != nil {
		t.Fatalf("close v3 store: %v", err)
	}

	// ── Phase 2: open the same DB, let EnsureSchema upgrade it to v4 ────────
	store4, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open v4 store: %v", err)
	}
	t.Cleanup(func() { _ = store4.Close() })

	// ── Phase 3: verify the URLs ─────────────────────────────────────────────
	repos, err := store4.ListDiscoveredRepos("gh-1", 10)
	if err != nil {
		t.Fatalf("list discovered repos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	byName := make(map[string]DiscoveredRepo, 2)
	for _, r := range repos {
		byName[r.FullName] = r
	}

	secretRepo, ok := byName["owner/secret-repo"]
	if !ok {
		t.Fatal("owner/secret-repo not found")
	}
	if want := "https://github.com/owner/secret-repo.git"; secretRepo.CloneURL != want {
		t.Errorf("secret-repo CloneURL = %q, want %q", secretRepo.CloneURL, want)
	}

	cleanRepo, ok := byName["owner/clean-repo"]
	if !ok {
		t.Fatal("owner/clean-repo not found")
	}
	if want := "https://github.com/owner/clean-repo.git"; cleanRepo.CloneURL != want {
		t.Errorf("clean-repo CloneURL = %q, want %q", cleanRepo.CloneURL, want)
	}
}

// newSQLiteStoreAtVersion opens (or creates) the SQLite DB at dbPath and
// runs only the schema migrations up to and including targetVersion.
// This lets tests seed an older-version DB before the migration under test
// is applied.
func newSQLiteStoreAtVersion(dbPath string, targetVersion int) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &SQLiteStore{db: db, dbPath: dbPath}

	row := db.QueryRow(`PRAGMA user_version;`)
	var version int
	if err := row.Scan(&version); err != nil {
		_ = db.Close()
		return nil, err
	}

	for version < targetVersion {
		next := version + 1
		if err := store.applyMigration(next); err != nil {
			_ = db.Close()
			return nil, err
		}
		version = next
	}

	return store, nil
}
