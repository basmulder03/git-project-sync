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
}
