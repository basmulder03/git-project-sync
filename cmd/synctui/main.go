package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/basmulder03/git-project-sync/internal/app/commands"
	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	coregit "github.com/basmulder03/git-project-sync/internal/core/git"
	"github.com/basmulder03/git-project-sync/internal/core/logging"
	"github.com/basmulder03/git-project-sync/internal/core/maintenance"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
	"github.com/basmulder03/git-project-sync/internal/ui/tui"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("synctui", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath(), "Path to config file")
	showVersion := fs.Bool("version", false, "Show version and exit")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "synctui: %v\n", err)
		return 2
	}

	if *showVersion {
		fmt.Printf("synctui %s\n", version)
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "synctui: failed to load config: %v\n", err)
		return 1
	}

	store, err := state.NewSQLiteStore(cfg.State.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "synctui: failed to open state DB: %v\n", err)
		return 1
	}
	defer func() {
		_ = store.Close()
	}()

	provider := &statusProvider{
		api:              daemon.NewServiceAPI(store),
		interval:         time.Duration(cfg.Daemon.IntervalSeconds) * time.Second,
		providerCacheTTL: time.Duration(cfg.Cache.ProviderTTLSeconds) * time.Second,
		branchCacheTTL:   time.Duration(cfg.Cache.BranchTTLSeconds) * time.Second,
	}
	logger, err := logging.New(logging.Options{Level: cfg.Logging.Level, Format: cfg.Logging.Format})
	if err != nil {
		fmt.Fprintf(os.Stderr, "synctui: failed to initialize logger: %v\n", err)
		return 1
	}
	engine := coresync.NewEngine(coregit.NewClient(), logger)

	actionExec := &actionExecutor{
		cfg:    cfg,
		api:    daemon.NewServiceAPI(store),
		engine: engine,
	}

	model := tui.NewModel(provider, actionExec)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "synctui: %v\n", err)
		return 1
	}

	return 0
}

type statusProvider struct {
	api              *daemon.ServiceAPI
	interval         time.Duration
	providerCacheTTL time.Duration
	branchCacheTTL   time.Duration
}

type actionExecutor struct {
	cfg    config.Config
	api    *daemon.ServiceAPI
	engine *coresync.Engine
}

func (e *actionExecutor) Execute(ctx context.Context, request tui.ActionRequest) (string, error) {
	switch request.Type {
	case tui.ActionSyncAll:
		return e.syncAll(ctx)
	case tui.ActionCacheRefresh:
		cacheSvc := commands.NewCacheService(e.api)
		if err := cacheSvc.Refresh(ctx, commands.CacheTargetAll); err != nil {
			return "", err
		}
		return "cache refresh completed (CLI equivalent: syncctl cache refresh all)", nil
	case tui.ActionTraceDrilldown:
		if request.TraceID == "" {
			return "no trace ID available", nil
		}
		events, err := e.api.Trace(request.TraceID, 200)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("trace %s has %d event(s) (CLI equivalent: syncctl trace show %s)", request.TraceID, len(events), request.TraceID), nil
	case tui.ActionRunCommand:
		return e.runCommand(ctx, request.Command)
	default:
		return "unknown action", nil
	}
}

func (e *actionExecutor) syncAll(ctx context.Context) (string, error) {
	resolved, err := workspace.ResolveRunRepos(e.cfg)
	if err != nil {
		return "", err
	}

	byID := make(map[string]config.SourceConfig, len(e.cfg.Sources))
	for _, source := range e.cfg.Sources {
		if !source.Enabled {
			continue
		}
		byID[source.ID] = source
	}

	runs := 0
	errorsCount := 0
	for _, repo := range resolved.Repos {
		if !repo.Enabled {
			continue
		}
		source, ok := byID[repo.SourceID]
		if !ok {
			continue
		}
		runs++
		traceID := fmt.Sprintf("tui-%d", time.Now().UTC().UnixNano())
		if _, err := e.engine.RunRepo(ctx, traceID, source, repo, false); err != nil {
			errorsCount++
		}
	}

	return fmt.Sprintf("sync all finished: repos=%d errors=%d (CLI equivalent: syncctl sync all)", runs, errorsCount), nil
}

