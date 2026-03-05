package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/state"
)

func TestEventsListAndTraceShowCommands(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	statePath := filepath.Join(dir, "state.db")

	cfg := config.Default()
	cfg.State.DBPath = statePath
	cfg.Repos = []config.RepoConfig{{Path: "/repos/a", SourceID: "gh1", Enabled: true}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	store, err := state.NewSQLiteStore(statePath)
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	_ = store.AppendEvent(state.Event{TraceID: "trace-abc", RepoPath: "/repos/a", Level: "info", ReasonCode: "sync_completed", Message: "done", CreatedAt: now})
	_ = store.AppendEvent(state.Event{TraceID: "trace-abc", RepoPath: "/repos/a", Level: "warn", ReasonCode: "repo_dirty", Message: "skipped", CreatedAt: now.Add(time.Second)})

	eventsOut, err := executeSyncctl("--config", configPath, "events", "list", "--limit", "10")
	if err != nil {
		t.Fatalf("events list failed: %v output=%s", err, eventsOut)
	}
	if !strings.Contains(eventsOut, "trace-abc") {
		t.Fatalf("events output missing trace id: %s", eventsOut)
	}

	traceOut, err := executeSyncctl("--config", configPath, "trace", "show", "trace-abc", "--limit", "10")
	if err != nil {
		t.Fatalf("trace show failed: %v output=%s", err, traceOut)
	}
	if !strings.Contains(traceOut, "repo_dirty") {
		t.Fatalf("trace output missing expected event: %s", traceOut)
	}

	jsonOut, err := executeSyncctl("--config", configPath, "events", "list", "--format", "json", "--source-id", "gh1", "--since", now.Add(-time.Minute).Format(time.RFC3339), "--until", now.Add(5*time.Minute).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("events json export failed: %v output=%s", err, jsonOut)
	}
	for _, want := range []string{"\"trace_id\": \"trace-abc\"", "\"source_id\": \"gh1\""} {
		if !strings.Contains(jsonOut, want) {
			t.Fatalf("events json output missing %q: %s", want, jsonOut)
		}
	}

	csvOut, err := executeSyncctl("--config", configPath, "trace", "show", "trace-abc", "--format", "csv", "--repo-path", "/repos/a")
	if err != nil {
		t.Fatalf("trace csv export failed: %v output=%s", err, csvOut)
	}
	if !strings.Contains(csvOut, "time,trace_id,level,reason_code,repo_path,source_id,message") {
		t.Fatalf("trace csv output missing header: %s", csvOut)
	}
}
