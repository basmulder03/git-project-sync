package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/install"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestDoctorShowsHealthScoreAndFindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	cfg.Sources = []config.SourceConfig{{ID: "gh1", Provider: "github", Account: "jane", Enabled: true}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store, err := state.NewSQLiteStore(statePath)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_ = store.AppendEvent(state.Event{TraceID: "trace-1", RepoPath: "/repos/a", Level: "error", ReasonCode: "sync_failed", Message: "boom", CreatedAt: time.Now().UTC()})
	_ = store.UpsertRunState(state.RunState{RunID: "run-1", TraceID: "trace-1", RepoPath: "/repos/a", SourceID: "gh1", Status: "running", Note: "in-flight"})

	out, err := executeSyncctl("--config", configPath, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v output=%s", err, out)
	}
	for _, want := range []string{"health_score", "finding: source_auth_missing", "finding: failed_jobs_last_hour"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q: %s", want, out)
		}
	}
}

func TestStatsShowOutputsRuntimeCounters(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	cfg.Repos = []config.RepoConfig{{Path: "/repos/a", Enabled: true}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store, err := state.NewSQLiteStore(statePath)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_ = store.PutRepoState(state.RepoState{RepoPath: "/repos/a", LastStatus: "ok", LastError: "", CurrentHash: "abc", UpdatedAt: time.Now().UTC()})
	_ = store.AppendEvent(state.Event{TraceID: "trace-1", RepoPath: "/repos/a", Level: "warn", ReasonCode: "repo_locked", Message: "skip", CreatedAt: time.Now().UTC()})

	out, err := executeSyncctl("--config", configPath, "stats", "show")
	if err != nil {
		t.Fatalf("stats show failed: %v output=%s", err, out)
	}
	for _, want := range []string{"repos_configured", "repo_states", "events_warn"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stats output missing %q: %s", want, out)
		}
	}
}

func TestStatsShowJSONAndCSVExports(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	cfg.Repos = []config.RepoConfig{{Path: "/repos/a", Enabled: true, SourceID: "gh1"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	jsonOut, err := executeSyncctl("--config", configPath, "stats", "show", "--format", "json")
	if err != nil {
		t.Fatalf("stats json failed: %v output=%s", err, jsonOut)
	}
	if !strings.Contains(jsonOut, "\"summary\"") {
		t.Fatalf("stats json output missing summary: %s", jsonOut)
	}

	csvOut, err := executeSyncctl("--config", configPath, "stats", "show", "--format", "csv")
	if err != nil {
		t.Fatalf("stats csv failed: %v output=%s", err, csvOut)
	}
	if !strings.Contains(csvOut, "metric,value") {
		t.Fatalf("stats csv output missing header: %s", csvOut)
	}
}

func TestDoctorIncludesInstallPreflightReasonCodes(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store, err := state.NewSQLiteStore(statePath)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	original := evaluateInstallPreflight
	evaluateInstallPreflight = func(mode install.Mode) []install.Finding {
		if mode != install.ModeSystem {
			t.Fatalf("expected mode system, got %s", mode)
		}
		return []install.Finding{{
			Severity: "critical",
			Code:     install.ReasonInstallDependencyMissing,
			Message:  "missing required dependency: systemctl",
			Hint:     "install systemctl",
		}}
	}
	t.Cleanup(func() {
		evaluateInstallPreflight = original
	})

	out, err := executeSyncctl("--config", configPath, "doctor", "--install-mode", "system")
	if err != nil {
		t.Fatalf("doctor command failed: %v output=%s", err, out)
	}
	for _, want := range []string{"finding: install_preflight reason_code=install_dependency_missing", "severity=critical", "hint: install systemctl"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q: %s", want, out)
		}
	}
}

func TestDoctorIncludesGovernanceAndWorkspaceDriftFindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	cfg.Workspace.Root = filepath.Join(dir, "workspace")
	cfg.Sources = []config.SourceConfig{{ID: "gh1", Provider: "github", Account: "jane", Enabled: true}}
	cfg.Governance.SourcePolicies = map[string]config.SyncPolicyConfig{"ghost-source": {ProtectedRepoPatterns: []string{"/x/"}}}
	cfg.Repos = []config.RepoConfig{{Path: "/tmp/non-managed", SourceID: "gh1", Enabled: true}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := executeSyncctl("--config", configPath, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v output=%s", err, out)
	}
	for _, want := range []string{"finding: governance_drift reason_code=governance_policy_source_missing", "finding: workspace_drift", "syncctl workspace layout fix --dry-run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q: %s", want, out)
		}
	}
}
