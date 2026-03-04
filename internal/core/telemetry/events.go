package telemetry

import "time"

type Event struct {
	TraceID    string
	RepoPath   string
	Level      string
	ReasonCode string
	Message    string
	CreatedAt  time.Time
}

const (
	ReasonSyncCompleted = "sync_completed"
	ReasonRepoLocked    = "repo_locked"
	ReasonSyncRetry     = "sync_retry"
	ReasonSyncFailed    = "sync_failed"
)
