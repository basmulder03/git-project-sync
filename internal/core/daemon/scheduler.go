package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/maintenance"
	"github.com/basmulder03/git-project-sync/internal/core/providers"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
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
	recorder *ServiceAPI
	rand     *rand.Rand
	now      func() time.Time
	sleepCtx func(context.Context, time.Duration) error

	rateLimitMu   sync.Mutex
	sourceBackoff map[string]time.Time
}

func NewScheduler(cfg config.DaemonConfig, logger *slog.Logger, locks *RepoLockManager, runRepo RunRepoFunc, recorder *ServiceAPI) *Scheduler {
	return &Scheduler{
		cfg:           cfg,
		logger:        logger,
		locks:         locks,
		runRepo:       runRepo,
		recorder:      recorder,
		rand:          rand.New(rand.NewSource(time.Now().UnixNano())),
		now:           time.Now,
		sourceBackoff: map[string]time.Time{},
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

// SetNow replaces the internal clock function used by the scheduler.
// This is intended for testing only — it allows callers to pin the clock to a
// specific instant so that maintenance-window and backoff logic can be
// exercised deterministically.
func (s *Scheduler) SetNow(fn func() time.Time) {
	s.now = fn
}

func (s *Scheduler) RunCycle(ctx context.Context, traceID string, tasks []RepoTask, dryRun bool) {
	maxParallelRepos := s.cfg.MaxParallelRepos
	if maxParallelRepos < 1 {
		maxParallelRepos = 1
	}
	maxParallelPerSource := s.cfg.MaxParallelPerSource
	if maxParallelPerSource < 1 {
		maxParallelPerSource = 1
	}

	orderedTasks := fairOrderTasks(tasks)

	globalSem := make(chan struct{}, maxParallelRepos)
	sourceSems := map[string]chan struct{}{}
	var sourceSemsMu sync.Mutex
	var wg sync.WaitGroup

	for _, task := range orderedTasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case globalSem <- struct{}{}:
			}
			defer func() { <-globalSem }()

			sourceKey := sourceBucketKey(task.Source.ID)
			sourceSemsMu.Lock()
			sourceSem, ok := sourceSems[sourceKey]
			if !ok {
				sourceSem = make(chan struct{}, maxParallelPerSource)
				sourceSems[sourceKey] = sourceSem
			}
			sourceSemsMu.Unlock()

			select {
			case <-ctx.Done():
				return
			case sourceSem <- struct{}{}:
			}
			defer func() { <-sourceSem }()

			s.runTaskWithRetry(ctx, traceID, task, dryRun)
		}()
	}

	wg.Wait()
}

func fairOrderTasks(tasks []RepoTask) []RepoTask {
	if len(tasks) <= 1 {
		return append([]RepoTask(nil), tasks...)
	}

	bySource := map[string][]RepoTask{}
	sourceOrder := make([]string, 0)
	for _, task := range tasks {
		key := sourceBucketKey(task.Source.ID)
		if _, exists := bySource[key]; !exists {
			sourceOrder = append(sourceOrder, key)
		}
		bySource[key] = append(bySource[key], task)
	}

	ordered := make([]RepoTask, 0, len(tasks))
	remaining := len(tasks)
	for remaining > 0 {
		for _, sourceID := range sourceOrder {
			queue := bySource[sourceID]
			if len(queue) == 0 {
				continue
			}
			ordered = append(ordered, queue[0])
			bySource[sourceID] = queue[1:]
			remaining--
		}
	}

	return ordered
}

