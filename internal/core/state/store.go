package state

import "time"

const CurrentSchemaVersion = 5

type Store interface {
	EnsureSchema() error
	PutRepoState(RepoState) error
	GetRepoState(repoPath string) (RepoState, bool, error)
	ListRepoStates(limit int) ([]RepoState, error)
	UpsertRunState(RunState) error
	CompleteRunState(runID, status, note string) error
	ListInFlightRunStates(limit int) ([]RunState, error)
	AppendEvent(Event) error
	ListEvents(limit int) ([]Event, error)
	ListEventsByTrace(traceID string, limit int) ([]Event, error)
	UpsertDiscoveredRepo(DiscoveredRepo) error
	GetDiscoveredRepo(sourceID, fullName string) (DiscoveredRepo, bool, error)
	ListDiscoveredRepos(sourceID string, limit int) ([]DiscoveredRepo, error)
	DeleteDiscoveredReposBySource(sourceID string) error
	RecordCloneOperation(*CloneOperation) error
	UpdateCloneOperation(id int64, status, errorMessage string, completedAt time.Time, retryCount int) error
	GetCloneOperation(id int64) (CloneOperation, bool, error)
	ListCloneOperations(limit int) ([]CloneOperation, error)
	ListCloneOperationsByTrace(traceID string, limit int) ([]CloneOperation, error)
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

type RunState struct {
	RunID       string
	TraceID     string
	RepoPath    string
	SourceID    string
	Status      string
	Note        string
	StartedAt   time.Time
	HeartbeatAt time.Time
	CompletedAt time.Time
}

type DiscoveredRepo struct {
	Provider      string
	SourceID      string
	FullName      string
	CloneURL      string
	DefaultBranch string
	IsArchived    bool
	SizeKB        int64
	DiscoveredAt  time.Time
}

type CloneOperation struct {
	ID           int64
	TraceID      string
	SourceID     string
	RepoFullName string
	TargetPath   string
	Status       string
	StartedAt    time.Time
	CompletedAt  time.Time
	ErrorMessage string
	RetryCount   int
}
