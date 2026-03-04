package tui

import (
	"fmt"
	"strings"
	"time"
)

type DashboardStatus struct {
	Health       string
	NextRunAt    time.Time
	ActiveJobs   int
	RecentErrors []string
	Repos        []RepoRow
	Cache        []CacheRow
	Events       []EventRow
	UpdatedAt    time.Time
}

type RepoRow struct {
	Path       string
	LastStatus string
	LastSyncAt time.Time
	LastError  string
}

type CacheRow struct {
	Name string
	Age  time.Duration
	TTL  time.Duration
}

type EventRow struct {
	Time       time.Time
	TraceID    string
	Level      string
	ReasonCode string
	Message    string
}

type Dashboard struct {
	selected int
	tabs     []string
}

func NewDashboard() *Dashboard {
	return &Dashboard{tabs: []string{"status", "repos", "cache", "logs"}}
}

func (d *Dashboard) SelectedTab() string {
	return d.tabs[d.selected]
}

func (d *Dashboard) HandleKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "left", "h":
		if d.selected == 0 {
			return false
		}
		d.selected--
		return true
	case "right", "l":
		if d.selected >= len(d.tabs)-1 {
			return false
		}
		d.selected++
		return true
	default:
		return false
	}
}

func (d *Dashboard) Render(status DashboardStatus) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "Git Project Sync - TUI\n")
	fmt.Fprintf(b, "Tabs: %s\n\n", d.renderTabs())

	switch d.SelectedTab() {
	case "repos":
		RenderReposView(b, status.Repos)
	case "cache":
		RenderCacheView(b, status.Cache)
	case "logs":
		RenderLogsView(b, status.Events, status.RecentErrors)
	default:
		d.renderStatus(b, status)
	}

	fmt.Fprintf(b, "\nKeys: h/left l/right r(refresh) s(sync all) c(cache refresh) t(trace) q(quit)\n")
	return b.String()
}

func (d *Dashboard) renderTabs() string {
	out := make([]string, 0, len(d.tabs))
	for i, tab := range d.tabs {
		if i == d.selected {
			out = append(out, "["+strings.ToUpper(tab)+"]")
			continue
		}
		out = append(out, tab)
	}
	return strings.Join(out, " ")
}

func (d *Dashboard) renderStatus(b *strings.Builder, status DashboardStatus) {
	health := status.Health
	if health == "" {
		health = "unknown"
	}

	nextRun := "unknown"
	if !status.NextRunAt.IsZero() {
		nextRun = status.NextRunAt.UTC().Format(time.RFC3339)
	}

	updated := "unknown"
	if !status.UpdatedAt.IsZero() {
		updated = status.UpdatedAt.UTC().Format(time.RFC3339)
	}

	fmt.Fprintf(b, "Daemon health: %s\n", health)
	fmt.Fprintf(b, "Next run:      %s\n", nextRun)
	fmt.Fprintf(b, "Active jobs:   %d\n", status.ActiveJobs)
	fmt.Fprintf(b, "Updated:       %s\n", updated)
}
