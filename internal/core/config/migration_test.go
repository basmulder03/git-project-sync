package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateStateDatabaseToAppData_NoOldDatabase(t *testing.T) {
	// Setup: no old database exists
	cfg := Default()

	// Should not error when no old database exists
	if err := migrateStateDatabaseToAppData(&cfg); err != nil {
		t.Errorf("migrateStateDatabaseToAppData should not error when no old database exists: %v", err)
	}
}

func TestMigrateStateDatabaseToAppData_FromWorkspaceRoot(t *testing.T) {
	// Setup: create a temporary workspace with old database
	workspaceRoot := t.TempDir()
	oldDBDir := filepath.Join(workspaceRoot, "state")
	oldDBPath := filepath.Join(oldDBDir, "sync.db")

	// Create old database
	if err := os.MkdirAll(oldDBDir, 0o755); err != nil {
		t.Fatalf("create old db directory: %v", err)
	}

	oldContent := []byte("old database content")
	if err := os.WriteFile(oldDBPath, oldContent, 0o600); err != nil {
		t.Fatalf("write old database: %v", err)
	}

	// Setup config with workspace root and new DB location
	newDataDir := t.TempDir()
	newDBPath := filepath.Join(newDataDir, "state", "sync.db")

	cfg := Default()
	cfg.Workspace.Root = workspaceRoot
	cfg.State.DBPath = newDBPath

	// Run migration
	if err := migrateStateDatabaseToAppData(&cfg); err != nil {
		t.Fatalf("migrateStateDatabaseToAppData failed: %v", err)
	}

	// Verify new database exists with same content
	newContent, err := os.ReadFile(newDBPath)
	if err != nil {
		t.Fatalf("read new database: %v", err)
	}

	if string(newContent) != string(oldContent) {
		t.Errorf("new database content = %q, want %q", newContent, oldContent)
	}

	// Verify old database is removed
	if _, err := os.Stat(oldDBPath); err == nil {
		t.Error("old database should be removed after migration")
	}
}

func TestMigrateStateDatabaseToAppData_WithWALFiles(t *testing.T) {
	// Setup: create old database with WAL files
	workspaceRoot := t.TempDir()
	oldDBDir := filepath.Join(workspaceRoot, "state")
	oldDBPath := filepath.Join(oldDBDir, "sync.db")

	if err := os.MkdirAll(oldDBDir, 0o755); err != nil {
		t.Fatalf("create old db directory: %v", err)
	}

	// Create database and WAL files
	files := map[string]string{
		oldDBPath:              "database",
		oldDBPath + "-wal":     "wal content",
		oldDBPath + "-shm":     "shm content",
		oldDBPath + "-journal": "journal content",
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Setup config
	newDataDir := t.TempDir()
	newDBPath := filepath.Join(newDataDir, "state", "sync.db")

	cfg := Default()
	cfg.Workspace.Root = workspaceRoot
	cfg.State.DBPath = newDBPath

	// Run migration
	if err := migrateStateDatabaseToAppData(&cfg); err != nil {
		t.Fatalf("migrateStateDatabaseToAppData failed: %v", err)
	}

	// Verify all files migrated
	expectedFiles := map[string]string{
		newDBPath:              "database",
		newDBPath + "-wal":     "wal content",
		newDBPath + "-shm":     "shm content",
		newDBPath + "-journal": "journal content",
	}

	for path, expectedContent := range expectedFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("%s content = %q, want %q", path, content, expectedContent)
		}
	}

	// Verify old files are removed
	for path := range files {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("old file %s should be removed after migration", path)
		}
	}
}

func TestMigrateStateDatabaseToAppData_NewLocationExists(t *testing.T) {
	// Setup: both old and new databases exist
	workspaceRoot := t.TempDir()
	oldDBPath := filepath.Join(workspaceRoot, "state", "sync.db")

	if err := os.MkdirAll(filepath.Dir(oldDBPath), 0o755); err != nil {
		t.Fatalf("create old db directory: %v", err)
	}

	oldContent := []byte("old database")
	if err := os.WriteFile(oldDBPath, oldContent, 0o600); err != nil {
		t.Fatalf("write old database: %v", err)
	}

	// Create new database
	newDataDir := t.TempDir()
	newDBPath := filepath.Join(newDataDir, "state", "sync.db")

	if err := os.MkdirAll(filepath.Dir(newDBPath), 0o755); err != nil {
		t.Fatalf("create new db directory: %v", err)
	}

	newContent := []byte("new database")
	if err := os.WriteFile(newDBPath, newContent, 0o600); err != nil {
		t.Fatalf("write new database: %v", err)
	}

	// Setup config
	cfg := Default()
	cfg.Workspace.Root = workspaceRoot
	cfg.State.DBPath = newDBPath

	// Run migration
	if err := migrateStateDatabaseToAppData(&cfg); err != nil {
		t.Fatalf("migrateStateDatabaseToAppData failed: %v", err)
	}

	// Verify new database is unchanged (not overwritten)
	content, err := os.ReadFile(newDBPath)
	if err != nil {
		t.Fatalf("read new database: %v", err)
	}

	if string(content) != string(newContent) {
		t.Errorf("new database should not be overwritten, got %q, want %q", content, newContent)
	}
}

func TestMigrateStateDatabaseToAppData_AlreadyAbsolutePath(t *testing.T) {
	// Setup: database already at absolute path
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "state", "sync.db")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("create db directory: %v", err)
	}

	content := []byte("existing database")
	if err := os.WriteFile(dbPath, content, 0o600); err != nil {
		t.Fatalf("write database: %v", err)
	}

	// Config with absolute path
	cfg := Default()
	cfg.State.DBPath = dbPath

	// Run migration
	if err := migrateStateDatabaseToAppData(&cfg); err != nil {
		t.Fatalf("migrateStateDatabaseToAppData failed: %v", err)
	}

	// Verify database unchanged
	newContent, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read database: %v", err)
	}

	if string(newContent) != string(content) {
		t.Errorf("database should be unchanged, got %q, want %q", newContent, content)
	}
}
