package maintenance

import (
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func makeWindow(name, start, end string, days ...string) config.MaintenanceWindow {
	return config.MaintenanceWindow{Name: name, Start: start, End: end, Days: days}
}

// tuesday 14:30 UTC
var refTime = time.Date(2026, 1, 6, 14, 30, 0, 0, time.UTC) // Tuesday

func TestActiveWindow_NoWindows(t *testing.T) {
	t.Parallel()
	w, desc := ActiveWindow(nil, refTime)
	if w != nil || desc != "" {
		t.Errorf("expected no active window, got %v %q", w, desc)
	}
}

func TestActiveWindow_NotActive_WrongDay(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{makeWindow("mon-only", "14:00", "16:00", "monday")}
	w, _ := ActiveWindow(windows, refTime) // Tuesday
	if w != nil {
		t.Error("window should not be active on wrong day")
	}
}

func TestActiveWindow_NotActive_BeforeStart(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{makeWindow("late", "15:00", "17:00", "tuesday")}
	w, _ := ActiveWindow(windows, refTime) // 14:30, before 15:00
	if w != nil {
		t.Error("window should not be active before start time")
	}
}

func TestActiveWindow_NotActive_AfterEnd(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{makeWindow("early", "12:00", "14:00", "tuesday")}
	w, _ := ActiveWindow(windows, refTime) // 14:30, after 14:00
	if w != nil {
		t.Error("window should not be active after end time")
	}
}

func TestActiveWindow_Active(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{makeWindow("deploy", "14:00", "16:00", "tuesday")}
	w, desc := ActiveWindow(windows, refTime) // 14:30 inside [14:00,16:00)
	if w == nil {
		t.Fatal("expected active window")
	}
	if desc == "" {
		t.Error("expected non-empty description")
	}
	if w.Name != "deploy" {
		t.Errorf("window name = %q, want deploy", w.Name)
	}
}

func TestActiveWindow_ActiveAtStartBoundary(t *testing.T) {
	t.Parallel()
	// exactly at start minute — should be active
	at := time.Date(2026, 1, 6, 14, 0, 0, 0, time.UTC) // Tuesday 14:00
	windows := []config.MaintenanceWindow{makeWindow("exact", "14:00", "16:00", "tuesday")}
	w, _ := ActiveWindow(windows, at)
	if w == nil {
		t.Error("window should be active at start boundary (inclusive)")
	}
}

func TestActiveWindow_NotActiveAtEndBoundary(t *testing.T) {
	t.Parallel()
	// exactly at end minute — should NOT be active (exclusive end)
	at := time.Date(2026, 1, 6, 16, 0, 0, 0, time.UTC) // Tuesday 16:00
	windows := []config.MaintenanceWindow{makeWindow("exact", "14:00", "16:00", "tuesday")}
	w, _ := ActiveWindow(windows, at)
	if w != nil {
		t.Error("window should not be active at end boundary (exclusive)")
	}
}

func TestActiveWindow_PicksFirstMatch(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{
		makeWindow("first", "14:00", "16:00", "tuesday"),
		makeWindow("second", "14:00", "16:00", "tuesday"),
	}
	w, _ := ActiveWindow(windows, refTime)
	if w == nil || w.Name != "first" {
		t.Errorf("expected first window, got %v", w)
	}
}

func TestActiveWindow_FallbackName(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{{Start: "14:00", End: "16:00", Days: []string{"tuesday"}}}
	_, desc := ActiveWindow(windows, refTime)
	if desc == "" {
		t.Error("expected non-empty description even without name")
	}
}

func TestNextAllowed_NoActiveWindow(t *testing.T) {
	t.Parallel()
	next := NextAllowed(nil, refTime)
	if !next.Equal(refTime) {
		t.Errorf("NextAllowed with no windows = %v, want %v", next, refTime)
	}
}

func TestNextAllowed_ActiveWindow(t *testing.T) {
	t.Parallel()
	windows := []config.MaintenanceWindow{makeWindow("deploy", "14:00", "16:00", "tuesday")}
	next := NextAllowed(windows, refTime) // currently inside window, ends 16:00
	want := time.Date(2026, 1, 6, 16, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextAllowed = %v, want %v", next, want)
	}
}

func TestReasonCode(t *testing.T) {
	t.Parallel()
	if ReasonCode == "" {
		t.Error("ReasonCode must not be empty")
	}
}
