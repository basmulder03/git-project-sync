package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

func makeEvent(level, reasonCode string) telemetry.Event {
	return telemetry.Event{
		TraceID:    "trace-001",
		RepoPath:   "/workspace/owner/repo",
		Level:      level,
		ReasonCode: reasonCode,
		Message:    "test message",
		CreatedAt:  time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
	}
}

func TestPassesFilter_SeverityGate(t *testing.T) {
	t.Parallel()

	sink := config.NotificationSinkConfig{MinSeverity: "error", Enabled: true}
	if passesFilter(makeEvent("info", "sync_completed"), sink) {
		t.Error("info should be filtered out when min_severity=error")
	}
	if passesFilter(makeEvent("warn", "sync_retry"), sink) {
		t.Error("warn should be filtered out when min_severity=error")
	}
	if !passesFilter(makeEvent("error", "sync_failed"), sink) {
		t.Error("error should pass when min_severity=error")
	}
}

func TestPassesFilter_DefaultSeverityIsError(t *testing.T) {
	t.Parallel()

	sink := config.NotificationSinkConfig{MinSeverity: "", Enabled: true}
	if passesFilter(makeEvent("warn", "sync_retry"), sink) {
		t.Error("warn should be filtered with default (error) severity")
	}
	if !passesFilter(makeEvent("error", "sync_failed"), sink) {
		t.Error("error should pass with default (error) severity")
	}
}

func TestPassesFilter_ReasonCodeAllowlist(t *testing.T) {
	t.Parallel()

	sink := config.NotificationSinkConfig{
		MinSeverity: "warn",
		ReasonCodes: []string{"sync_failed"},
		Enabled:     true,
	}
	if passesFilter(makeEvent("error", "sync_retry"), sink) {
		t.Error("reason_code not in allowlist should be filtered")
	}
	if !passesFilter(makeEvent("error", "sync_failed"), sink) {
		t.Error("reason_code in allowlist should pass")
	}
}

func TestPassesFilter_EmptyReasonCodesAllowsAll(t *testing.T) {
	t.Parallel()

	sink := config.NotificationSinkConfig{MinSeverity: "info", ReasonCodes: nil, Enabled: true}
	if !passesFilter(makeEvent("info", "anything"), sink) {
		t.Error("empty reason_codes should allow all reason codes")
	}
}

func TestDispatch_DeliverWebhook(t *testing.T) {
	t.Parallel()

	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "test-hook", Type: "webhook", URL: srv.URL, MinSeverity: "error", Enabled: true},
		},
	}
	d := NewDispatcher(cfg, srv.Client(), nil)
	d.Dispatch(context.Background(), makeEvent("error", "sync_failed"))

	var payload redactedPayload
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("invalid payload JSON: %v", err)
	}
	if payload.ReasonCode != "sync_failed" {
		t.Errorf("reason_code = %q, want sync_failed", payload.ReasonCode)
	}
	if payload.TraceID != "trace-001" {
		t.Errorf("trace_id = %q, want trace-001", payload.TraceID)
	}
	// RepoPath must NOT appear in the payload to avoid leaking workspace paths.
	raw := string(received)
	if strings.Contains(raw, "/workspace/") {
		t.Error("payload must not contain repo path")
	}
}

func TestDispatch_DeliverSlack(t *testing.T) {
	t.Parallel()

	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "slack", Type: "slack", URL: srv.URL, MinSeverity: "warn", Enabled: true},
		},
	}
	d := NewDispatcher(cfg, srv.Client(), nil)
	d.Dispatch(context.Background(), makeEvent("error", "sync_failed"))

	var m map[string]string
	if err := json.Unmarshal(received, &m); err != nil {
		t.Fatalf("invalid Slack payload JSON: %v", err)
	}
	if _, ok := m["text"]; !ok {
		t.Error("Slack payload must have 'text' field")
	}
}

func TestDispatch_DeliverTeams(t *testing.T) {
	t.Parallel()

	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "teams", Type: "teams", URL: srv.URL, MinSeverity: "warn", Enabled: true},
		},
	}
	d := NewDispatcher(cfg, srv.Client(), nil)
	d.Dispatch(context.Background(), makeEvent("error", "sync_failed"))

	var m map[string]any
	if err := json.Unmarshal(received, &m); err != nil {
		t.Fatalf("invalid Teams payload JSON: %v", err)
	}
	if m["@type"] != "MessageCard" {
		t.Errorf("@type = %v, want MessageCard", m["@type"])
	}
}

func TestDispatch_DisabledSinkSkipped(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "off", Type: "webhook", URL: srv.URL, MinSeverity: "info", Enabled: false},
		},
	}
	d := NewDispatcher(cfg, srv.Client(), nil)
	d.Dispatch(context.Background(), makeEvent("error", "sync_failed"))

	if called {
		t.Error("disabled sink must not be called")
	}
}

func TestDispatch_HTTPError_Logged_NotPropagated(t *testing.T) {
	t.Parallel()

	// Server that returns 500 – Dispatch must not panic or return an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := config.NotificationsConfig{
		Sinks: []config.NotificationSinkConfig{
			{Name: "bad", Type: "webhook", URL: srv.URL, MinSeverity: "info", Enabled: true},
		},
	}
	d := NewDispatcher(cfg, srv.Client(), nil)
	// Must not panic
	d.Dispatch(context.Background(), makeEvent("error", "sync_failed"))
}

func TestBuildPayload_RepoPathAbsent(t *testing.T) {
	t.Parallel()

	event := makeEvent("error", "sync_failed")
	p := buildPayload(event)
	data, _ := json.Marshal(p)
	if strings.Contains(string(data), event.RepoPath) {
		t.Error("redactedPayload must not contain RepoPath")
	}
	if p.Service != "git-project-sync" {
		t.Errorf("service = %q, want git-project-sync", p.Service)
	}
}

func TestLevelColor(t *testing.T) {
	t.Parallel()

	if levelColor("error") != "FF0000" {
		t.Error("error should map to red")
	}
	if levelColor("warn") != "FFA500" {
		t.Error("warn should map to orange")
	}
	if levelColor("info") != "0078D7" {
		t.Error("info should map to blue")
	}
}
