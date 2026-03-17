// Package notify delivers redacted telemetry event payloads to configured
// outbound notification sinks (webhook, Slack, Microsoft Teams).
//
// Safety contract:
//   - Payloads never contain raw secret values, PAT tokens, or full file paths.
//   - Each event is filtered by MinSeverity and optional ReasonCode allowlist
//     before dispatch.  Events that do not pass the filter are silently dropped.
//   - Network errors are logged but never propagate back to the caller: a failed
//     notification must never interrupt a sync cycle.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

// severityRank maps level strings to a numeric rank so that MinSeverity
// comparisons are straightforward.
var severityRank = map[string]int{
	"info":  0,
	"warn":  1,
	"error": 2,
}

// Dispatcher fans out a single telemetry event to all enabled sinks whose
// filter criteria are satisfied.
type Dispatcher struct {
	sinks  []config.NotificationSinkConfig
	client *http.Client
	logger *slog.Logger
}

// NewDispatcher creates a Dispatcher from the loaded config.  Pass nil for
// client to use http.DefaultClient.
func NewDispatcher(cfg config.NotificationsConfig, client *http.Client, logger *slog.Logger) *Dispatcher {
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{sinks: cfg.Sinks, client: client, logger: logger}
}

// Dispatch sends event to all matching sinks.  It is safe to call from
// multiple goroutines.  All delivery errors are logged, not returned.
func (d *Dispatcher) Dispatch(ctx context.Context, event telemetry.Event) {
	for _, sink := range d.sinks {
		if !sink.Enabled {
			continue
		}
		if !passesFilter(event, sink) {
			continue
		}
		if err := d.deliver(ctx, sink, event); err != nil {
			d.logger.Warn("notification delivery failed",
				"sink", sink.Name,
				"sink_type", sink.Type,
				"reason_code", event.ReasonCode,
				"error", err,
			)
		}
	}
}

// passesFilter returns true when event satisfies the sink's severity and
// reason-code constraints.
func passesFilter(event telemetry.Event, sink config.NotificationSinkConfig) bool {
	// Severity gate
	minLevel := strings.ToLower(strings.TrimSpace(sink.MinSeverity))
	if minLevel == "" {
		minLevel = "error"
	}
	minRank, ok := severityRank[minLevel]
	if !ok {
		minRank = severityRank["error"]
	}
	eventRank, ok := severityRank[strings.ToLower(strings.TrimSpace(event.Level))]
	if !ok {
		eventRank = 0
	}
	if eventRank < minRank {
		return false
	}

	// Reason-code allowlist gate (empty list = all reasons pass)
	if len(sink.ReasonCodes) > 0 {
		matched := false
		for _, rc := range sink.ReasonCodes {
			if strings.EqualFold(rc, event.ReasonCode) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// redactedPayload is the JSON body sent to each sink.  Field names follow
// common webhook conventions so recipients can parse them without custom logic.
type redactedPayload struct {
	Service    string `json:"service"`
	TraceID    string `json:"trace_id"`
	Level      string `json:"level"`
	ReasonCode string `json:"reason_code"`
	Message    string `json:"message"`
	// RepoPath is omitted deliberately to avoid leaking workspace layout.
	Timestamp string `json:"timestamp"`
}

func buildPayload(event telemetry.Event) redactedPayload {
	return redactedPayload{
		Service:    "git-project-sync",
		TraceID:    event.TraceID,
		Level:      event.Level,
		ReasonCode: event.ReasonCode,
		Message:    event.Message,
		Timestamp:  event.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// deliver sends the event to a single sink.
func (d *Dispatcher) deliver(ctx context.Context, sink config.NotificationSinkConfig, event telemetry.Event) error {
	payload := buildPayload(event)

	var body []byte
	var err error
	sinkType := strings.ToLower(strings.TrimSpace(sink.Type))

	switch sinkType {
	case "slack":
		body, err = slackBody(payload)
	case "teams":
		body, err = teamsBody(payload)
	default: // "webhook"
		body, err = json.Marshal(payload)
	}
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sink.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// slackBody wraps payload in a Slack incoming-webhook envelope.
func slackBody(p redactedPayload) ([]byte, error) {
	text := fmt.Sprintf("[%s] *%s* — %s\ntrace: %s  timestamp: %s",
		strings.ToUpper(p.Level), p.ReasonCode, p.Message, p.TraceID, p.Timestamp)
	return json.Marshal(map[string]string{"text": text})
}

// teamsBody wraps payload in a minimal Teams webhook card.
func teamsBody(p redactedPayload) ([]byte, error) {
	card := map[string]any{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"themeColor": levelColor(p.Level),
		"summary":    p.ReasonCode,
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("[%s] %s", strings.ToUpper(p.Level), p.ReasonCode),
				"activitySubtitle": p.Message,
				"facts": []map[string]string{
					{"name": "Trace ID", "value": p.TraceID},
					{"name": "Timestamp", "value": p.Timestamp},
					{"name": "Service", "value": p.Service},
				},
			},
		},
	}
	return json.Marshal(card)
}

func levelColor(level string) string {
	switch strings.ToLower(level) {
	case "error":
		return "FF0000"
	case "warn", "warning":
		return "FFA500"
	default:
		return "0078D7"
	}
}
