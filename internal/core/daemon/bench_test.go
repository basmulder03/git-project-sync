package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

// BenchmarkFairOrderTasks measures the cost of interleaving tasks from
// multiple sources at various scales.
func BenchmarkFairOrderTasks(b *testing.B) {
	cases := []struct {
		sources int
		repos   int
	}{
		{1, 10},
		{4, 100},
		{4, 330},
		{8, 500},
	}

	for _, tc := range cases {
		tc := tc
		name := fmt.Sprintf("sources%d_repos%d", tc.sources, tc.repos)
		b.Run(name, func(b *testing.B) {
			tasks := make([]RepoTask, 0, tc.sources*tc.repos)
			for s := 0; s < tc.sources; s++ {
				srcID := fmt.Sprintf("source-%d", s)
				for r := 0; r < tc.repos; r++ {
					tasks = append(tasks, RepoTask{
						Source: config.SourceConfig{ID: srcID},
						Repo:   config.RepoConfig{Path: fmt.Sprintf("/repos/%s/repo-%d", srcID, r)},
					})
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = fairOrderTasks(tasks)
			}
		})
	}
}

// BenchmarkRunCycle measures end-to-end scheduler cycle throughput with a
// no-op runRepo function (eliminates I/O from the measurement).
func BenchmarkRunCycle(b *testing.B) {
	cases := []struct {
		parallel int
		repos    int
	}{
		{4, 50},
		{8, 200},
		{16, 330},
	}

	for _, tc := range cases {
		tc := tc
		name := fmt.Sprintf("parallel%d_repos%d", tc.parallel, tc.repos)
		b.Run(name, func(b *testing.B) {
			dir := b.TempDir()
			store, err := state.NewSQLiteStore(filepath.Join(dir, "state.db"))
			if err != nil {
				b.Fatalf("new sqlite store: %v", err)
			}
			defer store.Close()

			api := NewServiceAPI(store)
			runRepo := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
				return coresync.RepoJobResult{}, nil
			}

			cfg := config.DaemonConfig{
				MaxParallelRepos:        tc.parallel,
				MaxParallelPerSource:    tc.parallel,
				OperationTimeoutSeconds: 30,
				Retry:                   config.RetryConfig{MaxAttempts: 1, BaseBackoffSeconds: 1},
			}
			sched := NewScheduler(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), runRepo, api)

			tasks := make([]RepoTask, tc.repos)
			for i := range tasks {
				tasks[i] = RepoTask{
					Source: config.SourceConfig{ID: fmt.Sprintf("src-%d", i%4), Enabled: true},
					Repo:   config.RepoConfig{Path: fmt.Sprintf("/repos/repo-%d", i), Enabled: true},
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sched.RunCycle(context.Background(), fmt.Sprintf("bench-trace-%d", i), tasks, false)
			}
		})
	}
}
