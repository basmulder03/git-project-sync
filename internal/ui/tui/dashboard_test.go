package tui

import (
	"strings"
	"testing"
	"time"
)

func TestDashboardNavigation(t *testing.T) {
	t.Parallel()

	d := NewDashboard()
	if d.SelectedTab() != "status" {
		t.Fatalf("default tab = %q, want status", d.SelectedTab())
	}

	if changed, _ := d.HandleKey("right", DashboardStatus{}); !changed {
		t.Fatal("expected right key to move tab")
	}
	if d.SelectedTab() != "repos" {
		t.Fatalf("selected tab = %q, want repos", d.SelectedTab())
	}

	if changed, _ := d.HandleKey("left", DashboardStatus{}); !changed {
		t.Fatal("expected left key to move tab")
	}
	if d.SelectedTab() != "status" {
		t.Fatalf("selected tab = %q, want status", d.SelectedTab())
	}
}

func TestDashboardFilterAndReasonDrillDown(t *testing.T) {
	t.Parallel()

	d := NewDashboard()
	_, _ = d.HandleKey("right", DashboardStatus{})

	changed, msg := d.HandleKey("f", DashboardStatus{})
	if !changed || !strings.Contains(msg, "repos filter") {
		t.Fatalf("expected repos filter toggle message, got changed=%t msg=%q", changed, msg)
	}

	_, _ = d.HandleKey("right", DashboardStatus{})
	_, _ = d.HandleKey("right", DashboardStatus{})
	status := DashboardStatus{Events: []EventRow{{ReasonCode: "repo_locked", Level: "warn"}, {ReasonCode: "repo_locked", Level: "warn"}}}
	changed, msg = d.HandleKey("d", status)
	if !changed || !strings.Contains(msg, "drill-down") {
		t.Fatalf("expected drill-down message, got changed=%t msg=%q", changed, msg)
	}

	text := d.Render(status, 120, 0, time.Now().UTC())
	if !strings.Contains(text, "Reason drill-down") {
		t.Fatalf("expected reason drill-down section in render: %s", text)
	}
}

func TestDashboardRenderContainsStatusFields(t *testing.T) {
	t.Parallel()

	d := NewDashboard()
	text := d.Render(DashboardStatus{
		Health:       "healthy",
		NextRunAt:    time.Now().UTC().Add(time.Minute),
		ActiveJobs:   2,
		RecentErrors: []string{"x"},
		UpdatedAt:    time.Now().UTC(),
	}, 120, 0, time.Now().UTC())

	for _, want := range []string{"Overview", "Health", "Active Jobs"} {
		if !strings.Contains(text, want) {
			t.Fatalf("render output missing %q: %s", want, text)
		}
	}
}

func TestDashboardRenderShowsStaleDataIndicator(t *testing.T) {
	t.Parallel()

	d := NewDashboard()
	text := d.Render(DashboardStatus{UpdatedAt: time.Now().UTC().Add(-2 * time.Minute)}, 120, 0, time.Now().UTC())
	if !strings.Contains(text, "Data status: stale") {
		t.Fatalf("expected stale data indicator: %s", text)
	}
}

func TestDashboardStatusPanelFocusCyclesWithTab(t *testing.T) {
	t.Parallel()

	d := NewDashboard()
	changed, msg := d.HandleKey("tab", DashboardStatus{})
	if !changed || !strings.Contains(msg, "recent errors") {
		t.Fatalf("expected focus switch to recent errors, got changed=%t msg=%q", changed, msg)
	}

	text := d.Render(DashboardStatus{}, 120, 0, time.Now().UTC())
	if !strings.Contains(text, "Focused panel:") || !strings.Contains(text, "RECENT ERRORS") {
		t.Fatalf("expected focused panel indicator for recent errors: %s", text)
	}

	changed, msg = d.HandleKey("shift+tab", DashboardStatus{})
	if !changed || !strings.Contains(msg, "system overview") {
		t.Fatalf("expected focus switch back to system overview, got changed=%t msg=%q", changed, msg)
	}
}
