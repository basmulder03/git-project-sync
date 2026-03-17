package telemetry

import (
	"fmt"
	"strings"
	"time"
)

type EventSummary struct {
	ErrorsLastHour int
	WarnsLastHour  int
	TotalLastHour  int
}

func SummarizeRecentEvents(events []Event, now time.Time) EventSummary {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Reuse BuildMetrics so both summary and full metrics are computed in a
	// single pass over the events slice.
	m := BuildMetrics(events, now)
	return EventSummary{
		ErrorsLastHour: int(m.RecentErrors),
		WarnsLastHour:  int(m.RecentWarns),
		TotalLastHour:  int(m.RecentTotal),
	}
}

func HealthScore(criticalCount, warningCount int) int {
	score := 100 - (criticalCount * 30) - (warningCount * 10)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

// MetricsSnapshot holds counters suitable for ingestion by external monitoring
// tools (Prometheus, Datadog, etc.).
type MetricsSnapshot struct {
	// Event counters by reason code
	SyncCompleted      int64
	SyncFailed         int64
	SyncRetried        int64
	RepoLocked         int64
	MaintenanceSkipped int64
	PolicySkipped      int64
	UpdateApplied      int64
	OtherEvents        int64

	// Aggregate level counters (all time, from supplied events slice)
	TotalEvents int64
	ErrorEvents int64
	WarnEvents  int64

	// Last-hour counters (computed in the same pass as the aggregate counters).
	RecentTotal  int64
	RecentErrors int64
	RecentWarns  int64

	// Health score [0..100]
	HealthScore int

	// Scrape timestamp (RFC3339 UTC)
	Timestamp string
}

// BuildMetrics computes a MetricsSnapshot from a slice of events.
// It also fills in the EventSummary fields so callers that previously called
// both BuildMetrics and SummarizeRecentEvents can use a single pass.
func BuildMetrics(events []Event, now time.Time) MetricsSnapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	cutoff := now.Add(-1 * time.Hour)

	var m MetricsSnapshot
	m.Timestamp = now.UTC().Format(time.RFC3339)

	for _, e := range events {
		m.TotalEvents++
		switch e.Level {
		case "error":
			m.ErrorEvents++
		case "warn", "warning":
			m.WarnEvents++
		}

		switch e.ReasonCode {
		case ReasonSyncCompleted:
			m.SyncCompleted++
		case ReasonSyncFailed:
			m.SyncFailed++
		case ReasonSyncRetry:
			m.SyncRetried++
		case ReasonRepoLocked:
			m.RepoLocked++
		case "maintenance_window_active":
			m.MaintenanceSkipped++
		case "policy_repo_excluded", "policy_repo_not_included",
			"policy_repo_protected", "policy_outside_sync_window":
			m.PolicySkipped++
		case "update_applied":
			m.UpdateApplied++
		default:
			m.OtherEvents++
		}

		// Accumulate last-hour summary in the same pass.
		if !e.CreatedAt.Before(cutoff) {
			m.RecentTotal++
			switch e.Level {
			case "error":
				m.RecentErrors++
			case "warn", "warning":
				m.RecentWarns++
			}
		}
	}

	m.HealthScore = HealthScore(int(m.ErrorEvents), int(m.WarnEvents))
	return m
}

// FormatPrometheus renders the snapshot as Prometheus text exposition format.
// Labels and help lines follow the OpenMetrics/Prometheus conventions.
func FormatPrometheus(m MetricsSnapshot) string {
	var b strings.Builder
	metric := func(name, help, typ string, value int64) {
		fmt.Fprintf(&b, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&b, "# TYPE %s %s\n", name, typ)
		fmt.Fprintf(&b, "%s %d\n", name, value)
	}
	gauge := func(name, help string, value int64) { metric(name, help, "gauge", value) }
	counter := func(name, help string, value int64) { metric(name, help, "counter", value) }

	counter("gps_events_total", "Total telemetry events recorded", m.TotalEvents)
	counter("gps_events_error_total", "Total error-level events", m.ErrorEvents)
	counter("gps_events_warn_total", "Total warn-level events", m.WarnEvents)
	counter("gps_sync_completed_total", "Successful sync operations", m.SyncCompleted)
	counter("gps_sync_failed_total", "Failed sync operations", m.SyncFailed)
	counter("gps_sync_retried_total", "Sync retry attempts", m.SyncRetried)
	counter("gps_repo_locked_total", "Skips due to repo already locked", m.RepoLocked)
	counter("gps_maintenance_skipped_total", "Skips due to active maintenance window", m.MaintenanceSkipped)
	counter("gps_policy_skipped_total", "Skips due to governance policy", m.PolicySkipped)
	counter("gps_update_applied_total", "Self-update applications", m.UpdateApplied)
	gauge("gps_health_score", "Composite health score [0..100]", int64(m.HealthScore))
	fmt.Fprintf(&b, "# scrape_timestamp %s\n", m.Timestamp)
	return b.String()
}

// FormatOpenMetrics renders the snapshot in OpenMetrics format (compatible with
// the OpenMetrics spec, which requires a trailing "# EOF" line).
func FormatOpenMetrics(m MetricsSnapshot) string {
	body := FormatPrometheus(m)
	return body + "# EOF\n"
}
