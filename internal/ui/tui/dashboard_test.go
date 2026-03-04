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

	if !d.HandleKey("right") {
		t.Fatal("expected right key to move tab")
	}
	if d.SelectedTab() != "repos" {
		t.Fatalf("selected tab = %q, want repos", d.SelectedTab())
	}

	if !d.HandleKey("left") {
		t.Fatal("expected left key to move tab")
	}
	if d.SelectedTab() != "status" {
		t.Fatalf("selected tab = %q, want status", d.SelectedTab())
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
