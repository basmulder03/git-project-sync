package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/notify"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type ServiceAPI struct {
	store      state.Store
	dispatcher *notify.Dispatcher
}

type RepoStatus struct {
	RepoPath   string
	LastStatus string
	LastError  string
	LastSyncAt time.Time
	UpdatedAt  time.Time
}

type RunState struct {
	RunID       string
	TraceID     string
	RepoPath    string
	SourceID    string
	Status      string
	Note        string
	StartedAt   time.Time
	HeartbeatAt time.Time
	CompletedAt time.Time
}

func NewServiceAPI(store state.Store) *ServiceAPI {
	return &ServiceAPI{store: store}
}

// WithDispatcher attaches a notification dispatcher to the API.
// RecordEvent will fan-out to all configured sinks after persisting.
func (s *ServiceAPI) WithDispatcher(d *notify.Dispatcher) *ServiceAPI {
	s.dispatcher = d
	return s
}

func (s *ServiceAPI) RecordEvent(ctx context.Context, event telemetry.Event) error {
	if err := s.store.AppendEvent(state.Event{
		TraceID:    event.TraceID,
		RepoPath:   event.RepoPath,
		Level:      event.Level,
		ReasonCode: event.ReasonCode,
		Message:    event.Message,
		CreatedAt:  event.CreatedAt,
	}); err != nil {
		return err
	}

	if s.dispatcher != nil {
		s.dispatcher.Dispatch(ctx, event)
	}

	return nil
}

func (s *ServiceAPI) ListEvents(limit int) ([]telemetry.Event, error) {
	events, err := s.store.ListEvents(limit)
	if err != nil {
		return nil, err
	}

	out := make([]telemetry.Event, 0, len(events))
	for _, event := range events {
		out = append(out, telemetry.Event{
			TraceID:    event.TraceID,
			RepoPath:   event.RepoPath,
			Level:      event.Level,
			ReasonCode: event.ReasonCode,
			Message:    event.Message,
			CreatedAt:  event.CreatedAt,
		})
	}

	return out, nil
}

func (s *ServiceAPI) Trace(traceID string, limit int) ([]telemetry.Event, error) {
	if traceID == "" {
		return nil, fmt.Errorf("trace id is required")
	}

	events, err := s.store.ListEventsByTrace(traceID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]telemetry.Event, 0, len(events))
	for _, event := range events {
		out = append(out, telemetry.Event{
			TraceID:    event.TraceID,
			RepoPath:   event.RepoPath,
			Level:      event.Level,
			ReasonCode: event.ReasonCode,
			Message:    event.Message,
			CreatedAt:  event.CreatedAt,
		})
	}

	return out, nil
}

func (s *ServiceAPI) RepoStatuses(limit int) ([]RepoStatus, error) {
	repos, err := s.store.ListRepoStates(limit)
	if err != nil {
		return nil, err
	}

	out := make([]RepoStatus, 0, len(repos))
	for _, repo := range repos {
		out = append(out, RepoStatus{
			RepoPath:   repo.RepoPath,
			LastStatus: repo.LastStatus,
			LastError:  repo.LastError,
			LastSyncAt: repo.LastSyncAt,
			UpdatedAt:  repo.UpdatedAt,
		})
	}

	return out, nil
}

func (s *ServiceAPI) InFlightRuns(limit int) ([]RunState, error) {
	runs, err := s.store.ListInFlightRunStates(limit)
	if err != nil {
		return nil, err
	}

	out := make([]RunState, 0, len(runs))
	for _, run := range runs {
		out = append(out, RunState{
			RunID:       run.RunID,
			TraceID:     run.TraceID,
			RepoPath:    run.RepoPath,
			SourceID:    run.SourceID,
			Status:      run.Status,
			Note:        run.Note,
			StartedAt:   run.StartedAt,
			HeartbeatAt: run.HeartbeatAt,
			CompletedAt: run.CompletedAt,
		})
	}

	return out, nil
}