func sourceBucketKey(sourceID string) string {
	trimmed := strings.TrimSpace(sourceID)
	if trimmed == "" {
		return "_unknown_source"
	}
	return trimmed
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
	// Maintenance window check — block all mutating operations while active.
	if mw, desc := maintenance.ActiveWindow(s.cfg.MaintenanceWindows, s.now()); mw != nil {
		s.logger.Info("repo sync skipped: maintenance window active",
			"trace_id", traceID,
			"repo_path", task.Repo.Path,
			"reason_code", maintenance.ReasonCode,
			"window", desc,
			"next_allowed", maintenance.NextAllowed(s.cfg.MaintenanceWindows, s.now()),
		)
		s.recordEvent(ctx, telemetry.Event{
			TraceID:    traceID,
			RepoPath:   task.Repo.Path,
			Level:      "warn",
			ReasonCode: maintenance.ReasonCode,
			Message:    "sync suppressed by maintenance window: " + desc,
			CreatedAt:  s.now().UTC(),
		})
		return
	}

	maxAttempts := s.cfg.Retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	retryBudget := time.Duration(s.cfg.OperationTimeoutSeconds) * time.Second
	if retryBudget <= 0 {
		retryBudget = 30 * time.Second
	}
	usedBudget := time.Duration(0)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if waitFor := s.sourceWait(task.Source.ID); waitFor > 0 {
			s.logger.Info("source backoff delay", "trace_id", traceID, "source_id", task.Source.ID, "wait_ms", waitFor.Milliseconds(), "reason_code", "provider_rate_limited")
			s.recordEvent(ctx, telemetry.Event{TraceID: traceID, RepoPath: task.Repo.Path, Level: "warn", ReasonCode: "provider_rate_limited", Message: "waiting due to previous provider throttling", CreatedAt: s.now().UTC()})
			if err := s.sleepCtx(ctx, waitFor); err != nil {
				return
			}
		}

		var lastResult coresync.RepoJobResult
		acquired, err := s.locks.TryWithLock(task.Repo.Path, func() error {
			opCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.OperationTimeoutSeconds)*time.Second)
			defer cancel()

			result, runErr := s.runRepo(opCtx, traceID, task.Source, task.Repo, dryRun)
			lastResult = result
			return runErr
		})

		if !acquired {
			s.logger.Info("repo sync skipped", "trace_id", traceID, "repo_path", task.Repo.Path, "reason_code", "repo_locked", "reason", "another sync run already holds lock")
			s.recordEvent(ctx, telemetry.Event{TraceID: traceID, RepoPath: task.Repo.Path, Level: "warn", ReasonCode: telemetry.ReasonRepoLocked, Message: "another sync run already holds lock", CreatedAt: s.now().UTC()})
			return
		}

		if err == nil {
			s.recordEvent(ctx, eventFromResult(traceID, task.Repo.Path, lastResult))
			return
		}

		class, reason := providers.ClassifyError(err)
		if class == providers.ErrorClassPermanent {
			s.logger.Error("repo sync failed without retry", "trace_id", traceID, "repo_path", task.Repo.Path, "reason_code", reason, "error", err)
			s.recordEvent(ctx, telemetry.Event{TraceID: traceID, RepoPath: task.Repo.Path, Level: "error", ReasonCode: reason, Message: err.Error(), CreatedAt: s.now().UTC()})
			return
		}

		if attempt == maxAttempts {
			s.logger.Error("repo sync failed after retries", "trace_id", traceID, "repo_path", task.Repo.Path, "attempts", attempt, "error", err)
			s.recordEvent(ctx, telemetry.Event{TraceID: traceID, RepoPath: task.Repo.Path, Level: "error", ReasonCode: telemetry.ReasonSyncFailed, Message: err.Error(), CreatedAt: s.now().UTC()})
			return
		}

		if rateLimitErr, ok := providers.AsRateLimitError(err); ok {
			delay := rateLimitErr.RetryAfter
			if delay <= 0 {
				delay = 30 * time.Second
			}
			s.applySourceBackoff(task.Source.ID, delay)
		}

		backoff := s.backoff(attempt)
		usedBudget += backoff
		if usedBudget > retryBudget {
			s.logger.Error("repo sync retry budget exceeded", "trace_id", traceID, "repo_path", task.Repo.Path, "retry_budget_ms", retryBudget.Milliseconds(), "used_ms", usedBudget.Milliseconds(), "error", err)
			s.recordEvent(ctx, telemetry.Event{TraceID: traceID, RepoPath: task.Repo.Path, Level: "error", ReasonCode: "retry_budget_exceeded", Message: err.Error(), CreatedAt: s.now().UTC()})
			return
		}
		s.logger.Warn("repo sync attempt failed", "trace_id", traceID, "repo_path", task.Repo.Path, "attempt", attempt, "next_backoff_ms", backoff.Milliseconds(), "error", err)
		s.recordEvent(ctx, telemetry.Event{TraceID: traceID, RepoPath: task.Repo.Path, Level: "warn", ReasonCode: telemetry.ReasonSyncRetry, Message: err.Error(), CreatedAt: s.now().UTC()})
		if sleepErr := s.sleepCtx(ctx, backoff); sleepErr != nil {
			return
		}
	}
}

func (s *Scheduler) applySourceBackoff(sourceID string, delay time.Duration) {
	if sourceID == "" || delay <= 0 {
		return
	}

	next := s.now().UTC().Add(delay)
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	current, ok := s.sourceBackoff[sourceID]
	if !ok || next.After(current) {
		s.sourceBackoff[sourceID] = next
	}
}

func (s *Scheduler) sourceWait(sourceID string) time.Duration {
	if sourceID == "" {
		return 0
	}

	s.rateLimitMu.Lock()
	until, ok := s.sourceBackoff[sourceID]
	s.rateLimitMu.Unlock()
	if !ok {
		return 0
	}

	remaining := until.Sub(s.now().UTC())
	if remaining <= 0 {
		s.rateLimitMu.Lock()
		delete(s.sourceBackoff, sourceID)
		s.rateLimitMu.Unlock()
		return 0
	}

	return remaining
}

func (s *Scheduler) recordEvent(ctx context.Context, event telemetry.Event) {
	if s.recorder == nil {
		return
	}
	_ = s.recorder.RecordEvent(ctx, event)
}

func eventFromResult(traceID, repoPath string, result coresync.RepoJobResult) telemetry.Event {
	if result.Skipped {
		return telemetry.Event{TraceID: traceID, RepoPath: repoPath, Level: "warn", ReasonCode: result.ReasonCode, Message: result.Reason, CreatedAt: time.Now().UTC()}
	}
	return telemetry.Event{TraceID: traceID, RepoPath: repoPath, Level: "info", ReasonCode: telemetry.ReasonSyncCompleted, Message: "sync completed", CreatedAt: time.Now().UTC()}
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
