package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestStateBackupCheckAndRestoreCommands(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "state", "sync.db")
	backupPath := filepath.Join(dir, "backup", "sync-backup.db")

	cfg := config.Default()
	cfg.State.DBPath = dbPath
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store, err := state.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.PutRepoState(state.RepoState{RepoPath: "/repos/a", LastStatus: "ok", CurrentHash: "abc"}); err != nil {
		_ = store.Close()
		t.Fatalf("seed repo state: %v", err)
	}
	_ = store.Close()

	backupOut, err := executeSyncctl("--config", configPath, "state", "backup", "--output", backupPath)
	if err != nil {
		t.Fatalf("state backup failed: %v output=%s", err, backupOut)
	}
	if !strings.Contains(backupOut, "state backup created") {
		t.Fatalf("unexpected backup output: %s", backupOut)
	}

	checkOut, err := executeSyncctl("--config", configPath, "state", "check")
	if err != nil {
		t.Fatalf("state check failed: %v output=%s", err, checkOut)
	}
	if !strings.Contains(checkOut, "integrity: ok") {
		t.Fatalf("unexpected check output: %s", checkOut)
	}

	if err := state.RestoreSQLiteDB(dbPath, backupPath); err != nil {
		t.Fatalf("direct restore precondition failed: %v", err)
	}

	restoreOut, err := executeSyncctl("--config", configPath, "state", "restore", "--input", backupPath)
	if err != nil {
		t.Fatalf("state restore failed: %v output=%s", err, restoreOut)
	}
	if !strings.Contains(restoreOut, "state backup restored") {
		t.Fatalf("unexpected restore output: %s", restoreOut)
	}
}
