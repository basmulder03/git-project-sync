package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func TestDaemonRestartStormRecoversInFlightRuns(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	const restarts = 4
	const inFlightPerRestart = 15

	for restart := 0; restart < restarts; restart++ {
		recoverer := daemon.NewRecoverer(store)
		for i := 0; i < inFlightPerRestart; i++ {
			runID := fmt.Sprintf("restart-%d-run-%d", restart, i)
			started, err := recoverer.BeginRun(runID, fmt.Sprintf("trace-restart-%d", restart), fmt.Sprintf("/repos/%d", i), "gh1")
			if err != nil || !started {
				t.Fatalf("begin run failed during restart %d: started=%t err=%v", restart, started, err)
			}
		}

		recovered, err := recoverer.RecoverInFlightRuns(100)
		if err != nil {
			t.Fatalf("recover in-flight runs during restart %d: %v", restart, err)
		}
		if len(recovered) != inFlightPerRestart {
			t.Fatalf("recovered count during restart %d = %d, want %d", restart, len(recovered), inFlightPerRestart)
		}
	}

	inFlight, err := store.ListInFlightRunStates(50)
	if err != nil {
		t.Fatalf("list in-flight runs: %v", err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected zero in-flight runs after restart storm, got %+v", inFlight)
	}
}

func TestDaemonLockContentionSpikeEmitsRepoLockedAndRecovers(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	api := daemon.NewServiceAPI(store)
	locks := daemon.NewRepoLockManager()

	var mu sync.Mutex
	executedHot := 0
	executedNormal := 0

	runRepo := func(_ context.Context, _ string, _ config.SourceConfig, repo config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		time.Sleep(4 * time.Millisecond)
		mu.Lock()
		if repo.Path == "/repos/hot" {
			executedHot++
		} else {
			executedNormal++
		}
		mu.Unlock()
		return coresync.RepoJobResult{RepoPath: repo.Path}, nil
	}

	scheduler := daemon.NewScheduler(config.DaemonConfig{
		IntervalSeconds:         60,
		JitterSeconds:           1,
		MaxParallelRepos:        12,
		MaxParallelPerSource:    6,
		OperationTimeoutSeconds: 20,
		Retry: config.RetryConfig{
			MaxAttempts:        1,
			BaseBackoffSeconds: 1,
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)), locks, runRepo, api)

	tasks := make([]daemon.RepoTask, 0, 60)
	for i := 0; i < 40; i++ {
		tasks = append(tasks, daemon.RepoTask{Source: config.SourceConfig{ID: "gh1"}, Repo: config.RepoConfig{Path: "/repos/hot"}})
	}
	for i := 0; i < 20; i++ {
		tasks = append(tasks, daemon.RepoTask{Source: config.SourceConfig{ID: "gh1"}, Repo: config.RepoConfig{Path: fmt.Sprintf("/repos/normal-%d", i)}})
	}

	traceID := "trace-lock-spike"
	scheduler.RunCycle(context.Background(), traceID, tasks, false)

	mu.Lock()
	if executedHot < 1 {
		mu.Unlock()
		t.Fatalf("expected hot-repo progress under contention, got %d", executedHot)
	}
	if executedHot >= 40 {
		mu.Unlock()
		t.Fatalf("expected contention to skip some hot-repo attempts, got %d", executedHot)
	}
	if executedNormal != 20 {
		mu.Unlock()
		t.Fatalf("expected all normal repos to execute, got %d", executedNormal)
	}
	mu.Unlock()

	events, err := api.Trace(traceID, 200)
	if err != nil {
		t.Fatalf("trace query failed: %v", err)
	}

	lockedEvents := 0
	for _, event := range events {
		if event.ReasonCode == telemetry.ReasonRepoLocked {
			lockedEvents++
		}
	}
	if lockedEvents == 0 {
		t.Fatalf("expected repo_locked events during contention spike, got %d", lockedEvents)
	}

	acquired, err := locks.TryWithLock("/repos/hot", func() error { return nil })
	if err != nil || !acquired {
		t.Fatalf("expected hot repo lock to be reusable after contention spike: acquired=%t err=%v", acquired, err)
	}
}
