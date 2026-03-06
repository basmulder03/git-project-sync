package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/auth"
	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/daemon"
	coregit "github.com/basmulder03/git-project-sync/internal/core/git"
	"github.com/basmulder03/git-project-sync/internal/core/logging"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("syncd", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath(), "Path to config file")
	once := fs.Bool("once", false, "Run one sync cycle and exit")
	showVersion := fs.Bool("version", false, "Show version and exit")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "syncd: %v\n", err)
		return 2
	}

	if *showVersion {
		fmt.Printf("syncd %s\n", version)
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncd: failed to load config: %v\n", err)
		return 1
	}

	logger, err := logging.New(logging.Options{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncd: failed to initialize logger: %v\n", err)
		return 1
	}

	logger.Info("syncd started", "mode", mode(*once), "workspace_root", cfg.Workspace.Root)

	engine := coresync.NewEngine(coregit.NewClient(), logger)
	store, err := state.NewSQLiteStore(cfg.State.DBPath)
	if err != nil {
		logger.Error("failed to initialize state store", "error", err)
		return 1
	}
	defer func() {
		_ = store.Close()
	}()

	serviceAPI := daemon.NewServiceAPI(store)
	locks := daemon.NewRepoLockManager()
	scheduler := daemon.NewScheduler(cfg.Daemon, logger, locks, engine.RunRepo, serviceAPI)

	// Initialize token store for API authentication
	secretsPath := filepath.Join(filepath.Dir(*configPath), "secrets", "tokens.enc")
	tokenStore, err := auth.NewTokenStore(auth.Options{
		ServiceName:    "git-project-sync",
		FallbackPath:   secretsPath,
		FallbackKeyEnv: "GIT_PROJECT_SYNC_FALLBACK_KEY",
	})
	if err != nil {
		logger.Warn("failed to initialize token store, discovery will be skipped", "error", err)
		tokenStore = nil // Continue without token store
	}

	// Initialize discovery-clone orchestrator
	var discoveryOrchestrator *daemon.DiscoveryCloneOrchestrator
	if tokenStore != nil {
		discoveryOrchestrator = daemon.NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)
	}

	// Helper function to build sync tasks from discovered repositories
	buildTasks := func() []daemon.RepoTask {
		resolved, err := workspace.ResolveRunRepos(cfg)
		if err != nil {
			logger.Error("failed to resolve repositories", "error", err)
			return nil
		}
		for _, skipped := range resolved.Skipped {
			logger.Info("repo sync skipped", "repo_path", skipped, "reason_code", "source_missing", "reason", "unable to resolve source for discovered repository")
		}

		tasks := make([]daemon.RepoTask, 0, len(resolved.Repos))
		sourcesByID := enabledSourcesByID(cfg.Sources)
		for _, repo := range resolved.Repos {
			if !repo.Enabled {
				continue
			}
			source, ok := sourcesByID[repo.SourceID]
			if !ok {
				logger.Info("repo sync skipped", "repo_path", repo.Path, "reason_code", "source_missing", "reason", "configured source for repository is missing or disabled")
				continue
			}
			tasks = append(tasks, daemon.RepoTask{Source: source, Repo: repo})
		}
		return tasks
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *once {
		traceID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())

		// Run discovery and auto-clone if enabled
		if discoveryOrchestrator != nil {
			if err := discoveryOrchestrator.Run(ctx, traceID); err != nil {
				logger.Warn("discovery phase failed", "error", err, "trace_id", traceID)
			}
		}

		// Build tasks after discovery completes (may include newly cloned repos)
		tasks := buildTasks()
		if tasks == nil {
			return 1
		}

		scheduler.RunCycle(ctx, traceID, tasks, false)
		logger.Info("syncd run completed", "trace_id", traceID, "repo_count", len(tasks))
		return 0
	}

	// Periodic mode: run discovery+clone, then enter periodic sync loop
	if discoveryOrchestrator != nil {
		traceID := fmt.Sprintf("discovery-%d", time.Now().UTC().UnixNano())
		if err := discoveryOrchestrator.Run(ctx, traceID); err != nil {
			logger.Warn("initial discovery phase failed", "error", err, "trace_id", traceID)
		}
	}

	// Run periodic sync with discovery at configured intervals
	lastDiscovery := time.Now()
	discoveryInterval := time.Duration(cfg.Daemon.DiscoveryIntervalSeconds) * time.Second

	for {
		// Build task list (may include newly discovered repos)
		tasks := buildTasks()
		if tasks == nil {
			return 1
		}

		// Run one sync cycle
		traceID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
		scheduler.RunCycle(ctx, traceID, tasks, false)

		// Calculate wait time until next sync
		syncInterval := time.Duration(cfg.Daemon.IntervalSeconds) * time.Second
		if syncInterval <= 0 {
			syncInterval = 5 * time.Minute
		}
		jitter := time.Duration(cfg.Daemon.JitterSeconds) * time.Second
		if jitter > 0 {
			jitter = time.Duration(time.Now().UnixNano()%(int64(jitter)+1)) * time.Nanosecond
		}
		waitFor := syncInterval + jitter

		logger.Info("scheduler sleeping", "duration_ms", waitFor.Milliseconds())

		// Sleep until next cycle or context cancellation
		select {
		case <-ctx.Done():
			logger.Info("syncd stopped")
			return 0
		case <-time.After(waitFor):
		}

		// Check if it's time to run discovery again
		if discoveryOrchestrator != nil && discoveryInterval > 0 && time.Since(lastDiscovery) >= discoveryInterval {
			traceID := fmt.Sprintf("discovery-%d", time.Now().UTC().UnixNano())
			logger.Info("running periodic discovery", "trace_id", traceID, "interval_seconds", cfg.Daemon.DiscoveryIntervalSeconds)
			if err := discoveryOrchestrator.Run(ctx, traceID); err != nil {
				logger.Warn("periodic discovery phase failed", "error", err, "trace_id", traceID)
			}
			lastDiscovery = time.Now()
		}
	}
}

func enabledSourcesByID(sources []config.SourceConfig) map[string]config.SourceConfig {
	byID := make(map[string]config.SourceConfig, len(sources))
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		byID[source.ID] = source
	}
	return byID
}

func mode(once bool) string {
	if once {
		return "once"
	}
	return "daemon"
}
