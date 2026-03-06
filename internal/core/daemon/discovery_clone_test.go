package daemon

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestDiscoveryOrchestratorSkipsWhenAutoCloneDisabled(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled: boolPtr(false),
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)
	err := orchestrator.Run(context.Background(), "test-trace")

	// Should return nil without error when disabled
	if err != nil {
		t.Fatalf("expected nil error when auto-clone disabled, got: %v", err)
	}

	// Should not have appended any events
	if store.appendedEventCount > 0 {
		t.Fatalf("expected no events appended when disabled, got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorRunsWithEnabledAutoClone(t *testing.T) {
	t.Parallel()

	// Create a temporary workspace directory
	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root:   tmpDir,
			Layout: "flat",
		},
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled:         boolPtr(true),
				AutoCloneMaxSizeMB:       2048,
				AutoCloneIncludeArchived: false,
			},
		},
		Sources: []config.SourceConfig{},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)
	err := orchestrator.Run(context.Background(), "test-trace")

	// Should complete without error even with no sources
	if err != nil {
		t.Fatalf("expected nil error with enabled auto-clone, got: %v", err)
	}

	// Should have appended start and completion events
	if store.appendedEventCount < 2 {
		t.Fatalf("expected at least 2 events (start+completion), got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorContextCancellation(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled: boolPtr(true),
			},
		},
	}

	store := &mockStateStore{
		// Simulate slow operation
		appendDelay: 100 * time.Millisecond,
	}
	tokenStore := &mockTokenStore{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := orchestrator.Run(ctx, "test-trace")

	// Should handle cancellation gracefully
	if err != nil && err.Error() != "discover remote repos: context canceled" {
		t.Logf("got error: %v", err)
	}
}

// Mock implementations

type mockStateStore struct {
	appendedEventCount int
	appendDelay        time.Duration
}

func (m *mockStateStore) EnsureSchema() error {
	return nil
}

func (m *mockStateStore) PutRepoState(state.RepoState) error {
	return nil
}

func (m *mockStateStore) GetRepoState(repoPath string) (state.RepoState, bool, error) {
	return state.RepoState{}, false, nil
}

func (m *mockStateStore) ListRepoStates(limit int) ([]state.RepoState, error) {
	return []state.RepoState{}, nil
}

func (m *mockStateStore) UpsertRunState(state.RunState) error {
	return nil
}

func (m *mockStateStore) CompleteRunState(runID, status, note string) error {
	return nil
}

func (m *mockStateStore) ListInFlightRunStates(limit int) ([]state.RunState, error) {
	return []state.RunState{}, nil
}

func (m *mockStateStore) AppendEvent(event state.Event) error {
	if m.appendDelay > 0 {
		time.Sleep(m.appendDelay)
	}
	m.appendedEventCount++
	return nil
}

func (m *mockStateStore) ListEvents(limit int) ([]state.Event, error) {
	return []state.Event{}, nil
}

func (m *mockStateStore) ListEventsByTrace(traceID string, limit int) ([]state.Event, error) {
	return []state.Event{}, nil
}

func (m *mockStateStore) UpsertDiscoveredRepo(state.DiscoveredRepo) error {
	return nil
}

func (m *mockStateStore) GetDiscoveredRepo(sourceID, fullName string) (state.DiscoveredRepo, bool, error) {
	return state.DiscoveredRepo{}, false, nil
}

func (m *mockStateStore) ListDiscoveredRepos(sourceID string, limit int) ([]state.DiscoveredRepo, error) {
	return []state.DiscoveredRepo{}, nil
}

func (m *mockStateStore) DeleteDiscoveredReposBySource(sourceID string) error {
	return nil
}

func (m *mockStateStore) RecordCloneOperation(*state.CloneOperation) error {
	return nil
}

func (m *mockStateStore) UpdateCloneOperation(id int64, status, errorMessage string, completedAt time.Time, retryCount int) error {
	return nil
}

func (m *mockStateStore) GetCloneOperation(id int64) (state.CloneOperation, bool, error) {
	return state.CloneOperation{}, false, nil
}

func (m *mockStateStore) ListCloneOperations(limit int) ([]state.CloneOperation, error) {
	return []state.CloneOperation{}, nil
}

func (m *mockStateStore) ListCloneOperationsByTrace(traceID string, limit int) ([]state.CloneOperation, error) {
	return []state.CloneOperation{}, nil
}

func (m *mockStateStore) Close() error {
	return nil
}

type mockTokenStore struct{}

func (m *mockTokenStore) SetToken(ctx context.Context, sourceID, token string) error {
	return nil
}

func (m *mockTokenStore) GetToken(ctx context.Context, sourceID string) (string, error) {
	return "", nil
}

func (m *mockTokenStore) DeleteToken(ctx context.Context, sourceID string) error {
	return nil
}

func boolPtr(b bool) *bool {
	return &b
}
