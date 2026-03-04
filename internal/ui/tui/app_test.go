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
