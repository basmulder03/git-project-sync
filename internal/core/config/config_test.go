package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `schema_version: 1
daemon:
  interval_seconds: 300
  max_parallel_repos: 2
  retry:
    max_attempts: 3
`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", cfg.SchemaVersion, CurrentSchemaVersion)
	}

	if cfg.Logging.Format != "json" {
		t.Fatalf("logging.format = %q, want json", cfg.Logging.Format)
	}

	if cfg.State.DBPath == "" {
		t.Fatal("state.db_path should default to non-empty path")
	}
}

func TestLoadRejectsUnsupportedSchema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `schema_version: 2
daemon:
  interval_seconds: 300
  max_parallel_repos: 2
  retry:
    max_attempts: 3
`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestLoadRejectsEmptyStateDBPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `schema_version: 1
state:
  db_path: ""
daemon:
  interval_seconds: 300
  max_parallel_repos: 2
  retry:
    max_attempts: 3
`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected state db path validation error")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	cfg := Default()
	cfg.Workspace.Root = "/tmp/workspace"
	cfg.Sources = []SourceConfig{{
		ID:       "gh-personal",
		Provider: "github",
		Account:  "jane-doe",
		Host:     "github.com",
		Enabled:  true,
	}}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.Workspace.Root != "/tmp/workspace" {
		t.Fatalf("workspace.root = %q, want /tmp/workspace", loaded.Workspace.Root)
	}
	if len(loaded.Sources) != 1 || loaded.Sources[0].ID != "gh-personal" {
		t.Fatalf("unexpected sources after round-trip: %+v", loaded.Sources)
	}
}
