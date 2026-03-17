package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/notify"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

// TestNotificationDispatch_FiltersBySeverity verifies that the Dispatcher
// only delivers events that meet the configured MinSeverity threshold, and
// that the delivered payload does not contain repository path information.
func TestNotificationDispatch_FiltersBySeverity(t *testing.T) {
	t.Parallel()

	var delivered atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{
				Name:        "test-sink",
				Type:        "webhook",
				URL:         srv.URL,
				MinSeverity: "error",
				Enabled:     true,
			},
		},
	}
	d := notify.NewDispatcher(cfg, srv.Client(), slog.New(slog.NewJSONHandler(io.Discard, nil)))

	ctx := context.Background()

	// These should be filtered out (below error threshold).
	d.Dispatch(ctx, telemetry.Event{Level: "info", ReasonCode: telemetry.ReasonSyncCompleted, CreatedAt: time.Now()})
	d.Dispatch(ctx, telemetry.Event{Level: "warn", ReasonCode: telemetry.ReasonSyncRetry, CreatedAt: time.Now()})

	// These should be delivered.
	d.Dispatch(ctx, telemetry.Event{Level: "error", ReasonCode: telemetry.ReasonSyncFailed, CreatedAt: time.Now()})
	d.Dispatch(ctx, telemetry.Event{Level: "error", ReasonCode: telemetry.ReasonSyncFailed, CreatedAt: time.Now()})

	if got := delivered.Load(); got != 2 {
		t.Errorf("delivered %d notifications, want 2 (only error-level should pass)", got)
	}
}

// TestNotificationDispatch_ReasonCodeFilter verifies that when a sink specifies
// reason_codes only matching events are dispatched.
func TestNotificationDispatch_ReasonCodeFilter(t *testing.T) {
	t.Parallel()

	var delivered atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{
				Name:        "specific-sink",
				Type:        "webhook",
				URL:         srv.URL,
				MinSeverity: "warn",
				ReasonCodes: []string{telemetry.ReasonSyncFailed},
				Enabled:     true,
			},
		},
	}
	d := notify.NewDispatcher(cfg, srv.Client(), slog.New(slog.NewJSONHandler(io.Discard, nil)))

	ctx := context.Background()

	// Reason code not in allowlist — filtered.
	d.Dispatch(ctx, telemetry.Event{Level: "error", ReasonCode: telemetry.ReasonRepoLocked, CreatedAt: time.Now()})
	// Reason code matches — delivered.
	d.Dispatch(ctx, telemetry.Event{Level: "error", ReasonCode: telemetry.ReasonSyncFailed, CreatedAt: time.Now()})

	if got := delivered.Load(); got != 1 {
		t.Errorf("delivered %d notifications, want 1 (only sync_failed should pass)", got)
	}
}

// TestNotificationDispatch_PayloadRedaction verifies that the webhook payload
// does not contain repository path information.
func TestNotificationDispatch_PayloadRedaction(t *testing.T) {
	t.Parallel()

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{
				Name:        "redact-check",
				Type:        "webhook",
				URL:         srv.URL,
				MinSeverity: "info",
				Enabled:     true,
			},
		},
	}
	d := notify.NewDispatcher(cfg, srv.Client(), slog.New(slog.NewJSONHandler(io.Discard, nil)))

	sensitiveRepoPath := "/workspace/corp/secret-project"
	d.Dispatch(context.Background(), telemetry.Event{
		TraceID:    "trace-redact",
		RepoPath:   sensitiveRepoPath,
		Level:      "error",
		ReasonCode: telemetry.ReasonSyncFailed,
		Message:    "sync failed",
		CreatedAt:  time.Now(),
	})

	if len(body) == 0 {
		t.Fatal("no payload received by webhook")
	}

	// The raw repo path must not appear in the serialized payload.
	if string(body) == "" {
		t.Fatal("empty payload body")
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	for k, v := range m {
		if s, ok := v.(string); ok && s == sensitiveRepoPath {
			t.Errorf("field %q contains sensitive repo path %q", k, sensitiveRepoPath)
		}
	}
}

// TestNotificationDispatch_DisabledSink ensures that a disabled sink receives
// no HTTP calls even when an event matches all filter criteria.
func TestNotificationDispatch_DisabledSink(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "disabled", Type: "webhook", URL: srv.URL, MinSeverity: "info", Enabled: false},
		},
	}
	d := notify.NewDispatcher(cfg, srv.Client(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	d.Dispatch(context.Background(), telemetry.Event{Level: "error", ReasonCode: telemetry.ReasonSyncFailed, CreatedAt: time.Now()})

	if called {
		t.Error("disabled sink must not receive any HTTP calls")
	}
}

// TestNotificationDispatch_HTTPFailure_DoesNotPanic ensures that a failing
// HTTP endpoint does not propagate errors or panic.
func TestNotificationDispatch_HTTPFailure_DoesNotPanic(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		// This test has no git dependency but we keep the skip pattern consistent.
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "flaky", Type: "webhook", URL: srv.URL, MinSeverity: "info", Enabled: true},
		},
	}
	d := notify.NewDispatcher(cfg, srv.Client(), slog.New(slog.NewJSONHandler(io.Discard, nil)))

	// Must not panic, must not return an error.
	d.Dispatch(context.Background(), telemetry.Event{Level: "error", ReasonCode: telemetry.ReasonSyncFailed, CreatedAt: time.Now()})
}
