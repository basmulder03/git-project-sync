package sync

import (
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func TestEvaluatePolicyPatternDecisions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 2, 10, 0, 0, 0, time.UTC)
	source := config.SourceConfig{ID: "gh1"}

	decision := evaluatePolicy(now, source, config.GovernanceConfig{
		DefaultPolicy: config.SyncPolicyConfig{IncludeRepoPatterns: []string{`^/repos/allowed/`}},
	}, config.RepoConfig{Path: "/repos/blocked/x"})
	if decision.allowed || decision.reasonCode != telemetry.ReasonPolicyRepoNotIncluded {
		t.Fatalf("expected include policy block, got %+v", decision)
	}

	decision = evaluatePolicy(now, source, config.GovernanceConfig{
		DefaultPolicy: config.SyncPolicyConfig{ExcludeRepoPatterns: []string{`/repos/blocked/`}},
	}, config.RepoConfig{Path: "/repos/blocked/x"})
	if decision.allowed || decision.reasonCode != telemetry.ReasonPolicyRepoExcluded {
		t.Fatalf("expected exclude policy block, got %+v", decision)
	}

	decision = evaluatePolicy(now, source, config.GovernanceConfig{
		DefaultPolicy: config.SyncPolicyConfig{ProtectedRepoPatterns: []string{`/repos/protected/`}},
	}, config.RepoConfig{Path: "/repos/protected/x"})
	if decision.allowed || decision.reasonCode != telemetry.ReasonPolicyRepoProtected {
		t.Fatalf("expected protected policy block, got %+v", decision)
	}
}

func TestEvaluatePolicyAllowedWindow(t *testing.T) {
	t.Parallel()

	outside := time.Date(2026, time.March, 2, 22, 30, 0, 0, time.UTC) // Monday
	inside := time.Date(2026, time.March, 2, 11, 30, 0, 0, time.UTC)
	gov := config.GovernanceConfig{DefaultPolicy: config.SyncPolicyConfig{AllowedSyncWindows: []config.SyncWindowConfig{{Days: []string{"monday"}, Start: "09:00", End: "18:00"}}}}

	if d := evaluatePolicy(outside, config.SourceConfig{}, gov, config.RepoConfig{Path: "/repos/a"}); d.allowed || d.reasonCode != telemetry.ReasonPolicyOutsideSyncWindow {
		t.Fatalf("expected outside-window block, got %+v", d)
	}
	if d := evaluatePolicy(inside, config.SourceConfig{}, gov, config.RepoConfig{Path: "/repos/a"}); !d.allowed {
		t.Fatalf("expected policy allow within window, got %+v", d)
	}
}
