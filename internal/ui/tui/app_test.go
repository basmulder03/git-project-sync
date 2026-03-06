package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct{}

func (fakeProvider) DashboardStatus(context.Context) (DashboardStatus, error) {
	return DashboardStatus{Health: "healthy", UpdatedAt: time.Now().UTC()}, nil
}

type fakeExecutor struct {
	requests []ActionRequest
}

func (e *fakeExecutor) Execute(_ context.Context, request ActionRequest) (string, error) {
	e.requests = append(e.requests, request)
	return "ok", nil
}

func TestAppRunQuitsOnQKey(t *testing.T) {
	t.Parallel()

	in := bytes.NewBufferString("q")
	out := &bytes.Buffer{}
	app := NewApp(fakeProvider{}, in, out)
	app.refreshInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := app.Run(ctx); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	if !strings.Contains(out.String(), "Git Project Sync - TUI") {
		t.Fatalf("expected tui output header, got: %s", out.String())
	}
}

func TestAppCommandPaletteRunsCommand(t *testing.T) {
	t.Parallel()

	in := bytes.NewBufferString("/sync all\nq")
	out := &bytes.Buffer{}
	app := NewApp(fakeProvider{}, in, out)
	app.refreshInterval = time.Hour
	executor := &fakeExecutor{}
	app.SetExecutor(executor)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := app.Run(ctx); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	if len(executor.requests) == 0 {
		t.Fatal("expected command palette request to be executed")
	}
	if executor.requests[0].Type != ActionRunCommand {
		t.Fatalf("expected action type %q, got %q", ActionRunCommand, executor.requests[0].Type)
	}
	if executor.requests[0].Command != "sync all" {
		t.Fatalf("unexpected command %q", executor.requests[0].Command)
	}
}
