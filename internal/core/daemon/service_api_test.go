package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func TestServiceAPIRecordListAndTrace(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	api := NewServiceAPI(store)
	now := time.Now().UTC()

	if err := api.RecordEvent(context.Background(), telemetry.Event{TraceID: "trace-1", RepoPath: "/repos/a", Level: "info", ReasonCode: telemetry.ReasonSyncCompleted, Message: "done", CreatedAt: now}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if err := api.RecordEvent(context.Background(), telemetry.Event{TraceID: "trace-2", RepoPath: "/repos/b", Level: "warn", ReasonCode: telemetry.ReasonRepoLocked, Message: "locked", CreatedAt: now.Add(time.Second)}); err != nil {
		t.Fatalf("record event: %v", err)
	}

	events, err := api.ListEvents(10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d want 2", len(events))
	}

	trace, err := api.Trace("trace-1", 10)
	if err != nil {
		t.Fatalf("trace query: %v", err)
	}
	if len(trace) != 1 || trace[0].TraceID != "trace-1" {
		t.Fatalf("unexpected trace events: %+v", trace)
	}
}
