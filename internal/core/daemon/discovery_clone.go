package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/auth"
	"github.com/basmulder03/git-project-sync/internal/core/clone"
	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/providers/api"
	"github.com/basmulder03/git-project-sync/internal/core/state"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
	"github.com/basmulder03/git-project-sync/internal/core/workspace"
)

// DiscoveryCloneOrchestrator handles remote repository discovery and automatic cloning
type DiscoveryCloneOrchestrator struct {
	cfg           config.Config
	logger        *slog.Logger
	tokenStore    auth.TokenStore
	stateStore    state.Store
	clientFactory *api.ClientFactory
	cloneEngine   *clone.Engine
}

// NewDiscoveryCloneOrchestrator creates a new orchestrator instance
func NewDiscoveryCloneOrchestrator(
	cfg config.Config,
	logger *slog.Logger,
	tokenStore auth.TokenStore,
	stateStore state.Store,
) *DiscoveryCloneOrchestrator {
	return &DiscoveryCloneOrchestrator{
		cfg:           cfg,
		logger:        logger,
		tokenStore:    tokenStore,
		stateStore:    stateStore,
		clientFactory: api.NewClientFactory(30 * time.Second),
		cloneEngine:   clone.NewEngine(cfg),
	}
}

// Run executes discovery and clone phases for all enabled sources
func (o *DiscoveryCloneOrchestrator) Run(ctx context.Context, traceID string) error {
	// Check if auto-clone is enabled globally
	if o.cfg.Governance.DefaultPolicy.AutoCloneEnabled != nil && !*o.cfg.Governance.DefaultPolicy.AutoCloneEnabled {
		o.logger.Debug("auto-clone disabled globally, skipping discovery phase", "trace_id", traceID)
		return nil
	}

	o.logger.Info("starting discovery phase", "trace_id", traceID)
	if err := o.recordEvent(ctx, telemetry.Event{
		TraceID:    traceID,
		Level:      "info",
		ReasonCode: telemetry.ReasonDiscoveryStarted,
		Message:    "starting remote repository discovery",
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		o.logger.Warn("failed to record discovery start event", "error", err)
	}

	// Discover remote repositories from all sources
	remoteRepos, err := workspace.DiscoverRemoteRepos(ctx, o.cfg, o.clientFactory, o.getToken)
	if err != nil {
		o.logger.Error("discovery failed", "trace_id", traceID, "error", err)
		_ = o.recordEvent(ctx, telemetry.Event{
			TraceID:    traceID,
			Level:      "error",
			ReasonCode: telemetry.ReasonDiscoveryFailed,
			Message:    fmt.Sprintf("discovery failed: %v", err),
			CreatedAt:  time.Now().UTC(),
		})
		return fmt.Errorf("discover remote repos: %w", err)
	}

	o.logger.Info("discovery completed", "trace_id", traceID, "discovered_count", len(remoteRepos))
	_ = o.recordEvent(ctx, telemetry.Event{
		TraceID:    traceID,
		Level:      "info",
		ReasonCode: telemetry.ReasonDiscoveryCompleted,
		Message:    fmt.Sprintf("discovered %d repositories", len(remoteRepos)),
		CreatedAt:  time.Now().UTC(),
	})

	// Persist discovered repositories to state database
	for _, repo := range remoteRepos {
		discoveredRepo := state.DiscoveredRepo{
			Provider:      repo.Provider,
			SourceID:      repo.SourceID,
			FullName:      repo.FullName,
			CloneURL:      repo.CloneURL,
			DefaultBranch: repo.DefaultBranch,
			IsArchived:    repo.IsArchived,
			SizeKB:        repo.SizeKB,
			DiscoveredAt:  time.Now().UTC(),
		}
		if err := o.stateStore.UpsertDiscoveredRepo(discoveredRepo); err != nil {
			o.logger.Warn("failed to persist discovered repo", "repo", repo.FullName, "error", err)
		}
	}

	// Resolve current local repos
	resolved, err := workspace.ResolveRunRepos(o.cfg)
	if err != nil {
		return fmt.Errorf("resolve local repos: %w", err)
	}

	// Identify repos that need cloning
	reposToClone := workspace.IdentifyReposToClone(o.cfg, remoteRepos, resolved.Repos)
	if len(reposToClone) == 0 {
		o.logger.Debug("no new repositories to clone", "trace_id", traceID)
		return nil
	}

	o.logger.Info("starting clone phase", "trace_id", traceID, "clone_count", len(reposToClone))

	// Clone repositories sequentially
	layout := workspace.NewLayout(o.cfg.Workspace)
	for _, repo := range reposToClone {
		targetPath := layout.RepoPath(repo.Provider, repo.Owner, repo.Name)

		// Record clone operation start
		cloneOp := &state.CloneOperation{
			TraceID:      traceID,
			SourceID:     repo.SourceID,
			RepoFullName: repo.FullName,
			TargetPath:   targetPath,
			Status:       "started",
		}
		if err := o.stateStore.RecordCloneOperation(cloneOp); err != nil {
			o.logger.Warn("failed to record clone operation", "repo", repo.FullName, "error", err)
		}

		o.logger.Info("cloning repository", "trace_id", traceID, "repo", repo.FullName, "target", targetPath)
		_ = o.recordEvent(ctx, telemetry.Event{
			TraceID:    traceID,
			RepoPath:   targetPath,
			Level:      "info",
			ReasonCode: telemetry.ReasonCloneStarted,
			Message:    fmt.Sprintf("cloning %s", repo.FullName),
			CreatedAt:  time.Now().UTC(),
		})

		// Execute clone with retry
		result := o.cloneEngine.CloneWithRetry(ctx, repo, clone.RetryConfig{
			MaxAttempts:        3,
			BaseBackoffSeconds: 2,
		}, false)

		// Update clone operation in database
		completedAt := time.Now().UTC()
		if result.Error != nil {
			o.logger.Error("clone failed", "trace_id", traceID, "repo", repo.FullName, "error", result.Error)
			_ = o.stateStore.UpdateCloneOperation(cloneOp.ID, "failed", result.Error.Error(), completedAt, 0)
			_ = o.recordEvent(ctx, telemetry.Event{
				TraceID:    traceID,
				RepoPath:   targetPath,
				Level:      "error",
				ReasonCode: telemetry.ReasonCloneFailed,
				Message:    fmt.Sprintf("clone failed: %v", result.Error),
				CreatedAt:  completedAt,
			})
			continue
		}

		if !result.Success {
			reasonMessage := fmt.Sprintf("clone unsuccessful: %s", result.ReasonCode)
			o.logger.Warn("clone unsuccessful", "trace_id", traceID, "repo", repo.FullName, "reason", result.ReasonCode)
			_ = o.stateStore.UpdateCloneOperation(cloneOp.ID, result.ReasonCode, reasonMessage, completedAt, 0)
			_ = o.recordEvent(ctx, telemetry.Event{
				TraceID:    traceID,
				RepoPath:   targetPath,
				Level:      "warn",
				ReasonCode: result.ReasonCode,
				Message:    reasonMessage,
				CreatedAt:  completedAt,
			})
			continue
		}

		// Success
		o.logger.Info("clone completed", "trace_id", traceID, "repo", repo.FullName)
		_ = o.stateStore.UpdateCloneOperation(cloneOp.ID, "completed", "", completedAt, 0)
		_ = o.recordEvent(ctx, telemetry.Event{
			TraceID:    traceID,
			RepoPath:   targetPath,
			Level:      "info",
			ReasonCode: telemetry.ReasonCloneCompleted,
			Message:    "clone completed successfully",
			CreatedAt:  completedAt,
		})
	}

	return nil
}

// getToken retrieves a PAT token for the specified source
func (o *DiscoveryCloneOrchestrator) getToken(sourceID string) (string, error) {
	token, err := o.tokenStore.GetToken(context.Background(), sourceID)
	if err != nil {
		return "", fmt.Errorf("retrieve token for source %s: %w", sourceID, err)
	}
	return token, nil
}

// recordEvent writes an event to the state store
func (o *DiscoveryCloneOrchestrator) recordEvent(ctx context.Context, event telemetry.Event) error {
	return o.stateStore.AppendEvent(state.Event{
		TraceID:    event.TraceID,
		RepoPath:   event.RepoPath,
		Level:      event.Level,
		ReasonCode: event.ReasonCode,
		Message:    event.Message,
		CreatedAt:  event.CreatedAt,
	})
}
