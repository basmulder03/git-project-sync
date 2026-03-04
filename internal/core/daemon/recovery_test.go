package daemon

import (
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestRecovererBeginRunPreventsDuplicateRunID(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	recoverer := NewRecoverer(store)
	started, err := recoverer.BeginRun("run-1", "trace-1", "/repos/a", "gh1")
	if err != nil || !started {
		t.Fatalf("first begin run failed: started=%t err=%v", started, err)
	}

	started, err = recoverer.BeginRun("run-1", "trace-1", "/repos/a", "gh1")
	if err != nil {
		t.Fatalf("second begin run unexpected err: %v", err)
	}
	if started {
		t.Fatal("duplicate run id should not start twice")
	}
}

func TestRecovererMarksInFlightRunsAsRecovered(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	recoverer := NewRecoverer(store)
	_, _ = recoverer.BeginRun("run-1", "trace-1", "/repos/a", "gh1")

	recovered, err := recoverer.RecoverInFlightRuns(10)
	if err != nil {
		t.Fatalf("recover in-flight runs: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered len=%d want 1", len(recovered))
	}

	inFlight, err := store.ListInFlightRunStates(10)
	if err != nil {
		t.Fatalf("list in-flight after recovery: %v", err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected no in-flight runs after recovery, got %+v", inFlight)
	}
}
