package daemon

import (
	"fmt"
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

func TestRecovererHandlesRestartStormRecovery(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	const cycles = 5
	const runsPerCycle = 20

	for cycle := 0; cycle < cycles; cycle++ {
		recoverer := NewRecoverer(store)
		for i := 0; i < runsPerCycle; i++ {
			runID := fmt.Sprintf("storm-%d-%d", cycle, i)
			started, err := recoverer.BeginRun(runID, fmt.Sprintf("trace-%d", cycle), "/repos/a", "gh1")
			if err != nil || !started {
				t.Fatalf("begin run failed in cycle %d: started=%t err=%v", cycle, started, err)
			}
		}

		recovered, err := recoverer.RecoverInFlightRuns(200)
		if err != nil {
			t.Fatalf("recover in-flight runs in cycle %d: %v", cycle, err)
		}
		if len(recovered) != runsPerCycle {
			t.Fatalf("recovered len in cycle %d = %d, want %d", cycle, len(recovered), runsPerCycle)
		}
	}

	inFlight, err := store.ListInFlightRunStates(50)
	if err != nil {
		t.Fatalf("list in-flight after storm recovery: %v", err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected no in-flight runs after recovery storm, got %+v", inFlight)
	}

	// Simulate a post-restart run beginning with a previously used run ID.
	recoverer := NewRecoverer(store)
	started, err := recoverer.BeginRun("storm-0-0", "trace-post", "/repos/a", "gh1")
	if err != nil || !started {
		t.Fatalf("expected reused run id to be allowed after process restart: started=%t err=%v", started, err)
	}
}

func TestRecovererCompleteRun(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	recoverer := NewRecoverer(store)

	// Start a run
	started, err := recoverer.BeginRun("run-complete-1", "trace-complete", "/repos/test", "source-1")
	if err != nil || !started {
		t.Fatalf("begin run failed: started=%t err=%v", started, err)
	}

	// Complete the run successfully
	err = recoverer.CompleteRun("run-complete-1", "success", "completed successfully")
	if err != nil {
		t.Fatalf("complete run failed: %v", err)
	}

	// Verify run is no longer in-flight
	inFlight, err := store.ListInFlightRunStates(10)
	if err != nil {
		t.Fatalf("list in-flight runs: %v", err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected no in-flight runs after completion, got %d", len(inFlight))
	}
}

func TestRecovererCompleteRunWithError(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	recoverer := NewRecoverer(store)

	// Start a run
	started, err := recoverer.BeginRun("run-error-1", "trace-error", "/repos/test", "source-1")
	if err != nil || !started {
		t.Fatalf("begin run failed: started=%t err=%v", started, err)
	}

	// Complete the run with error status
	err = recoverer.CompleteRun("run-error-1", "error", "sync failed due to network issue")
	if err != nil {
		t.Fatalf("complete run failed: %v", err)
	}

	// Verify run is no longer in-flight
	inFlight, err := store.ListInFlightRunStates(10)
	if err != nil {
		t.Fatalf("list in-flight runs: %v", err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected no in-flight runs after completion, got %d", len(inFlight))
	}
}
