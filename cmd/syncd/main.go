package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
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

	traceID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	engine := coresync.NewEngine(coregit.NewClient(), logger)
	sourcesByID := make(map[string]config.SourceConfig, len(cfg.Sources))
	for _, source := range cfg.Sources {
		sourcesByID[source.ID] = source
	}

	for _, repo := range cfg.Repos {
		source, ok := sourcesByID[repo.SourceID]
		if !ok {
			logger.Info("repo sync skipped", "trace_id", traceID, "repo_path", repo.Path, "reason_code", "source_missing", "reason", "configured source for repository is missing")
			continue
		}

		result, err := engine.RunRepo(context.Background(), traceID, source, repo, *once)
		if err != nil {
			logger.Error("repo sync failed", "trace_id", traceID, "repo_path", repo.Path, "error", err)
			continue
		}

		if result.Skipped {
			continue
		}

		logger.Info("repo sync preflight complete", "trace_id", traceID, "repo_path", repo.Path)
	}

	logger.Info("syncd run completed", "trace_id", traceID, "repo_count", len(cfg.Repos))
	return 0
}

func mode(once bool) string {
	if once {
		return "once"
	}
	return "daemon"
}
