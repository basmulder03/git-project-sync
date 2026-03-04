package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type ServiceAPI struct {
	store state.Store
}

type RepoStatus struct {
	RepoPath   string
	LastStatus string
	LastError  string
	LastSyncAt time.Time
	UpdatedAt  time.Time
}

func NewServiceAPI(store state.Store) *ServiceAPI {
	return &ServiceAPI{store: store}
}

func (s *ServiceAPI) RecordEvent(_ context.Context, event telemetry.Event) error {
	return s.store.AppendEvent(state.Event{
		TraceID:    event.TraceID,
		RepoPath:   event.RepoPath,
		Level:      event.Level,
		ReasonCode: event.ReasonCode,
		Message:    event.Message,
		CreatedAt:  event.CreatedAt,
	})
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
