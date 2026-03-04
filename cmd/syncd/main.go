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
	coresync "github.com/basmulder03/git-project-sync/internal/core/sync"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("syncd", flag.ContinueOnError)
	configPath := fs.String("config", "configs/config.example.yaml", "Path to config file")
	once := fs.Bool("once", false, "Run one sync cycle and exit (stub)")
	showVersion := fs.Bool("version", false, "Show version and exit")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "syncd: %v\n", err)
		return 2
	}

	if *showVersion {
		fmt.Println("syncd dev")
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
	locks := daemon.NewRepoLockManager()
	scheduler := daemon.NewScheduler(cfg.Daemon, logger, locks, engine.RunRepo)

	tasks := make([]daemon.RepoTask, 0, len(cfg.Repos))
	sourcesByID := enabledSourcesByID(cfg.Sources)
	for _, repo := range cfg.Repos {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *once {
		traceID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
		scheduler.RunCycle(ctx, traceID, tasks, false)
		logger.Info("syncd run completed", "trace_id", traceID, "repo_count", len(tasks))
		return 0
	}

	if err := scheduler.RunPeriodic(ctx, tasks, false); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("scheduler exited with error", "error", err)
		return 1
	}

	logger.Info("syncd stopped")
	return 0
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
