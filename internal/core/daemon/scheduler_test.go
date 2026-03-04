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
	"github.com/basmulder03/git-project-sync/internal/core/providers"
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

func TestSchedulerAppliesAdaptiveSourceBackoff(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	run := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		if calls.Add(1) == 1 {
			return coresync.RepoJobResult{}, providers.NewRateLimitError("github", 4*time.Second, "throttled")
		}
		return coresync.RepoJobResult{}, nil
	}

	var slept []time.Duration
	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), run, nil)
	s.now = func() time.Time { return time.Unix(100, 0).UTC() }
	s.sleepCtx = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	tasks := []RepoTask{{Source: config.SourceConfig{ID: "gh1"}, Repo: config.RepoConfig{Path: "/repos/a"}}}
	s.RunCycle(context.Background(), "trace-rate-limit", tasks, false)

	if calls.Load() < 2 {
		t.Fatalf("expected retry after rate limit, calls=%d", calls.Load())
	}

	foundAdaptive := false
	for _, duration := range slept {
		if duration >= 4*time.Second {
			foundAdaptive = true
			break
		}
	}
	if !foundAdaptive {
		t.Fatalf("expected adaptive backoff sleep >=4s, slept=%v", slept)
	}
}

func TestSchedulerWaitsForExistingSourceBackoff(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	run := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		calls.Add(1)
		return coresync.RepoJobResult{}, nil
	}

	var slept []time.Duration
	now := time.Unix(200, 0).UTC()
	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), run, nil)
	s.now = func() time.Time { return now }
	s.sleepCtx = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	s.sourceBackoff["gh1"] = now.Add(3 * time.Second)

	tasks := []RepoTask{{Source: config.SourceConfig{ID: "gh1"}, Repo: config.RepoConfig{Path: "/repos/b"}}}
	s.RunCycle(context.Background(), "trace-source-wait", tasks, false)

	if calls.Load() != 1 {
		t.Fatalf("expected repo run after waiting, calls=%d", calls.Load())
	}
	if len(slept) == 0 || slept[0] < 3*time.Second {
		t.Fatalf("expected scheduler to wait for source backoff, slept=%v", slept)
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
