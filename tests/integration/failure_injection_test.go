package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func TestFailureInjectionClassifiesTransientAndPermanentErrors(t *testing.T) {
	t.Parallel()

	store, err := state.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	api := daemon.NewServiceAPI(store)

	var mu sync.Mutex
	repoAttempts := map[string]int{}
	runRepo := func(_ context.Context, _ string, _ config.SourceConfig, repo config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		mu.Lock()
		repoAttempts[repo.Path]++
		attempt := repoAttempts[repo.Path]
		mu.Unlock()

		switch repo.Path {
		case "/repos/flaky":
			if attempt < 3 {
				return coresync.RepoJobResult{}, net.UnknownNetworkError("tcp")
			}
			return coresync.RepoJobResult{RepoPath: repo.Path}, nil
		case "/repos/permanent":
			return coresync.RepoJobResult{}, errors.New("invalid request payload")
		default:
			return coresync.RepoJobResult{RepoPath: repo.Path}, nil
		}
	}

	scheduler := daemon.NewScheduler(config.DaemonConfig{
		IntervalSeconds:         60,
		JitterSeconds:           1,
		MaxParallelRepos:        2,
		MaxParallelPerSource:    1,
		OperationTimeoutSeconds: 10,
		Retry: config.RetryConfig{
			MaxAttempts:        3,
			BaseBackoffSeconds: 0,
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)), daemon.NewRepoLockManager(), runRepo, api)

	traceID := "trace-failure-injection"
	tasks := []daemon.RepoTask{
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/flaky"}},
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/permanent"}},
	}

	scheduler.RunCycle(context.Background(), traceID, tasks, false)

	events, err := api.Trace(traceID, 20)
	if err != nil {
		t.Fatalf("trace query failed: %v", err)
	}

	hasRetry := false
	hasPermanent := false
	hasCompletion := false
	for _, event := range events {
		switch event.ReasonCode {
		case telemetry.ReasonSyncRetry:
			hasRetry = true
		case telemetry.ReasonPermanentError:
			hasPermanent = true
		case telemetry.ReasonSyncCompleted:
			hasCompletion = true
		}
	}

	if !hasRetry {
		t.Fatal("expected transient failure retry event")
	}
	if !hasPermanent {
		t.Fatal("expected permanent failure event")
	}
	if !hasCompletion {
		t.Fatal("expected completion event for recovered flaky repo")
	}
}
