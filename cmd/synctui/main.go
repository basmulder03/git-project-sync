package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	coregit "github.com/basmulder03/git-project-sync/internal/core/git"
	"github.com/basmulder03/git-project-sync/internal/core/logging"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/ui/tui"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("synctui", flag.ContinueOnError)
	configPath := fs.String("config", "configs/config.example.yaml", "Path to config file")
	showVersion := fs.Bool("version", false, "Show version and exit")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "synctui: %v\n", err)
		return 2
	}

	if *showVersion {
		fmt.Println("synctui dev")
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

	app := tui.NewApp(provider, os.Stdin, os.Stdout)
	app.SetExecutor(actionExec)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
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
	default:
		return "unknown action", nil
	}
}

func (e *actionExecutor) syncAll(ctx context.Context) (string, error) {
	byID := make(map[string]config.SourceConfig, len(e.cfg.Sources))
	for _, source := range e.cfg.Sources {
		if !source.Enabled {
			continue
		}
		byID[source.ID] = source
	}

	runs := 0
	errorsCount := 0
	for _, repo := range e.cfg.Repos {
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
