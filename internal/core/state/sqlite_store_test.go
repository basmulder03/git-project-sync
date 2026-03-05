package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreSchemaInitialization(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state", "sync.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
}

func TestSQLiteStoreRepoStateRoundTrip(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	input := RepoState{
		RepoPath:    "/repos/example",
		LastStatus:  "success",
		LastError:   "",
		LastSyncAt:  time.Now().UTC().Truncate(time.Second),
		CurrentHash: "abc123",
	}

	if err := store.PutRepoState(input); err != nil {
		t.Fatalf("put repo state: %v", err)
	}

	got, found, err := store.GetRepoState(input.RepoPath)
	if err != nil {
		t.Fatalf("get repo state: %v", err)
	}
	if !found {
		t.Fatal("expected repo state to exist")
	}

	if got.RepoPath != input.RepoPath {
		t.Fatalf("repo_path = %q, want %q", got.RepoPath, input.RepoPath)
	}
	if got.LastStatus != input.LastStatus {
		t.Fatalf("last_status = %q, want %q", got.LastStatus, input.LastStatus)
	}
	if got.CurrentHash != input.CurrentHash {
		t.Fatalf("current_hash = %q, want %q", got.CurrentHash, input.CurrentHash)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set")
	}
}

func TestSQLiteStoreListsRepoStates(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	_ = store.PutRepoState(RepoState{RepoPath: "/repos/a", LastStatus: "ok", LastError: "", CurrentHash: "a", UpdatedAt: time.Now().UTC()})
	_ = store.PutRepoState(RepoState{RepoPath: "/repos/b", LastStatus: "warn", LastError: "x", CurrentHash: "b", UpdatedAt: time.Now().UTC().Add(time.Second)})

	repos, err := store.ListRepoStates(10)
	if err != nil {
		t.Fatalf("list repo states: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("repo states len=%d want 2", len(repos))
	}
	if repos[0].RepoPath != "/repos/b" {
		t.Fatalf("newest repo_path=%q want /repos/b", repos[0].RepoPath)
	}
}

func TestSQLiteStoreAppendsAndListsEvents(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	first := Event{TraceID: "trace-1", RepoPath: "/repos/a", Level: "info", ReasonCode: "sync_completed", Message: "ok"}
	second := Event{TraceID: "trace-2", RepoPath: "/repos/b", Level: "warn", ReasonCode: "repo_dirty", Message: "skipped"}

	if err := store.AppendEvent(first); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if err := store.AppendEvent(second); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	events, err := store.ListEvents(10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}

	if events[0].TraceID != "trace-2" {
		t.Fatalf("newest trace_id = %q, want trace-2", events[0].TraceID)
	}
	if events[1].TraceID != "trace-1" {
		t.Fatalf("older trace_id = %q, want trace-1", events[1].TraceID)
	}
}

func TestSQLiteStoreListsEventsByTrace(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	_ = store.AppendEvent(Event{TraceID: "trace-a", RepoPath: "/repos/a", Level: "info", ReasonCode: "one", Message: "1"})
	_ = store.AppendEvent(Event{TraceID: "trace-b", RepoPath: "/repos/b", Level: "info", ReasonCode: "two", Message: "2"})
	_ = store.AppendEvent(Event{TraceID: "trace-a", RepoPath: "/repos/a", Level: "warn", ReasonCode: "three", Message: "3"})

	events, err := store.ListEventsByTrace("trace-a", 10)
	if err != nil {
		t.Fatalf("list events by trace: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].TraceID != "trace-a" || events[1].TraceID != "trace-a" {
		t.Fatalf("all events should match trace-a: %+v", events)
	}
}

func TestSQLiteStoreRunStateLifecycle(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	run := RunState{
		RunID:    "run-1",
		TraceID:  "trace-1",
		RepoPath: "/repos/a",
		SourceID: "gh1",
		Status:   "running",
		Note:     "started",
	}

	if err := store.UpsertRunState(run); err != nil {
		t.Fatalf("upsert run state: %v", err)
	}

	inFlight, err := store.ListInFlightRunStates(10)
	if err != nil {
		t.Fatalf("list in-flight runs: %v", err)
	}
	if len(inFlight) != 1 || inFlight[0].RunID != "run-1" {
		t.Fatalf("unexpected in-flight runs: %+v", inFlight)
	}

	if err := store.CompleteRunState("run-1", "completed", "done"); err != nil {
		t.Fatalf("complete run state: %v", err)
	}

	inFlight, err = store.ListInFlightRunStates(10)
	if err != nil {
		t.Fatalf("list in-flight runs after complete: %v", err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected no in-flight runs after completion: %+v", inFlight)
	}
}

func TestSQLiteStoreBackupRestoreAndIntegrityRecovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	backupPath := filepath.Join(dir, "backup", "state-backup.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}

	if err := store.PutRepoState(RepoState{RepoPath: "/repos/a", LastStatus: "ok", LastError: "", CurrentHash: "abc"}); err != nil {
		t.Fatalf("put repo state: %v", err)
	}
	if err := store.IntegrityCheck(); err != nil {
		t.Fatalf("initial integrity check: %v", err)
	}
	if err := store.BackupTo(backupPath, false); err != nil {
		t.Fatalf("backup state db: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store before corruption: %v", err)
	}

	if err := os.WriteFile(dbPath, []byte("not-a-sqlite-db"), 0o600); err != nil {
		t.Fatalf("corrupt db file: %v", err)
	}

	corruptStore, err := NewSQLiteStore(dbPath)
	if err == nil {
		_ = corruptStore.Close()
		t.Fatal("expected opening corrupted db to fail")
	}

	if err := RestoreSQLiteDB(dbPath, backupPath); err != nil {
		t.Fatalf("restore sqlite db: %v", err)
	}

	restored, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })

	if err := restored.IntegrityCheck(); err != nil {
		t.Fatalf("restored integrity check: %v", err)
	}
	if _, found, err := restored.GetRepoState("/repos/a"); err != nil {
		t.Fatalf("get restored repo state: %v", err)
	} else if !found {
		t.Fatal("expected restored repo state to exist")
	}
}
