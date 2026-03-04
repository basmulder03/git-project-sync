package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"reflect"
	"sync"
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
			return coresync.RepoJobResult{}, net.UnknownNetworkError("tcp")
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

func TestSchedulerDoesNotRetryPermanentErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	run := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		calls.Add(1)
		return coresync.RepoJobResult{}, errors.New("invalid request payload")
	}

	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), run, nil)
	s.sleepCtx = func(_ context.Context, _ time.Duration) error { return nil }

	s.RunCycle(context.Background(), "trace-permanent", []RepoTask{{Source: config.SourceConfig{ID: "gh1"}, Repo: config.RepoConfig{Path: "/repos/c"}}}, false)

	if calls.Load() != 1 {
		t.Fatalf("permanent errors should not be retried, calls=%d", calls.Load())
	}
}

func TestSchedulerStopsWhenRetryBudgetExceeded(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	run := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		calls.Add(1)
		return coresync.RepoJobResult{}, providers.NewRateLimitError("github", 100*time.Millisecond, "throttled")
	}

	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), run, nil)
	s.cfg.OperationTimeoutSeconds = 1
	s.cfg.Retry.MaxAttempts = 10
	s.sleepCtx = func(_ context.Context, _ time.Duration) error { return nil }

	s.RunCycle(context.Background(), "trace-budget", []RepoTask{{Source: config.SourceConfig{ID: "gh1"}, Repo: config.RepoConfig{Path: "/repos/d"}}}, false)

	if calls.Load() >= 10 {
		t.Fatalf("expected retries to stop before max attempts due to budget, calls=%d", calls.Load())
	}
}

func TestFairOrderTasksRoundRobinAcrossSources(t *testing.T) {
	t.Parallel()

	tasks := []RepoTask{
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/a"}},
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/b"}},
		{Source: config.SourceConfig{ID: "az"}, Repo: config.RepoConfig{Path: "/repos/c"}},
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/d"}},
		{Source: config.SourceConfig{ID: "az"}, Repo: config.RepoConfig{Path: "/repos/e"}},
	}

	ordered := fairOrderTasks(tasks)
	got := make([]string, 0, len(ordered))
	for _, task := range ordered {
		got = append(got, task.Repo.Path)
	}

	want := []string{"/repos/a", "/repos/c", "/repos/b", "/repos/e", "/repos/d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fair order = %v, want %v", got, want)
	}
}

func TestSchedulerRespectsPerSourceConcurrencyLimit(t *testing.T) {
	t.Parallel()

	start := make(chan struct{})
	finish := make(chan struct{})

	var activeBySource sync.Map
	var maxBySource sync.Map

	run := func(_ context.Context, _ string, source config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		countAny, _ := activeBySource.LoadOrStore(source.ID, &atomic.Int32{})
		count := countAny.(*atomic.Int32)
		active := count.Add(1)

		maxAny, _ := maxBySource.LoadOrStore(source.ID, &atomic.Int32{})
		maxCount := maxAny.(*atomic.Int32)
		for {
			current := maxCount.Load()
			if active <= current || maxCount.CompareAndSwap(current, active) {
				break
			}
		}

		<-start
		<-finish
		count.Add(-1)
		return coresync.RepoJobResult{}, nil
	}

	s := NewScheduler(testDaemonConfig(), slog.New(slog.NewJSONHandler(io.Discard, nil)), NewRepoLockManager(), run, nil)
	s.cfg.MaxParallelRepos = 4
	s.cfg.MaxParallelPerSource = 1

	tasks := []RepoTask{
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/a"}},
		{Source: config.SourceConfig{ID: "gh"}, Repo: config.RepoConfig{Path: "/repos/b"}},
		{Source: config.SourceConfig{ID: "az"}, Repo: config.RepoConfig{Path: "/repos/c"}},
		{Source: config.SourceConfig{ID: "az"}, Repo: config.RepoConfig{Path: "/repos/d"}},
	}

	done := make(chan struct{})
	go func() {
		s.RunCycle(context.Background(), "trace-source-limit", tasks, false)
		close(done)
	}()

	close(start)
	time.Sleep(20 * time.Millisecond)
	close(finish)
	<-done

	assertMax := func(source string) {
		t.Helper()
		maxAny, ok := maxBySource.Load(source)
		if !ok {
			t.Fatalf("missing source stats for %s", source)
		}
		if got := maxAny.(*atomic.Int32).Load(); got > 1 {
			t.Fatalf("source %s max concurrency = %d, want <= 1", source, got)
		}
	}

	assertMax("gh")
	assertMax("az")
}

func testDaemonConfig() config.DaemonConfig {
	return config.DaemonConfig{
		IntervalSeconds:         5,
		JitterSeconds:           2,
		MaxParallelRepos:        2,
		MaxParallelPerSource:    1,
		OperationTimeoutSeconds: 5,
		Retry: config.RetryConfig{
			MaxAttempts:        3,
			BaseBackoffSeconds: 1,
		},
	}
}
