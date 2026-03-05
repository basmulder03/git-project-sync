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

	text := d.Render(status)
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
	})

	for _, want := range []string{"Daemon health", "Next run", "Active jobs"} {
		if !strings.Contains(text, want) {
			t.Fatalf("render output missing %q: %s", want, text)
		}
	}
}

func TestDashboardRenderShowsStaleDataIndicator(t *testing.T) {
	t.Parallel()

	d := NewDashboard()
	text := d.Render(DashboardStatus{UpdatedAt: time.Now().UTC().Add(-2 * time.Minute)})
	if !strings.Contains(text, "Data status:   stale") {
		t.Fatalf("expected stale data indicator: %s", text)
	}
}
