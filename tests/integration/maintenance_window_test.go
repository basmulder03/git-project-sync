package integration

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	"github.com/basmulder03/git-project-sync/internal/core/maintenance"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

// makeTestScheduler constructs a minimal Scheduler with a controlled clock and
// a stub runRepo function that records invocations.
func makeTestScheduler(
	windows []config.MaintenanceWindow,
	nowFn func() time.Time,
	invoked *int,
) *daemon.Scheduler {
	daemonCfg := config.DaemonConfig{
		IntervalSeconds:         10,
		OperationTimeoutSeconds: 5,
		MaxParallelRepos:        1,
		MaxParallelPerSource:    1,
		MaintenanceWindows:      windows,
		Retry: config.RetryConfig{
			MaxAttempts:        1,
			BaseBackoffSeconds: 1,
		},
	}

	runRepo := func(_ context.Context, _ string, _ config.SourceConfig, _ config.RepoConfig, _ bool) (coresync.RepoJobResult, error) {
		*invoked++
		return coresync.RepoJobResult{}, nil
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	s := daemon.NewScheduler(daemonCfg, logger, daemon.NewRepoLockManager(), runRepo, nil)
	s.SetNow(nowFn)
	return s
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestMaintenanceWindow_BlocksSyncDuringBlackout verifies that when the
// current time falls inside a configured maintenance window, RunCycle does
// NOT invoke the runRepo function for any task.
func TestMaintenanceWindow_BlocksSyncDuringBlackout(t *testing.T) {
	t.Parallel()

	// Pin clock to Tuesday 02:30 — inside the window defined below.
	fixedTime := time.Date(2026, 1, 6, 2, 30, 0, 0, time.UTC) // Tuesday

	windows := []config.MaintenanceWindow{
		{Name: "nightly", Days: []string{"tue"}, Start: "02:00", End: "03:00"},
	}

	var invoked int
	s := makeTestScheduler(windows, func() time.Time { return fixedTime }, &invoked)

	tasks := []daemon.RepoTask{
		{
			Source: config.SourceConfig{ID: "src1", Provider: "github"},
			Repo:   config.RepoConfig{Path: "/workspace/repo1"},
		},
		{
			Source: config.SourceConfig{ID: "src1", Provider: "github"},
			Repo:   config.RepoConfig{Path: "/workspace/repo2"},
		},
	}

	s.RunCycle(context.Background(), "trace-mw-block", tasks, false)

	if invoked != 0 {
		t.Errorf("runRepo invoked %d times during maintenance window, want 0", invoked)
	}
}

// TestMaintenanceWindow_AllowsSyncOutsideBlackout verifies that when the
// current time is outside all configured maintenance windows, RunCycle
// proceeds normally and invokes runRepo for each task.
func TestMaintenanceWindow_AllowsSyncOutsideBlackout(t *testing.T) {
	t.Parallel()

	// Pin clock to Tuesday 04:00 — outside the window defined below.
	fixedTime := time.Date(2026, 1, 6, 4, 0, 0, 0, time.UTC) // Tuesday

	windows := []config.MaintenanceWindow{
		{Name: "nightly", Days: []string{"tue"}, Start: "02:00", End: "03:00"},
	}

	var invoked int
	s := makeTestScheduler(windows, func() time.Time { return fixedTime }, &invoked)

	tasks := []daemon.RepoTask{
		{
			Source: config.SourceConfig{ID: "src1", Provider: "github"},
			Repo:   config.RepoConfig{Path: "/workspace/repo1"},
		},
	}

	s.RunCycle(context.Background(), "trace-mw-allow", tasks, false)

	if invoked != 1 {
		t.Errorf("runRepo invoked %d times outside maintenance window, want 1", invoked)
	}
}

// TestMaintenanceWindow_WrongDayDoesNotBlock verifies that a maintenance
// window defined for a specific weekday does not block sync on other days.
func TestMaintenanceWindow_WrongDayDoesNotBlock(t *testing.T) {
	t.Parallel()

	// Window is for Monday; pin clock to Tuesday.
	fixedTime := time.Date(2026, 1, 6, 2, 30, 0, 0, time.UTC) // Tuesday

	windows := []config.MaintenanceWindow{
		{Name: "monday-maint", Days: []string{"mon"}, Start: "02:00", End: "04:00"},
	}

	var invoked int
	s := makeTestScheduler(windows, func() time.Time { return fixedTime }, &invoked)

	tasks := []daemon.RepoTask{
		{
			Source: config.SourceConfig{ID: "src1", Provider: "github"},
			Repo:   config.RepoConfig{Path: "/workspace/repo1"},
		},
	}

	s.RunCycle(context.Background(), "trace-mw-wrong-day", tasks, false)

	if invoked != 1 {
		t.Errorf("runRepo invoked %d times (window on wrong day), want 1", invoked)
	}
}

// TestMaintenanceWindow_ActiveWindowReasonCode verifies that ActiveWindow
// returns the correct reason code string so that callers can emit it in
// telemetry events.
func TestMaintenanceWindow_ActiveWindowReasonCode(t *testing.T) {
	t.Parallel()

	if maintenance.ReasonCode != "maintenance_window_active" {
		t.Errorf("unexpected reason code %q, want %q", maintenance.ReasonCode, "maintenance_window_active")
	}
}

// TestMaintenanceWindow_NextAllowed_DuringWindow verifies that NextAllowed
// returns a time strictly after the current window's end when called from
// inside an active window.
func TestMaintenanceWindow_NextAllowed_DuringWindow(t *testing.T) {
	t.Parallel()

	// Tuesday 02:30, inside 02:00-03:00.
	now := time.Date(2026, 1, 6, 2, 30, 0, 0, time.UTC)

	windows := []config.MaintenanceWindow{
		{Name: "nightly", Days: []string{"tue"}, Start: "02:00", End: "03:00"},
	}

	next := maintenance.NextAllowed(windows, now)
	if !next.After(now) {
		t.Errorf("NextAllowed %v is not after current time %v", next, now)
	}

	// Expected: 03:00 on the same day.
	want := time.Date(2026, 1, 6, 3, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextAllowed = %v, want %v", next, want)
	}
}

// TestMaintenanceWindow_NextAllowed_OutsideWindow verifies that NextAllowed
// returns the current time when no window is active.
func TestMaintenanceWindow_NextAllowed_OutsideWindow(t *testing.T) {
	t.Parallel()

	// Tuesday 04:00, outside window.
	now := time.Date(2026, 1, 6, 4, 0, 0, 0, time.UTC)

	windows := []config.MaintenanceWindow{
		{Name: "nightly", Days: []string{"tue"}, Start: "02:00", End: "03:00"},
	}

	next := maintenance.NextAllowed(windows, now)
	if !next.Equal(now) {
		t.Errorf("NextAllowed = %v, want %v (current time, no window active)", next, now)
	}
}

// TestMaintenanceWindow_ActiveWindow_Description verifies that the description
// string returned by ActiveWindow is non-empty and contains the window name.
func TestMaintenanceWindow_ActiveWindow_Description(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 6, 2, 30, 0, 0, time.UTC) // Tuesday 02:30

	windows := []config.MaintenanceWindow{
		{Name: "corp-nightly", Days: []string{"tue"}, Start: "02:00", End: "03:00"},
	}

	w, desc := maintenance.ActiveWindow(windows, now)
	if w == nil {
		t.Fatal("expected active window, got nil")
	}
	if !strings.Contains(desc, "corp-nightly") {
		t.Errorf("description %q does not contain window name %q", desc, "corp-nightly")
	}
}

// TestMaintenanceWindow_MultipleWindows_FirstMatchWins verifies that when
// multiple windows overlap in time, the first matching window is returned.
func TestMaintenanceWindow_MultipleWindows_FirstMatchWins(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 6, 2, 30, 0, 0, time.UTC) // Tuesday 02:30

	windows := []config.MaintenanceWindow{
		{Name: "first", Days: []string{"tue"}, Start: "02:00", End: "04:00"},
		{Name: "second", Days: []string{"tue"}, Start: "02:00", End: "05:00"},
	}

	w, _ := maintenance.ActiveWindow(windows, now)
	if w == nil {
		t.Fatal("expected active window, got nil")
	}
	if w.Name != "first" {
		t.Errorf("expected first window to match, got %q", w.Name)
	}
}
