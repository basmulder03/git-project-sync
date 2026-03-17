package telemetry

import (
	"strings"
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

func TestBuildMetrics_Counters(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	events := []Event{
		{Level: "info", ReasonCode: ReasonSyncCompleted, CreatedAt: now},
		{Level: "info", ReasonCode: ReasonSyncCompleted, CreatedAt: now},
		{Level: "error", ReasonCode: ReasonSyncFailed, CreatedAt: now},
		{Level: "warn", ReasonCode: ReasonSyncRetry, CreatedAt: now},
		{Level: "warn", ReasonCode: ReasonRepoLocked, CreatedAt: now},
		{Level: "warn", ReasonCode: "maintenance_window_active", CreatedAt: now},
		{Level: "warn", ReasonCode: "policy_repo_excluded", CreatedAt: now},
		{Level: "info", ReasonCode: "update_applied", CreatedAt: now},
		{Level: "info", ReasonCode: "something_else", CreatedAt: now},
	}

	m := BuildMetrics(events, now)

	if m.SyncCompleted != 2 {
		t.Errorf("SyncCompleted=%d want 2", m.SyncCompleted)
	}
	if m.SyncFailed != 1 {
		t.Errorf("SyncFailed=%d want 1", m.SyncFailed)
	}
	if m.SyncRetried != 1 {
		t.Errorf("SyncRetried=%d want 1", m.SyncRetried)
	}
	if m.RepoLocked != 1 {
		t.Errorf("RepoLocked=%d want 1", m.RepoLocked)
	}
	if m.MaintenanceSkipped != 1 {
		t.Errorf("MaintenanceSkipped=%d want 1", m.MaintenanceSkipped)
	}
	if m.PolicySkipped != 1 {
		t.Errorf("PolicySkipped=%d want 1", m.PolicySkipped)
	}
	if m.UpdateApplied != 1 {
		t.Errorf("UpdateApplied=%d want 1", m.UpdateApplied)
	}
	if m.OtherEvents != 1 {
		t.Errorf("OtherEvents=%d want 1", m.OtherEvents)
	}
	if m.TotalEvents != 9 {
		t.Errorf("TotalEvents=%d want 9", m.TotalEvents)
	}
	if m.ErrorEvents != 1 {
		t.Errorf("ErrorEvents=%d want 1", m.ErrorEvents)
	}
	if m.WarnEvents != 4 {
		t.Errorf("WarnEvents=%d want 4", m.WarnEvents)
	}
}

func TestBuildMetrics_Empty(t *testing.T) {
	t.Parallel()

	m := BuildMetrics(nil, time.Now().UTC())
	if m.TotalEvents != 0 {
		t.Errorf("expected 0 total events for empty slice, got %d", m.TotalEvents)
	}
	if m.HealthScore != 100 {
		t.Errorf("expected health score 100 for zero events, got %d", m.HealthScore)
	}
	if m.Timestamp == "" {
		t.Error("Timestamp must be set")
	}
}

func TestFormatPrometheus_ContainsRequiredLines(t *testing.T) {
	t.Parallel()

	m := BuildMetrics([]Event{
		{Level: "error", ReasonCode: ReasonSyncFailed, CreatedAt: time.Now()},
	}, time.Now().UTC())

	out := FormatPrometheus(m)

	required := []string{
		"gps_events_total",
		"gps_sync_failed_total",
		"gps_health_score",
		"# HELP",
		"# TYPE",
	}
	for _, want := range required {
		if !strings.Contains(out, want) {
			t.Errorf("Prometheus output missing %q", want)
		}
	}
}

func TestFormatOpenMetrics_HasEOF(t *testing.T) {
	t.Parallel()

	m := BuildMetrics(nil, time.Now().UTC())
	out := FormatOpenMetrics(m)
	if !strings.HasSuffix(out, "# EOF\n") {
		t.Errorf("OpenMetrics output must end with '# EOF\\n', got: %q", out[max(0, len(out)-20):])
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
