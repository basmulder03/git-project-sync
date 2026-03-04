package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

type RepoTask struct {
	Source config.SourceConfig
	Repo   config.RepoConfig
}

type RunRepoFunc func(ctx context.Context, traceID string, source config.SourceConfig, repo config.RepoConfig, dryRun bool) (coresync.RepoJobResult, error)

type Scheduler struct {
	cfg      config.DaemonConfig
	logger   *slog.Logger
	locks    *RepoLockManager
	runRepo  RunRepoFunc
	rand     *rand.Rand
	now      func() time.Time
	sleepCtx func(context.Context, time.Duration) error
}

func NewScheduler(cfg config.DaemonConfig, logger *slog.Logger, locks *RepoLockManager, runRepo RunRepoFunc) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		logger:  logger,
		locks:   locks,
		runRepo: runRepo,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
		now:     time.Now,
		sleepCtx: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (s *Scheduler) RunCycle(ctx context.Context, traceID string, tasks []RepoTask, dryRun bool) {
	maxParallel := s.cfg.MaxParallelRepos
	if maxParallel < 1 {
		maxParallel = 1
	}

	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for _, task := range tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			s.runTaskWithRetry(ctx, traceID, task, dryRun)
		}()
	}

	wg.Wait()
}

func (s *Scheduler) RunPeriodic(ctx context.Context, tasks []RepoTask, dryRun bool) error {
	for {
		traceID := fmt.Sprintf("run-%d", s.now().UTC().UnixNano())
		s.RunCycle(ctx, traceID, tasks, dryRun)

		waitFor := s.nextInterval()
		s.logger.Info("scheduler sleeping", "duration_ms", waitFor.Milliseconds())
		if err := s.sleepCtx(ctx, waitFor); err != nil {
			return err
		}
	}
}

func (s *Scheduler) runTaskWithRetry(ctx context.Context, traceID string, task RepoTask, dryRun bool) {
	maxAttempts := s.cfg.Retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		acquired, err := s.locks.TryWithLock(task.Repo.Path, func() error {
			opCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.OperationTimeoutSeconds)*time.Second)
			defer cancel()

			_, runErr := s.runRepo(opCtx, traceID, task.Source, task.Repo, dryRun)
			return runErr
		})

		if !acquired {
			s.logger.Info("repo sync skipped", "trace_id", traceID, "repo_path", task.Repo.Path, "reason_code", "repo_locked", "reason", "another sync run already holds lock")
			return
		}

		if err == nil {
			return
		}

		if attempt == maxAttempts {
			s.logger.Error("repo sync failed after retries", "trace_id", traceID, "repo_path", task.Repo.Path, "attempts", attempt, "error", err)
			return
		}

		backoff := s.backoff(attempt)
		s.logger.Warn("repo sync attempt failed", "trace_id", traceID, "repo_path", task.Repo.Path, "attempt", attempt, "next_backoff_ms", backoff.Milliseconds(), "error", err)
		if sleepErr := s.sleepCtx(ctx, backoff); sleepErr != nil {
			return
		}
	}
}

func (s *Scheduler) nextInterval() time.Duration {
	base := time.Duration(s.cfg.IntervalSeconds) * time.Second
	if base <= 0 {
		base = 5 * time.Minute
	}

	jitter := time.Duration(s.cfg.JitterSeconds) * time.Second
	if jitter <= 0 {
		return base
	}

	delta := time.Duration(s.rand.Int63n(int64(jitter) + 1))
	return base + delta
}

func (s *Scheduler) backoff(attempt int) time.Duration {
	base := time.Duration(s.cfg.Retry.BaseBackoffSeconds) * time.Second
	if base <= 0 {
		base = 2 * time.Second
	}

	multiplier := 1 << (attempt - 1)
	return time.Duration(multiplier) * base
}
