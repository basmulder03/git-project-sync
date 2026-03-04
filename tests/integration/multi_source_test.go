package integration

import (
	"context"
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
)

func TestSchedulerRoutesRepositoriesToCorrectSourcesAndRecordsTrace(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	api := daemon.NewServiceAPI(store)

	var mu sync.Mutex
	routed := map[string]string{}
	runRepo := func(_ context.Context, _ string, source config.SourceConfig, repo config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		mu.Lock()
		routed[repo.Path] = source.ID
		mu.Unlock()
		return coresync.RepoJobResult{RepoPath: repo.Path}, nil
	}

	scheduler := daemon.NewScheduler(config.DaemonConfig{
		IntervalSeconds:         60,
		JitterSeconds:           1,
		MaxParallelRepos:        1,
		OperationTimeoutSeconds: 30,
		Retry: config.RetryConfig{
			MaxAttempts:        1,
			BaseBackoffSeconds: 1,
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)), daemon.NewRepoLockManager(), runRepo, api)

	traceID := "trace-integration-2"
	tasks := []daemon.RepoTask{
		{Source: config.SourceConfig{ID: "gh-personal", Provider: "github"}, Repo: config.RepoConfig{Path: "/repos/a"}},
		{Source: config.SourceConfig{ID: "az-work", Provider: "azuredevops"}, Repo: config.RepoConfig{Path: "/repos/b"}},
	}

	scheduler.RunCycle(context.Background(), traceID, tasks, false)

	mu.Lock()
	if routed["/repos/a"] != "gh-personal" || routed["/repos/b"] != "az-work" {
		t.Fatalf("unexpected source routing: %+v", routed)
	}
	mu.Unlock()

	events, err := api.Trace(traceID, 10)
	if err != nil {
		t.Fatalf("trace query failed: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected trace events for both repos, got %d", len(events))
	}

	for _, event := range events {
		if event.CreatedAt.IsZero() {
			t.Fatal("trace event timestamp should be set")
		}
		if event.CreatedAt.After(time.Now().Add(1 * time.Minute)) {
			t.Fatalf("trace event timestamp looks invalid: %s", event.CreatedAt)
		}
	}
}
