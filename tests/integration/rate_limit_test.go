package integration

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/providers"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func TestRateLimitRetryEmitsProviderBackoffReason(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	api := daemon.NewServiceAPI(store)

	var calls atomic.Int32
	runRepo := func(_ context.Context, _ string, _ config.SourceConfig, repo config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		if repo.Path == "/repos/rate-limited" && calls.Add(1) == 1 {
			return coresync.RepoJobResult{}, providers.NewRateLimitError("github", 3*time.Second, "throttled")
		}
		return coresync.RepoJobResult{RepoPath: repo.Path}, nil
	}

	scheduler := daemon.NewScheduler(config.DaemonConfig{
		IntervalSeconds:         60,
		JitterSeconds:           1,
		MaxParallelRepos:        1,
		MaxParallelPerSource:    1,
		OperationTimeoutSeconds: 10,
		Retry: config.RetryConfig{
			MaxAttempts:        3,
			BaseBackoffSeconds: 0,
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)), daemon.NewRepoLockManager(), runRepo, api)

	traceID := "trace-rate-limit-integration"
	tasks := []daemon.RepoTask{{
		Source: config.SourceConfig{ID: "gh", Provider: "github"},
		Repo:   config.RepoConfig{Path: "/repos/rate-limited"},
	}}

	scheduler.RunCycle(context.Background(), traceID, tasks, false)

	events, err := api.Trace(traceID, 20)
	if err != nil {
		t.Fatalf("trace query failed: %v", err)
	}

	hasRateLimited := false
	hasCompleted := false
	for _, event := range events {
		switch event.ReasonCode {
		case telemetry.ReasonProviderRateLimited:
			hasRateLimited = true
		case telemetry.ReasonSyncCompleted:
			hasCompleted = true
		}
	}

	if !hasRateLimited {
		t.Fatal("expected provider_rate_limited event after throttling")
	}
	if !hasCompleted {
		t.Fatal("expected sync_completed event after retry")
	}
}
