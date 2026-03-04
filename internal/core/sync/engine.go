package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/git"
	"github.com/basmulder03/git-project-sync/internal/core/providers"
)

type Engine struct {
	git       *git.Client
	preflight *RepoJob
	logger    *slog.Logger
	resolvers map[string]providers.DefaultBranchResolver
}

func NewEngine(client *git.Client, logger *slog.Logger) *Engine {
	return &Engine{
		git:       client,
		preflight: NewRepoJob(client, logger),
		logger:    logger,
		resolvers: map[string]providers.DefaultBranchResolver{
			"github":      providers.NewGitHubResolver(client),
			"azure":       providers.NewAzureDevOpsResolver(client),
			"azuredevops": providers.NewAzureDevOpsResolver(client),
		},
	}
}

func (e *Engine) RunRepo(ctx context.Context, traceID string, source config.SourceConfig, repo config.RepoConfig, dryRun bool) (RepoJobResult, error) {
	result, err := e.preflight.Run(ctx, traceID, repo, dryRun)
	if err != nil || result.Skipped {
		return result, err
	}

	remote := repo.Remote
	if remote == "" {
		remote = "origin"
	}

	if err := e.git.FetchAndPrune(ctx, repo.Path, remote); err != nil {
		return result, err
	}

	defaultBranch, err := e.resolveDefaultBranch(ctx, source, repo)
	if err != nil {
		return result, err
	}

	upstreamRef, ok, err := e.git.UpstreamBranch(ctx, repo.Path)
	if err != nil {
		return result, err
	}

	ahead := 0
	behind := 0
	if !ok {
		e.logger.Info("repo sync skipped", "trace_id", traceID, "repo_path", repo.Path, "reason_code", "upstream_missing", "reason", "current branch has no upstream configured")
		result.Skipped = true
		result.ReasonCode = "upstream_missing"
		result.Reason = "current branch has no upstream configured"
	} else {
		currentBranch, err := e.git.CurrentBranch(ctx, repo.Path)
		if err != nil {
			return result, err
		}

		ahead, behind, err = e.git.AheadBehind(ctx, repo.Path, currentBranch, upstreamRef)
		if err != nil {
			return result, err
		}

		if ahead > 0 && behind > 0 {
			result.Skipped = true
			result.ReasonCode = "non_fast_forward"
			result.Reason = "branch diverged from upstream; manual intervention required"
			e.logger.Info("repo sync skipped", "trace_id", traceID, "repo_path", repo.Path, "reason_code", result.ReasonCode, "reason", result.Reason)
			return result, nil
		}

		if behind > 0 && ahead == 0 {
			if dryRun {
				e.logger.Info("repo sync dry-run", "trace_id", traceID, "repo_path", repo.Path, "action", "fast_forward", "upstream", upstreamRef)
			} else {
				if err := e.git.FastForwardTo(ctx, repo.Path, upstreamRef); err != nil {
					return result, err
				}

				result.Mutated = true
				e.logger.Info("repo fast-forwarded", "trace_id", traceID, "repo_path", repo.Path, "upstream", upstreamRef)
			}
		}
	}

	if repo.CleanupMergedLocalBranches {
		if dryRun {
			e.logger.Info("cleanup dry-run", "trace_id", traceID, "repo_path", repo.Path, "default_branch", defaultBranch)
		} else {
			cleanup, err := e.git.CleanupMergedLocalBranches(ctx, repo.Path, defaultBranch)
			if err != nil {
				return result, err
			}
			if len(cleanup.DeletedBranches) > 0 {
				result.Skipped = false
				result.ReasonCode = ""
				result.Reason = ""
				result.Mutated = true
				e.logger.Info("stale branches deleted", "trace_id", traceID, "repo_path", repo.Path, "deleted_branches", cleanup.DeletedBranches)
			}

			for _, decision := range cleanup.Decisions {
				if decision.Deleted {
					continue
				}
				e.logger.Info("cleanup skipped", "trace_id", traceID, "repo_path", repo.Path, "branch", decision.Branch, "reason_code", decision.ReasonCode, "reason", decision.Reason)
			}
		}
	}

	e.logger.Info("repo sync completed", "trace_id", traceID, "repo_path", repo.Path, "default_branch", defaultBranch, "ahead", ahead, "behind", behind)
	return result, nil
}

func (e *Engine) resolveDefaultBranch(ctx context.Context, source config.SourceConfig, repo config.RepoConfig) (string, error) {
	remote := repo.Remote
	if remote == "" {
		remote = "origin"
	}

	branch, err := e.git.ResolveDefaultBranchFromRemoteHEAD(ctx, repo.Path, remote)
	if err == nil {
		return branch, nil
	}

	provider := strings.ToLower(strings.TrimSpace(source.Provider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(repo.Provider))
	}

	resolver, ok := e.resolvers[provider]
	if !ok {
		return "", fmt.Errorf("default branch resolution failed and no fallback resolver for provider %q", provider)
	}

	fallback, fallbackErr := resolver.ResolveDefaultBranch(ctx, repo.Path, remote)
	if fallbackErr != nil {
		return "", fmt.Errorf("default branch resolution failed (%v) and fallback failed: %w", err, fallbackErr)
	}

	e.logger.Info("default branch resolved via provider fallback", "repo_path", repo.Path, "provider", provider, "branch", fallback)
	return fallback, nil
}
