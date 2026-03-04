package state

import "time"

const CurrentSchemaVersion = 1

type Store interface {
	EnsureSchema() error
	PutRepoState(RepoState) error
	GetRepoState(repoPath string) (RepoState, bool, error)
	ListRepoStates(limit int) ([]RepoState, error)
	AppendEvent(Event) error
	ListEvents(limit int) ([]Event, error)
	ListEventsByTrace(traceID string, limit int) ([]Event, error)
	Close() error
}

type RepoState struct {
	RepoPath    string
	LastStatus  string
	LastError   string
	LastSyncAt  time.Time
	UpdatedAt   time.Time
	CurrentHash string
}

type Event struct {
	ID         int64
	TraceID    string
	RepoPath   string
	Level      string
	ReasonCode string
	Message    string
	CreatedAt  time.Time
}
