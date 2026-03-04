package state

import (
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
