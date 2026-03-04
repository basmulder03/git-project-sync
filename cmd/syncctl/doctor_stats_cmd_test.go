package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
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
