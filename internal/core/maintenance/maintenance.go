// Package maintenance provides helpers for evaluating daemon maintenance
// windows.  A maintenance window blocks all mutating sync operations for its
// duration.  Unlike governance AllowedSyncWindows (which restrict when sync is
// permitted), a maintenance window explicitly BLOCKS sync.
package maintenance

import (
	"fmt"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

// ReasonCode is emitted in log lines and telemetry events when a sync cycle
// is suppressed by a maintenance window.
const ReasonCode = "maintenance_window_active"

// ActiveWindow returns the first maintenance window that is currently active
// at the given wall-clock time, plus a human-readable description.
// If no window is active it returns nil, "".
func ActiveWindow(windows []config.MaintenanceWindow, now time.Time) (*config.MaintenanceWindow, string) {
	todayWD := now.Weekday()
	nowMins := now.Hour()*60 + now.Minute()

	for i, w := range windows {
		if !matchesDay(w.Days, todayWD) {
			continue
		}
		startMins, err1 := clockMins(w.Start)
		endMins, err2 := clockMins(w.End)
		if err1 != nil || err2 != nil {
			// Malformed window — skip silently (validation catches this at load time).
			continue
		}
		if nowMins >= startMins && nowMins < endMins {
			name := w.Name
			if name == "" {
				name = fmt.Sprintf("window[%d]", i)
			}
			desc := fmt.Sprintf("%s (%s %s-%s)", name, todayWD, w.Start, w.End)
			return &windows[i], desc
		}
	}
	return nil, ""
}

// NextAllowed returns the earliest time after now at which all current
// maintenance windows have ended.  If no window is active it returns now.
func NextAllowed(windows []config.MaintenanceWindow, now time.Time) time.Time {
	w, _ := ActiveWindow(windows, now)
	if w == nil {
		return now
	}
	endMins, err := clockMins(w.End)
	if err != nil {
		return now
	}
	endHour := endMins / 60
	endMin := endMins % 60
	candidate := time.Date(now.Year(), now.Month(), now.Day(), endHour, endMin, 0, 0, now.Location())
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

func matchesDay(days []string, wd time.Weekday) bool {
	for _, d := range days {
		if wdParsed, ok := parseDay(d); ok && wdParsed == wd {
			return true
		}
	}
	return false
}

func parseDay(raw string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

func clockMins(hhmm string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(hhmm))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}