func (e *actionExecutor) runCommand(ctx context.Context, raw string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) == 0 {
		return "", fmt.Errorf("command is required")
	}

	switch parts[0] {
	case "doctor":
		if len(parts) == 1 {
			events, err := e.api.ListEvents(500)
			if err != nil {
				return "", err
			}
			runs, err := e.api.InFlightRuns(200)
			if err != nil {
				return "", err
			}
			missingCreds := 0
			for _, source := range e.cfg.Sources {
				if source.Enabled && source.CredentialRef == "" {
					missingCreds++
				}
			}
			return fmt.Sprintf("doctor: events=%d in_flight=%d missing_credentials=%d", len(events), len(runs), missingCreds), nil
		}
	case "sync":
		if len(parts) == 2 && parts[1] == "all" {
			return e.syncAll(ctx)
		}
	case "discover":
		if len(parts) == 1 {
			resolved, err := workspace.ResolveRunRepos(e.cfg)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("discover: repos=%d skipped=%d", len(resolved.Repos), len(resolved.Skipped)), nil
		}
	case "auth":
		if len(parts) == 2 && parts[1] == "status" {
			enabledSources := 0
			sshEnabled := e.cfg.SSH.Enabled
			for _, source := range e.cfg.Sources {
				if source.Enabled {
					enabledSources++
				}
			}
			return fmt.Sprintf("auth: enabled_sources=%d ssh_enabled=%t", enabledSources, sshEnabled), nil
		}
	case "cache":
		if len(parts) >= 2 {
			if parts[1] == "show" {
				return fmt.Sprintf("cache: provider_ttl=%ds branch_ttl=%ds", e.cfg.Cache.ProviderTTLSeconds, e.cfg.Cache.BranchTTLSeconds), nil
			}
			target := commands.CacheTargetAll
			if len(parts) >= 3 {
				target = commands.CacheTarget(parts[2])
			}
			svc := commands.NewCacheService(e.api)
			switch parts[1] {
			case "refresh":
				if err := svc.Refresh(ctx, target); err != nil {
					return "", err
				}
				return fmt.Sprintf("cache refresh completed (target=%s)", target), nil
			case "clear":
				if err := svc.Clear(ctx, target); err != nil {
					return "", err
				}
				return fmt.Sprintf("cache clear completed (target=%s)", target), nil
			}
		}
	case "stats":
		if len(parts) == 2 && parts[1] == "show" {
			repoStates, err := e.api.RepoStatuses(1000)
			if err != nil {
				return "", err
			}
			runs, err := e.api.InFlightRuns(1000)
			if err != nil {
				return "", err
			}
			events, err := e.api.ListEvents(1000)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("stats: repos=%d in_flight=%d events=%d", len(repoStates), len(runs), len(events)), nil
		}
	case "events":
		if len(parts) >= 2 && parts[1] == "list" {
			limit := 20
			if len(parts) == 3 {
				parsed, err := strconv.Atoi(parts[2])
				if err != nil {
					return "", fmt.Errorf("invalid events limit: %w", err)
				}
				limit = parsed
			}
			events, err := e.api.ListEvents(limit)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("events listed: count=%d", len(events)), nil
		}
	case "trace":
		if len(parts) == 3 && parts[1] == "show" && parts[2] == "latest" {
			events, err := e.api.ListEvents(1)
			if err != nil {
				return "", err
			}
			if len(events) == 0 || strings.TrimSpace(events[0].TraceID) == "" {
				return "no latest trace found", nil
			}
			traceEvents, err := e.api.Trace(events[0].TraceID, 200)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("trace %s has %d event(s)", events[0].TraceID, len(traceEvents)), nil
		}
	case "repo":
		if len(parts) == 2 && parts[1] == "list" {
			resolved, err := workspace.ResolveRunRepos(e.cfg)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("repos configured: %d", len(resolved.Repos)), nil
		}
	case "source":
		if len(parts) == 2 && parts[1] == "list" {
			return fmt.Sprintf("sources configured: %d", len(e.cfg.Sources)), nil
		}
	case "config":
		if len(parts) == 2 && parts[1] == "show" {
			return fmt.Sprintf("config schema=%d repos=%d sources=%d", e.cfg.SchemaVersion, len(e.cfg.Repos), len(e.cfg.Sources)), nil
		}
	case "workspace":
		if len(parts) == 2 && parts[1] == "show" {
			return fmt.Sprintf("workspace root=%s layout=%s", e.cfg.Workspace.Root, e.cfg.Workspace.Layout), nil
		}
	case "daemon":
		if len(parts) == 2 && parts[1] == "status" {
			events, err := e.api.ListEvents(50)
			if err != nil {
				return "", err
			}
			health := "healthy"
			now := time.Now().UTC()
			for _, event := range events {
				if strings.EqualFold(event.Level, "error") && now.Sub(event.CreatedAt) < 15*time.Minute {
					health = "degraded"
					break
				}
			}
			return fmt.Sprintf("daemon status: %s", health), nil
		}
	case "maintenance":
		if len(parts) == 2 && parts[1] == "status" {
			now := time.Now().UTC()
			mw, desc := maintenance.ActiveWindow(e.cfg.Daemon.MaintenanceWindows, now)
			if mw == nil {
				return fmt.Sprintf("maintenance status: inactive windows=%d", len(e.cfg.Daemon.MaintenanceWindows)), nil
			}
			nextAllowed := maintenance.NextAllowed(e.cfg.Daemon.MaintenanceWindows, now)
			return fmt.Sprintf("maintenance status: active window=%s next_allowed=%s", desc, nextAllowed.Format(time.RFC3339)), nil
		}
	case "update":
		if len(parts) == 2 && parts[1] == "status" {
			return fmt.Sprintf("update: channel=%s auto_check=%t auto_apply=%t", e.cfg.Update.Channel, e.cfg.Update.AutoCheck, e.cfg.Update.AutoApply), nil
		}
	case "state":
		if len(parts) == 2 && parts[1] == "check" {
			store, err := state.NewSQLiteStore(e.cfg.State.DBPath)
			if err != nil {
				return "", err
			}
			defer func() {
				_ = store.Close()
			}()
			if err := store.IntegrityCheck(); err != nil {
				return "", err
			}
			return fmt.Sprintf("state integrity ok (%s)", e.cfg.State.DBPath), nil
		}
	}

	return "", fmt.Errorf("unsupported command %q", raw)
}

