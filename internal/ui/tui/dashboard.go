package tui

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Data types (public – used by cmd/synctui and tests)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

var (
	repoFilters = []string{"all", "error", "warning", "success", "unknown"}
	logFilters  = []string{"all", "error", "warn", "info"}
)

type Dashboard struct {
	selected        int
	tabs            []string
	repoFilterIndex int
	logFilterIndex  int
	reasonDetail    string
	statusPanel     int
}

func NewDashboard() *Dashboard {
	return &Dashboard{tabs: []string{"status", "repos", "cache", "logs"}}
}

func (d *Dashboard) SelectedTab() string {
	return d.tabs[d.selected]
}

// HandleKey processes a key press and returns (changed, message).
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

	case "tab":
		if d.SelectedTab() != "status" {
			return false, ""
		}
		d.statusPanel = (d.statusPanel + 1) % 2
		if d.statusPanel == 0 {
			return true, "focus: system overview"
		}
		return true, "focus: recent errors"

	case "shift+tab":
		if d.SelectedTab() != "status" {
			return false, ""
		}
		if d.statusPanel == 0 {
			d.statusPanel = 1
		} else {
			d.statusPanel = 0
		}
		if d.statusPanel == 0 {
			return true, "focus: system overview"
		}
		return true, "focus: recent errors"
	}

	return false, ""
}

