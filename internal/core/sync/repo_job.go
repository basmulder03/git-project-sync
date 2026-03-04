package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/git"
)

type RepoJob struct {
	GitClient *git.Client
	Logger    *slog.Logger
}

type RepoJobResult struct {
	RepoPath   string
	Mutated    bool
	Skipped    bool
	ReasonCode string
	Reason     string
	DirtyState git.DirtyState
	TraceID    string
	SourceID   string
	RemoteName string
	Provider   string
	DryRun     bool
}

func NewRepoJob(client *git.Client, logger *slog.Logger) *RepoJob {
	return &RepoJob{GitClient: client, Logger: logger}
}

func (j *RepoJob) Run(ctx context.Context, traceID string, repo config.RepoConfig, dryRun bool) (RepoJobResult, error) {
	result := RepoJobResult{
		RepoPath:   repo.Path,
		TraceID:    traceID,
		SourceID:   repo.SourceID,
		RemoteName: repo.Remote,
		Provider:   repo.Provider,
		DryRun:     dryRun,
	}

	dirty, err := j.GitClient.DirtyState(ctx, repo.Path)
	if err != nil {
		return result, fmt.Errorf("inspect dirty state: %w", err)
	}

	result.DirtyState = dirty

	if repo.SkipIfDirty && dirty.IsDirty() {
		result.Skipped = true
		result.ReasonCode = dirty.ReasonCode()
		result.Reason = "repository is dirty; mutating sync actions are blocked"
		j.Logger.Info("repo sync skipped", "trace_id", traceID, "repo_path", repo.Path, "reason_code", result.ReasonCode, "reason", result.Reason)
		return result, nil
	}

	j.Logger.Info("repo sync preflight passed", "trace_id", traceID, "repo_path", repo.Path, "dry_run", dryRun)
	return result, nil
}