func (p *statusProvider) DashboardStatus(ctx context.Context) (tui.DashboardStatus, error) {
	if p.interval <= 0 {
		p.interval = 5 * time.Minute
	}

	events, err := p.api.ListEvents(200)
	if err != nil {
		return tui.DashboardStatus{}, err
	}
	repoStatuses, err := p.api.RepoStatuses(200)
	if err != nil {
		return tui.DashboardStatus{}, err
	}

	recentErrors := make([]string, 0, 5)
	health := "healthy"
	now := time.Now().UTC()

	for _, event := range events {
		if event.Level != "error" {
			continue
		}
		if now.Sub(event.CreatedAt) < 15*time.Minute {
			health = "degraded"
		}
		if len(recentErrors) < 5 {
			recentErrors = append(recentErrors, fmt.Sprintf("%s [%s] %s", event.CreatedAt.UTC().Format(time.RFC3339), event.ReasonCode, event.Message))
		}
	}

	repos := make([]tui.RepoRow, 0, len(repoStatuses))
	for _, repo := range repoStatuses {
		repos = append(repos, tui.RepoRow{
			Path:       repo.RepoPath,
			LastStatus: repo.LastStatus,
			LastSyncAt: repo.LastSyncAt,
			LastError:  repo.LastError,
		})
	}

	cacheRows := []tui.CacheRow{
		{Name: "providers", Age: 0, TTL: p.providerCacheTTL},
		{Name: "branches", Age: 0, TTL: p.branchCacheTTL},
	}

	eventRows := make([]tui.EventRow, 0, 20)
	for i, event := range events {
		if i >= 20 {
			break
		}
		eventRows = append(eventRows, tui.EventRow{
			Time:       event.CreatedAt,
			TraceID:    event.TraceID,
			Level:      event.Level,
			ReasonCode: event.ReasonCode,
			Message:    event.Message,
		})
	}

	return tui.DashboardStatus{
		Health:       health,
		NextRunAt:    now.Add(p.interval),
		ActiveJobs:   0,
		RecentErrors: recentErrors,
		Repos:        repos,
		Cache:        cacheRows,
		Events:       eventRows,
		UpdatedAt:    now,
	}, nil
}
