package telemetry

import (
	"testing"
	"time"
)

func TestSummarizeRecentEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	events := []Event{
		{Level: "error", CreatedAt: now.Add(-10 * time.Minute)},
		{Level: "warn", CreatedAt: now.Add(-20 * time.Minute)},
		{Level: "info", CreatedAt: now.Add(-50 * time.Minute)},
		{Level: "error", CreatedAt: now.Add(-2 * time.Hour)},
	}

	s := SummarizeRecentEvents(events, now)
	if s.TotalLastHour != 3 || s.ErrorsLastHour != 1 || s.WarnsLastHour != 1 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestHealthScoreBounds(t *testing.T) {
	t.Parallel()

	if got := HealthScore(0, 0); got != 100 {
		t.Fatalf("score=%d want 100", got)
	}
	if got := HealthScore(1, 2); got != 50 {
		t.Fatalf("score=%d want 50", got)
	}
	if got := HealthScore(10, 10); got != 0 {
		t.Fatalf("score=%d want 0", got)
	}
}
