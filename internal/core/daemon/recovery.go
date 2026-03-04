package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/state"
)

type Recoverer struct {
	store state.Store
	mu    sync.Mutex
	seen  map[string]struct{}
}

func NewRecoverer(store state.Store) *Recoverer {
	return &Recoverer{store: store, seen: map[string]struct{}{}}
}

func (r *Recoverer) BeginRun(runID, traceID, repoPath, sourceID string) (bool, error) {
	if runID == "" || repoPath == "" {
		return false, fmt.Errorf("run id and repo path are required")
	}

	r.mu.Lock()
	if _, exists := r.seen[runID]; exists {
		r.mu.Unlock()
		return false, nil
	}
	r.seen[runID] = struct{}{}
	r.mu.Unlock()

	err := r.store.UpsertRunState(state.RunState{
		RunID:       runID,
		TraceID:     traceID,
		RepoPath:    repoPath,
		SourceID:    sourceID,
		Status:      "running",
		Note:        "in-flight",
		StartedAt:   time.Now().UTC(),
		HeartbeatAt: time.Now().UTC(),
	})
	if err != nil {
		r.mu.Lock()
		delete(r.seen, runID)
		r.mu.Unlock()
		return false, err
	}

	return true, nil
}

func (r *Recoverer) CompleteRun(runID, status, note string) error {
	if err := r.store.CompleteRunState(runID, status, note); err != nil {
		return err
	}

	r.mu.Lock()
	delete(r.seen, runID)
	r.mu.Unlock()
	return nil
}

func (r *Recoverer) RecoverInFlightRuns(limit int) ([]state.RunState, error) {
	runs, err := r.store.ListInFlightRunStates(limit)
	if err != nil {
		return nil, err
	}

	for _, run := range runs {
		_ = r.store.CompleteRunState(run.RunID, "recovered", "daemon restarted before completion")
	}

	return runs, nil
}
