package commands

import (
	"context"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type testRecorder struct {
	events []telemetry.Event
}

func (r *testRecorder) RecordEvent(_ context.Context, event telemetry.Event) error {
	r.events = append(r.events, event)
	return nil
}

func TestCacheServiceRefreshAllRecordsTwoEvents(t *testing.T) {
	recorder := &testRecorder{}
	svc := NewCacheService(recorder)
	svc.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

	if err := svc.Refresh(context.Background(), CacheTargetAll); err != nil {
		t.Fatalf("refresh all failed: %v", err)
	}

	if len(recorder.events) != 2 {
		t.Fatalf("expected two events, got %d", len(recorder.events))
	}
	if recorder.events[0].ReasonCode != "cache_refresh_providers" {
		t.Fatalf("unexpected first reason code: %s", recorder.events[0].ReasonCode)
	}
	if recorder.events[1].ReasonCode != "cache_refresh_branches" {
		t.Fatalf("unexpected second reason code: %s", recorder.events[1].ReasonCode)
	}
}

func TestLatestCacheEvents(t *testing.T) {
	now := time.Now().UTC()
	events := []telemetry.Event{
		{ReasonCode: "cache_refresh_providers", CreatedAt: now.Add(-2 * time.Minute)},
		{ReasonCode: "cache_clear_providers", CreatedAt: now.Add(-1 * time.Minute)},
		{ReasonCode: "cache_refresh_providers", CreatedAt: now},
	}

	refreshed, cleared := LatestCacheEvents(events, CacheTargetProviders)
	if !refreshed.Equal(now) {
		t.Fatalf("unexpected refreshed timestamp: %s", refreshed)
	}
	if !cleared.Equal(now.Add(-1 * time.Minute)) {
		t.Fatalf("unexpected cleared timestamp: %s", cleared)
	}
}
