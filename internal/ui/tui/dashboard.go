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
	selected        int
	tabs            []string
	repoFilterIndex int
	logFilterIndex  int
	reasonDetail    string
}

func NewDashboard() *Dashboard {
	return &Dashboard{tabs: []string{"status", "repos", "cache", "logs"}}
}

func (d *Dashboard) SelectedTab() string {
	return d.tabs[d.selected]
}

func (d *Dashboard) HandleKey(key string, status DashboardStatus) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "left", "h":
		if d.selected == 0 {
			return false, ""
		}
		d.selected--
		return true, ""
	case "right", "l":
		if d.selected >= len(d.tabs)-1 {
			return false, ""
		}
		d.selected++
		return true, ""
	case "f":
		switch d.SelectedTab() {
		case "repos":
			d.repoFilterIndex = (d.repoFilterIndex + 1) % len(repoFilters)
			return true, "repos filter: " + repoFilters[d.repoFilterIndex]
		case "logs":
			d.logFilterIndex = (d.logFilterIndex + 1) % len(logFilters)
			d.reasonDetail = ""
			return true, "logs filter: " + logFilters[d.logFilterIndex]
		}
	case "d":
		if d.SelectedTab() != "logs" {
			return false, ""
		}
		events := d.filteredEvents(status.Events)
		if len(events) == 0 {
			d.reasonDetail = ""
			return true, "no events available for drill-down"
		}
		d.reasonDetail = d.reasonSummary(events)
		return true, "reason drill-down selected"
	case "x":
		if d.reasonDetail == "" {
			return false, ""
		}
		d.reasonDetail = ""
		return true, "cleared reason drill-down"
	default:
		return false, ""
	}

	return false, ""
}

func (d *Dashboard) Render(status DashboardStatus) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "Git Project Sync - TUI\n")
	fmt.Fprintf(b, "Tabs: %s\n\n", d.renderTabs())

	switch d.SelectedTab() {
	case "repos":
		RenderReposView(b, d.filteredRepos(status.Repos), repoFilters[d.repoFilterIndex])
	case "cache":
		RenderCacheView(b, status.Cache)
	case "logs":
		RenderLogsView(b, d.filteredEvents(status.Events), status.RecentErrors, logFilters[d.logFilterIndex], d.reasonDetail)
	default:
		d.renderStatus(b, status)
	}

	fmt.Fprintf(b, "\nKeys: h/left l/right r(refresh) f(filter) d(drill-down) x(clear drill-down) s(sync all) c(cache refresh) t(trace) /(command palette) q(quit)\n")
	return b.String()
}

var repoFilters = []string{"all", "error", "warning", "success", "unknown"}
var logFilters = []string{"all", "error", "warn", "info"}

func (d *Dashboard) filteredRepos(repos []RepoRow) []RepoRow {
	filter := repoFilters[d.repoFilterIndex]
	if filter == "all" {
		return repos
	}
	out := make([]RepoRow, 0, len(repos))
	for _, repo := range repos {
		status := strings.ToLower(strings.TrimSpace(repo.LastStatus))
		if status == "" {
			status = "unknown"
		}
		switch filter {
		case "error":
			if strings.Contains(status, "error") || strings.Contains(status, "fail") {
				out = append(out, repo)
			}
		case "warning":
			if strings.Contains(status, "warn") || strings.Contains(status, "degrad") {
				out = append(out, repo)
			}
		case "success":
			if status == "ok" || status == "success" || status == "completed" {
				out = append(out, repo)
			}
		case "unknown":
			if status == "unknown" {
				out = append(out, repo)
			}
		}
	}
	return out
}

func (d *Dashboard) filteredEvents(events []EventRow) []EventRow {
	filter := logFilters[d.logFilterIndex]
	if filter == "all" {
		return events
	}
	out := make([]EventRow, 0, len(events))
	for _, event := range events {
		if strings.EqualFold(strings.TrimSpace(event.Level), filter) {
			out = append(out, event)
		}
	}
	return out
}

func (d *Dashboard) reasonSummary(events []EventRow) string {
	topReason := "unknown"
	count := 0
	byReason := map[string]int{}
	for _, event := range events {
		reason := strings.TrimSpace(event.ReasonCode)
		if reason == "" {
			reason = "unknown"
		}
		byReason[reason]++
		if byReason[reason] > count {
			count = byReason[reason]
			topReason = reason
		}
	}
	return fmt.Sprintf("reason=%s occurrences=%d filtered_events=%d", topReason, count, len(events))
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
	stale := false
	if !status.UpdatedAt.IsZero() {
		updated = status.UpdatedAt.UTC().Format(time.RFC3339)
		stale = time.Since(status.UpdatedAt) > 30*time.Second
	}

	fmt.Fprintf(b, "Daemon health: %s\n", health)
	fmt.Fprintf(b, "Next run:      %s\n", nextRun)
	fmt.Fprintf(b, "Active jobs:   %d\n", status.ActiveJobs)
	fmt.Fprintf(b, "Updated:       %s\n", updated)
	if stale {
		fmt.Fprintf(b, "Data status:   stale (no refresh in >30s)\n")
	}
}
