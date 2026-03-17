package state

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkAppendEvent measures the throughput of sequential event writes,
// which is the hottest write path during a scheduler cycle.
func BenchmarkAppendEvent(b *testing.B) {
	store, err := newTestStore(b)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	defer store.Close()

	evt := Event{
		TraceID:    "bench-trace",
		RepoPath:   "/repos/bench",
		Level:      "info",
		ReasonCode: "sync_completed",
		Message:    "sync completed",
		CreatedAt:  time.Now().UTC(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evt.TraceID = fmt.Sprintf("trace-%d", i)
		if err := store.AppendEvent(evt); err != nil {
			b.Fatalf("AppendEvent: %v", err)
		}
	}
}

// BenchmarkListEvents measures the cost of fetching the most-recent events,
// the read path exercised by TUI and metrics scrape.
func BenchmarkListEvents(b *testing.B) {
	store, err := newTestStore(b)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	defer store.Close()

	// Pre-populate with 1 000 events.
	for i := 0; i < 1000; i++ {
		_ = store.AppendEvent(Event{
			TraceID:    fmt.Sprintf("trace-%d", i),
			RepoPath:   fmt.Sprintf("/repos/repo-%d", i%50),
			Level:      "info",
			ReasonCode: "sync_completed",
			Message:    "sync completed",
			CreatedAt:  time.Now().UTC(),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.ListEvents(100); err != nil {
			b.Fatalf("ListEvents: %v", err)
		}
	}
}

// BenchmarkListEventsByTrace measures trace-ID lookups, used by the Trace()
// API and diagnositcs queries.  With the new idx_events_trace_id index this
// should be an index seek rather than a full table scan.
func BenchmarkListEventsByTrace(b *testing.B) {
	store, err := newTestStore(b)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	defer store.Close()

	// Pre-populate with 2 000 events across 20 traces.
	for i := 0; i < 2000; i++ {
		_ = store.AppendEvent(Event{
			TraceID:    fmt.Sprintf("trace-%d", i%20),
			RepoPath:   "/repos/bench",
			Level:      "info",
			ReasonCode: "sync_completed",
			Message:    "sync completed",
			CreatedAt:  time.Now().UTC(),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.ListEventsByTrace(fmt.Sprintf("trace-%d", i%20), 200); err != nil {
			b.Fatalf("ListEventsByTrace: %v", err)
		}
	}
}

// BenchmarkPutRepoState measures upsert throughput for the repo-state table.
func BenchmarkPutRepoState(b *testing.B) {
	store, err := newTestStore(b)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs := RepoState{
			RepoPath:    fmt.Sprintf("/repos/repo-%d", i%100),
			LastStatus:  "ok",
			LastError:   "",
			LastSyncAt:  time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			CurrentHash: fmt.Sprintf("abc%06d", i),
		}
		if err := store.PutRepoState(rs); err != nil {
			b.Fatalf("PutRepoState: %v", err)
		}
	}
}

// BenchmarkListRepoStates measures the ordered listing used by dashboard and CLI.
func BenchmarkListRepoStates(b *testing.B) {
	store, err := newTestStore(b)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	defer store.Close()

	for i := 0; i < 200; i++ {
		_ = store.PutRepoState(RepoState{
			RepoPath:   fmt.Sprintf("/repos/repo-%d", i),
			LastStatus: "ok",
			UpdatedAt:  time.Now().UTC(),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.ListRepoStates(100); err != nil {
			b.Fatalf("ListRepoStates: %v", err)
		}
	}
}

// newTestStore is a benchmark-local helper (mirrors the test helper pattern).
func newTestStore(b *testing.B) (*SQLiteStore, error) {
	b.Helper()
	return NewSQLiteStore(filepath.Join(b.TempDir(), "bench.db"))
}
