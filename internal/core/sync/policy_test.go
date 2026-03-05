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

func TestMergePolicyOverridesAndAppends(t *testing.T) {
	t.Parallel()

	base := config.SyncPolicyConfig{
		IncludeRepoPatterns:   []string{`^/repos/`},
		ExcludeRepoPatterns:   []string{`/tmp/`},
		ProtectedRepoPatterns: []string{`/critical/`},
		AllowedSyncWindows:    []config.SyncWindowConfig{{Days: []string{"monday"}, Start: "09:00", End: "17:00"}},
	}
	override := config.SyncPolicyConfig{
		IncludeRepoPatterns:   []string{`^/workspace/`},
		ExcludeRepoPatterns:   []string{`/archive/`},
		ProtectedRepoPatterns: []string{`/regulated/`},
		AllowedSyncWindows:    []config.SyncWindowConfig{{Days: []string{"tuesday"}, Start: "08:00", End: "12:00"}},
	}

	merged := mergePolicy(base, override)
	if len(merged.IncludeRepoPatterns) != 1 || merged.IncludeRepoPatterns[0] != `^/workspace/` {
		t.Fatalf("expected include override, got %+v", merged.IncludeRepoPatterns)
	}
	if len(merged.ExcludeRepoPatterns) != 2 {
		t.Fatalf("expected exclude append, got %+v", merged.ExcludeRepoPatterns)
	}
	if len(merged.ProtectedRepoPatterns) != 2 {
		t.Fatalf("expected protected append, got %+v", merged.ProtectedRepoPatterns)
	}
	if len(merged.AllowedSyncWindows) != 1 || merged.AllowedSyncWindows[0].Days[0] != "tuesday" {
		t.Fatalf("expected window override, got %+v", merged.AllowedSyncWindows)
	}
}

func TestMapDayAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want time.Weekday
		ok   bool
	}{
		{in: "sun", want: time.Sunday, ok: true},
		{in: "monday", want: time.Monday, ok: true},
		{in: "Tue", want: time.Tuesday, ok: true},
		{in: "wednesday", want: time.Wednesday, ok: true},
		{in: "thu", want: time.Thursday, ok: true},
		{in: "Friday", want: time.Friday, ok: true},
		{in: "sat", want: time.Saturday, ok: true},
		{in: "funday", want: time.Sunday, ok: false},
	}

	for _, tc := range cases {
		got, ok := mapDay(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("mapDay(%q) = (%v,%t), want (%v,%t)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
