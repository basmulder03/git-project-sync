package integration

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func TestDaemonSoakRunCycleMaintainsProgressAcrossSources(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	callsByRepo := map[string]int{}

	runRepo := func(_ context.Context, _ string, _ config.SourceConfig, repo config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		mu.Lock()
		callsByRepo[repo.Path]++
		mu.Unlock()
		return coresync.RepoJobResult{RepoPath: repo.Path}, nil
	}

	scheduler := daemon.NewScheduler(config.DaemonConfig{
		IntervalSeconds:         60,
		JitterSeconds:           1,
		MaxParallelRepos:        6,
		MaxParallelPerSource:    2,
		OperationTimeoutSeconds: 10,
		Retry: config.RetryConfig{
			MaxAttempts:        1,
			BaseBackoffSeconds: 1,
		},
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)), daemon.NewRepoLockManager(), runRepo, nil)

	tasks := []daemon.RepoTask{
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/gh-1"}},
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/gh-2"}},
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/gh-3"}},
		{Source: config.SourceConfig{ID: "az"}, Repo: config.RepoConfig{Path: "/repos/az-1"}},
		{Source: config.SourceConfig{ID: "az"}, Repo: config.RepoConfig{Path: "/repos/az-2"}},
		{Source: config.SourceConfig{ID: "corp"}, Repo: config.RepoConfig{Path: "/repos/corp-1"}},
	}

	const cycles = 20
	for i := 0; i < cycles; i++ {
		scheduler.RunCycle(context.Background(), "trace-soak", tasks, false)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, task := range tasks {
		if got := callsByRepo[task.Repo.Path]; got != cycles {
			t.Fatalf("repo %s call count = %d, want %d", task.Repo.Path, got, cycles)
		}
	}
}
