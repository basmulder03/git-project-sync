package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func TestSchedulerRetriesFailedTask(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	run := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		if calls.Add(1) < 3 {
			return coresync.RepoJobResult{}, errors.New("transient")
		}
		return coresync.RepoJobResult{}, nil
	}

	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), run, nil)
	s.sleepCtx = func(_ context.Context, _ time.Duration) error { return nil }

	s.RunCycle(context.Background(), "trace-1", []RepoTask{{Repo: config.RepoConfig{Path: "/repos/a"}}}, false)

	if got := calls.Load(); got != 3 {
		t.Fatalf("retry calls = %d, want 3", got)
	}
}

func TestSchedulerSkipsWhenRepoLocked(t *testing.T) {
	t.Parallel()

	locks := NewRepoLockManager()
	block := make(chan struct{})

	go func() {
		_, _ = locks.TryWithLock("/repos/a", func() error {
			<-block
			return nil
		})
	}()

	var calls atomic.Int32
	run := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		calls.Add(1)
		return coresync.RepoJobResult{}, nil
	}

	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), locks, run, nil)
	s.sleepCtx = func(_ context.Context, _ time.Duration) error { return nil }

	time.Sleep(10 * time.Millisecond)
	s.RunCycle(context.Background(), "trace-2", []RepoTask{{Repo: config.RepoConfig{Path: "/repos/a"}}}, false)
	close(block)

	if got := calls.Load(); got != 0 {
		t.Fatalf("runner should not be called while locked, got %d", got)
	}
}

func TestSchedulerIntervalUsesJitter(t *testing.T) {
	t.Parallel()

	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), func(context.Context, string, config.SourceConfig, config.RepoConfig, bool) (coresync.RepoJobResult, error) {
		return coresync.RepoJobResult{}, nil
	}, nil)

	for i := 0; i < 20; i++ {
		next := s.nextInterval()
		if next < 5*time.Second || next > 7*time.Second {
			t.Fatalf("next interval %s outside expected range [5s,7s]", next)
		}
	}
}

func testDaemonConfig() config.DaemonConfig {
	return config.DaemonConfig{
		IntervalSeconds:         5,
		JitterSeconds:           2,
		MaxParallelRepos:        2,
		OperationTimeoutSeconds: 5,
		Retry: config.RetryConfig{
			MaxAttempts:        3,
			BaseBackoffSeconds: 1,
		},
	}
}