// Render produces the full TUI view string. width is the terminal width (0 = no wrapping).
func (d *Dashboard) Render(status DashboardStatus, width int, pulseFrame int, lastRefreshAt time.Time) string {
	if width <= 0 {
		width = 110
	}
	contentWidth := width - 2
	if contentWidth < 60 {
		contentWidth = 60
	}
	b := &strings.Builder{}

	nowLabel := time.Now().UTC().Format("2006-01-02 15:04:05Z")
	spinner := []string{"◜", "◠", "◝", "◞", "◡", "◟", "◜", "◠"}[pulseFrame%8]
	refreshPulse := muted("idle")
	if !lastRefreshAt.IsZero() && time.Since(lastRefreshAt) < 2*time.Second {
		refreshPulse = accent("refreshing")
	}
	fmt.Fprintf(b, "%s %s %s\n", bold(accent("Git Project Sync Dashboard")), accent(spinner), muted("| "+nowLabel+" | ")+refreshPulse)
	fmt.Fprintln(b, d.renderTabs())
	fmt.Fprintln(b)

	switch d.SelectedTab() {
	case "repos":
		body := &strings.Builder{}
		RenderReposView(body, d.filteredRepos(status.Repos), repoFilters[d.repoFilterIndex])
		fmt.Fprintln(b, boxWithFocus("Repositories", splitLines(body.String()), contentWidth, true))
	case "cache":
		body := &strings.Builder{}
		RenderCacheView(body, status.Cache)
		fmt.Fprintln(b, boxWithFocus("Cache", splitLines(body.String()), contentWidth, true))
	case "logs":
		body := &strings.Builder{}
		RenderLogsView(body, d.filteredEvents(status.Events), status.RecentErrors, logFilters[d.logFilterIndex], d.reasonDetail)
		fmt.Fprintln(b, boxWithFocus("Events and Logs", splitLines(body.String()), contentWidth, true))
	default:
		left, right := d.renderStatusPanels(status, contentWidth, pulseFrame)
		fmt.Fprintln(b, joinColumns(left, right, contentWidth, 3))
	}

	hints := "h/left l/right tab(shift focus) r(refresh) f(filter) d(drill) x(clear) s(sync) c(cache) t(trace) /(palette) !(rerun) q(quit)"
	fmt.Fprintln(b)
	fmt.Fprintf(b, "%s %s\n", muted("Focused tab:"), bold(accent(strings.ToUpper(d.SelectedTab()))))
	if d.SelectedTab() == "status" {
		focusLabel := "SYSTEM OVERVIEW"
		if d.statusPanel == 1 {
			focusLabel = "RECENT ERRORS"
		}
		fmt.Fprintf(b, "%s %s\n", muted("Focused panel:"), bold(accent(focusLabel)))
	}
	fmt.Fprintln(b, muted(hints))

	return b.String()
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

func (d *Dashboard) renderTabs() string {
	parts := make([]string, 0, len(d.tabs))
	for i, tab := range d.tabs {
		parts = append(parts, pill(tab, i == d.selected))
	}
	return strings.Join(parts, " ")
}

func (d *Dashboard) renderStatusPanels(status DashboardStatus, totalWidth int, pulseFrame int) (string, string) {
	health := status.Health
	if health == "" {
		health = "unknown"
	}
	healthValue := statusBadge(health)

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

	errorCount := 0
	warnCount := 0
	infoCount := 0
	for _, event := range status.Events {
		switch strings.ToLower(strings.TrimSpace(event.Level)) {
		case "error":
			errorCount++
		case "warn", "warning":
			warnCount++
		case "info":
			infoCount++
		}
	}

	totalEvents := len(status.Events)
	if totalEvents < 1 {
		totalEvents = 1
	}
	errRatio := float64(errorCount) / float64(totalEvents)
	warnRatio := float64(warnCount) / float64(totalEvents)
	infoRatio := float64(infoCount) / float64(totalEvents)
	pulseTrail := bar(12, float64(pulseFrame%8)/7.0)

	overviewLines := []string{
		"Health         " + healthValue,
		"Next Run       " + accent(nextRun),
		"Active Jobs    " + accent(fmt.Sprintf("%d", status.ActiveJobs)),
		"Repo Count     " + accent(fmt.Sprintf("%d", len(status.Repos))),
		"Error Events   " + bad(fmt.Sprintf("%d", errorCount)),
		"Warn Events    " + warn(fmt.Sprintf("%d", warnCount)),
		"Info Events    " + accent(fmt.Sprintf("%d", infoCount)),
		"Updated        " + muted(updated),
		"Error Mix      " + bad(bar(20, errRatio)),
		"Warn Mix       " + warn(bar(20, warnRatio)),
		"Info Mix       " + accent(bar(20, infoRatio)),
		"Pulse          " + accent(pulseTrail),
	}
	if stale {
		overviewLines = append(overviewLines, warn("Data status: stale (no refresh in >30s)"))
	}

	errorLines := make([]string, 0, 8)
	if len(status.RecentErrors) == 0 {
		errorLines = append(errorLines, good("No recent errors"))
	} else {
		limit := len(status.RecentErrors)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			errorLines = append(errorLines, bad("!")+" "+clip(strings.TrimSpace(status.RecentErrors[i]), 74))
		}
		if len(status.RecentErrors) > limit {
			errorLines = append(errorLines, muted(fmt.Sprintf("... and %d more", len(status.RecentErrors)-limit)))
		}
	}

	leftW := (totalWidth - 3) / 2
	rightW := totalWidth - 3 - leftW
	leftFocused := d.statusPanel == 0
	rightFocused := d.statusPanel == 1
	left := boxWithFocus("System Overview", overviewLines, leftW, leftFocused)
	right := boxWithFocus("Recent Errors", errorLines, rightW, rightFocused)
	return left, right
}

func (d *Dashboard) filteredRepos(repos []RepoRow) []RepoRow {
	filter := repoFilters[d.repoFilterIndex]
	if filter == "all" {
		return repos
	}
	out := make([]RepoRow, 0, len(repos))
	for _, repo := range repos {
		st := strings.ToLower(strings.TrimSpace(repo.LastStatus))
		if st == "" {
			st = "unknown"
		}
		switch filter {
		case "error":
			if strings.Contains(st, "error") || strings.Contains(st, "fail") {
				out = append(out, repo)
			}
		case "warning":
			if strings.Contains(st, "warn") || strings.Contains(st, "degrad") {
				out = append(out, repo)
			}
		case "success":
			if st == "ok" || st == "success" || st == "completed" {
				out = append(out, repo)
			}
		case "unknown":
			if st == "unknown" {
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
