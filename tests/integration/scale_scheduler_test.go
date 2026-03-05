package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func TestSchedulerScaleFairnessAcrossMultipleSources(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	api := daemon.NewServiceAPI(store)

	bySource := map[string]int{
		"gh-personal": 120,
		"gh-org":      90,
		"az-team":     70,
		"az-corp":     50,
	}

	tasks := make([]daemon.RepoTask, 0, 330)
	for sourceID, count := range bySource {
		for i := 0; i < count; i++ {
			tasks = append(tasks, daemon.RepoTask{
				Source: config.SourceConfig{ID: sourceID, Enabled: true},
				Repo:   config.RepoConfig{Path: fmt.Sprintf("/repos/%s/repo-%d", sourceID, i), Enabled: true},
			})
		}
	}

	var mu sync.Mutex
	completed := make(map[string]int, len(bySource))
	runRepo := func(_ context.Context, _ string, source config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		mu.Lock()
		completed[source.ID]++
		mu.Unlock()
		return coresync.RepoJobResult{}, nil
	}

	scheduler := daemon.NewScheduler(config.DaemonConfig{
		IntervalSeconds:         60,
		JitterSeconds:           1,
		MaxParallelRepos:        16,
		MaxParallelPerSource:    3,
		OperationTimeoutSeconds: 30,
		Retry: config.RetryConfig{
			MaxAttempts:        1,
			BaseBackoffSeconds: 1,
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)), daemon.NewRepoLockManager(), runRepo, api)

	traceID := "trace-scale-fairness"
	scheduler.RunCycle(context.Background(), traceID, tasks, false)

	mu.Lock()
	defer mu.Unlock()
	for sourceID, want := range bySource {
		if got := completed[sourceID]; got != want {
			t.Fatalf("source %s completion count = %d, want %d", sourceID, got, want)
		}
	}

	events, err := api.Trace(traceID, 500)
	if err != nil {
		t.Fatalf("query trace events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected trace events for scale cycle, got %d", len(events))
	}
}
