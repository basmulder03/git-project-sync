package daemon

import (
	"context"
	"fmt"

	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type ServiceAPI struct {
	store state.Store
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
