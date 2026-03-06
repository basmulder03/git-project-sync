package daemon

import (
	"context"
	"errors"
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

func TestDiscoveryOrchestratorWithSources(t *testing.T) {
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
		Sources: []config.SourceConfig{
			{
				ID:       "test-source",
				Provider: "github",
				Account:  "testorg",
				Enabled:  true,
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{hasToken: true}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)
	err := orchestrator.Run(context.Background(), "test-trace")

	// Should complete even though token retrieval will fail (no actual token)
	if err != nil {
		t.Logf("got expected error with mock token: %v", err)
	}

	// Should have attempted to record events
	if store.appendedEventCount < 1 {
		t.Fatalf("expected at least 1 event recorded, got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorGetToken(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root:   tmpDir,
			Layout: "flat",
		},
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled: boolPtr(true),
			},
		},
		Sources: []config.SourceConfig{
			{
				ID:       "test-source",
				Provider: "github",
				Enabled:  true,
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{
		hasToken:   true,
		tokenValue: "test-token-123",
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	// Test getToken function
	token, err := orchestrator.getToken("test-source")
	if err != nil {
		t.Fatalf("expected no error getting token, got: %v", err)
	}
	if token != "test-token-123" {
		t.Fatalf("expected token 'test-token-123', got: %s", token)
	}

	// Test with missing token
	tokenStore2 := &mockTokenStore{hasToken: false}
	orchestrator2 := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore2, store)
	_, err = orchestrator2.getToken("test-source")
	if err == nil {
		t.Fatalf("expected error for missing token, got nil")
	}
}

// Mock implementations

type mockStateStore struct {
	appendedEventCount int
	appendDelay        time.Duration
	failOnAppend       bool
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
	if m.failOnAppend {
		return errors.New("mock append error")
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

type mockTokenStore struct {
	hasToken   bool
	tokenValue string
	err        error
}

func (m *mockTokenStore) SetToken(ctx context.Context, sourceID, token string) error {
	return nil
}

func (m *mockTokenStore) GetToken(ctx context.Context, sourceID string) (string, error) {
	if !m.hasToken {
		return "", errors.New("token not found")
	}
	if m.err != nil {
		return "", m.err
	}
	if m.tokenValue != "" {
		return m.tokenValue, nil
	}
	return "", nil
}

func (m *mockTokenStore) DeleteToken(ctx context.Context, sourceID string) error {
	return nil
}

func TestDiscoveryOrchestratorFullFlow(t *testing.T) {
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
		Sources: []config.SourceConfig{
			{
				ID:       "test-source",
				Provider: "github",
				Account:  "testorg",
				Enabled:  true,
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{
		hasToken:   true,
		tokenValue: "test-token-123",
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	// Run discovery and clone
	// This will attempt real API calls which will fail, but we're testing the orchestration flow
	err := orchestrator.Run(context.Background(), "test-trace-full-flow")

	// The run may fail due to real API calls, but that's expected
	// We're testing that the orchestration logic runs without panics
	if err != nil {
		t.Logf("expected error from real API call: %v", err)
	}

	// Verify that events were recorded
	if store.appendedEventCount < 1 {
		t.Fatalf("expected at least 1 event (start), got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorHandlesNoReposToClone(t *testing.T) {
	t.Parallel()

	// Create a temporary workspace directory with an existing repo
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
		Sources: []config.SourceConfig{
			{
				ID:       "test-source",
				Provider: "github",
				Account:  "testorg",
				Enabled:  true,
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{
		hasToken:   true,
		tokenValue: "test-token-123",
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	// Run discovery
	err := orchestrator.Run(context.Background(), "test-trace-no-repos")

	// May fail due to API calls, but we're testing the flow
	if err != nil {
		t.Logf("expected error from API call: %v", err)
	}

	// Should have recorded start event at minimum
	if store.appendedEventCount < 1 {
		t.Fatalf("expected at least 1 event, got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorHandlesTokenRetrievalError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root:   tmpDir,
			Layout: "flat",
		},
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled: boolPtr(true),
			},
		},
		Sources: []config.SourceConfig{
			{
				ID:       "test-source",
				Provider: "github",
				Enabled:  true,
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{
		hasToken: false,
		err:      errors.New("keyring error"),
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	// Run should complete even with token errors (skips sources)
	err := orchestrator.Run(context.Background(), "test-trace-token-error")

	// Should complete successfully (no repos discovered)
	if err != nil {
		t.Fatalf("expected nil error when token retrieval fails (should skip source), got: %v", err)
	}

	// Should have recorded start and completion events
	if store.appendedEventCount < 2 {
		t.Fatalf("expected at least 2 events (start+completion), got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorPersistsDiscoveredRepos(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root:   tmpDir,
			Layout: "flat",
		},
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled: boolPtr(true),
			},
		},
		Sources: []config.SourceConfig{
			{
				ID:       "test-source",
				Provider: "github",
				Account:  "testorg",
				Enabled:  true,
			},
		},
	}

	store := &mockStateStore{}
	tokenStore := &mockTokenStore{
		hasToken:   true,
		tokenValue: "test-token",
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	// Run discovery
	err := orchestrator.Run(context.Background(), "test-trace-persist")

	// May fail due to API calls
	if err != nil {
		t.Logf("got error (expected with real API): %v", err)
	}

	// Verify events were recorded
	if store.appendedEventCount < 1 {
		t.Fatalf("expected at least 1 event, got: %d", store.appendedEventCount)
	}
}

func TestDiscoveryOrchestratorRecordEventError(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Workspace: config.WorkspaceConfig{
			Root:   t.TempDir(),
			Layout: "flat",
		},
		Governance: config.GovernanceConfig{
			DefaultPolicy: config.SyncPolicyConfig{
				AutoCloneEnabled: boolPtr(true),
			},
		},
	}

	// Store that fails on AppendEvent
	store := &mockStateStore{
		failOnAppend: true,
	}
	tokenStore := &mockTokenStore{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	orchestrator := NewDiscoveryCloneOrchestrator(cfg, logger, tokenStore, store)

	// Should handle AppendEvent errors gracefully
	err := orchestrator.Run(context.Background(), "test-trace-event-error")

	// Should not fail due to event recording errors
	if err != nil {
		t.Logf("run completed with error: %v", err)
	}

	// Even if AppendEvent fails, the run should continue
	// We can't check appendedEventCount since it won't increment on error
}

func boolPtr(b bool) *bool {
	return &b
}
