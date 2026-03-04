package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if dbPath == "" {
		return nil, errors.New("db path is required")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.EnsureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) EnsureSchema() error {
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("set journal mode: %w", err)
	}

	version, err := s.schemaVersion()
	if err != nil {
		return err
	}

	for version < CurrentSchemaVersion {
		next := version + 1
		if err := s.applyMigration(next); err != nil {
			return fmt.Errorf("apply migration %d: %w", next, err)
		}
		version = next
	}

	if version > CurrentSchemaVersion {
		return fmt.Errorf("state db schema version %d is newer than supported %d", version, CurrentSchemaVersion)
	}

	return nil
}

func (s *SQLiteStore) PutRepoState(repo RepoState) error {
	if repo.RepoPath == "" {
		return errors.New("repo path is required")
	}

	now := time.Now().UTC()
	if repo.UpdatedAt.IsZero() {
		repo.UpdatedAt = now
	}

	_, err := s.db.Exec(`
		INSERT INTO repo_state (repo_path, last_status, last_error, last_sync_at, updated_at, current_hash)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_path) DO UPDATE SET
			last_status=excluded.last_status,
			last_error=excluded.last_error,
			last_sync_at=excluded.last_sync_at,
			updated_at=excluded.updated_at,
			current_hash=excluded.current_hash
	`, repo.RepoPath, repo.LastStatus, repo.LastError, nullableTime(repo.LastSyncAt), repo.UpdatedAt.UTC().Format(time.RFC3339Nano), repo.CurrentHash)
	if err != nil {
		return fmt.Errorf("upsert repo state: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetRepoState(repoPath string) (RepoState, bool, error) {
	row := s.db.QueryRow(`
		SELECT repo_path, last_status, last_error, last_sync_at, updated_at, current_hash
		FROM repo_state
		WHERE repo_path = ?
	`, repoPath)

	var (
		repo         RepoState
		lastSyncRaw  sql.NullString
		updatedAtRaw string
	)

	err := row.Scan(&repo.RepoPath, &repo.LastStatus, &repo.LastError, &lastSyncRaw, &updatedAtRaw, &repo.CurrentHash)
	if errors.Is(err, sql.ErrNoRows) {
		return RepoState{}, false, nil
	}
	if err != nil {
		return RepoState{}, false, fmt.Errorf("query repo state: %w", err)
	}

	if lastSyncRaw.Valid {
		repo.LastSyncAt, err = time.Parse(time.RFC3339Nano, lastSyncRaw.String)
		if err != nil {
			return RepoState{}, false, fmt.Errorf("parse last_sync_at: %w", err)
		}
	}

	repo.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtRaw)
	if err != nil {
		return RepoState{}, false, fmt.Errorf("parse updated_at: %w", err)
	}

	return repo, true, nil
}

func (s *SQLiteStore) ListRepoStates(limit int) ([]RepoState, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT repo_path, last_status, last_error, last_sync_at, updated_at, current_hash
		FROM repo_state
		ORDER BY updated_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list repo states: %w", err)
	}
	defer rows.Close()

	out := make([]RepoState, 0, limit)
	for rows.Next() {
		var (
			repo         RepoState
			lastSyncRaw  sql.NullString
			updatedAtRaw string
		)

		if err := rows.Scan(&repo.RepoPath, &repo.LastStatus, &repo.LastError, &lastSyncRaw, &updatedAtRaw, &repo.CurrentHash); err != nil {
			return nil, fmt.Errorf("scan repo state row: %w", err)
		}

		if lastSyncRaw.Valid {
			repo.LastSyncAt, err = time.Parse(time.RFC3339Nano, lastSyncRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse repo list last_sync_at: %w", err)
			}
		}

		repo.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse repo list updated_at: %w", err)
		}

		out = append(out, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repo state rows: %w", err)
	}

	return out, nil
}

func (s *SQLiteStore) AppendEvent(event Event) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	result, err := s.db.Exec(`
		INSERT INTO events (trace_id, repo_path, level, reason_code, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, event.TraceID, event.RepoPath, event.Level, event.ReasonCode, event.Message, event.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	event.ID, _ = result.LastInsertId()
	return nil
}

func (s *SQLiteStore) UpsertRunState(run RunState) error {
	if run.RunID == "" {
		return errors.New("run id is required")
	}
	if run.RepoPath == "" {
		return errors.New("repo path is required")
	}

	now := time.Now().UTC()
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	if run.HeartbeatAt.IsZero() {
		run.HeartbeatAt = now
	}
	if run.Status == "" {
		run.Status = "running"
	}

	_, err := s.db.Exec(`
		INSERT INTO run_state (run_id, trace_id, repo_path, source_id, status, note, started_at, heartbeat_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			trace_id=excluded.trace_id,
			repo_path=excluded.repo_path,
			source_id=excluded.source_id,
			status=excluded.status,
			note=excluded.note,
			heartbeat_at=excluded.heartbeat_at,
			completed_at=excluded.completed_at
	`, run.RunID, run.TraceID, run.RepoPath, run.SourceID, run.Status, run.Note, run.StartedAt.Format(time.RFC3339Nano), run.HeartbeatAt.Format(time.RFC3339Nano), nullableTime(run.CompletedAt))
	if err != nil {
		return fmt.Errorf("upsert run state: %w", err)
	}

	return nil
}

func (s *SQLiteStore) CompleteRunState(runID, status, note string) error {
	if runID == "" {
		return errors.New("run id is required")
	}
	if status == "" {
		status = "completed"
	}

	_, err := s.db.Exec(`
		UPDATE run_state
		SET status = ?, note = ?, completed_at = ?, heartbeat_at = ?
		WHERE run_id = ?
	`, status, note, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), runID)
	if err != nil {
		return fmt.Errorf("complete run state: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListInFlightRunStates(limit int) ([]RunState, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.db.Query(`
		SELECT run_id, trace_id, repo_path, source_id, status, note, started_at, heartbeat_at, completed_at
		FROM run_state
		WHERE completed_at IS NULL
		ORDER BY started_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list in-flight run states: %w", err)
	}
	defer rows.Close()

	out := make([]RunState, 0, limit)
	for rows.Next() {
		var (
			run            RunState
			startedRaw     string
			heartbeatRaw   string
			completedAtRaw sql.NullString
		)

		if err := rows.Scan(&run.RunID, &run.TraceID, &run.RepoPath, &run.SourceID, &run.Status, &run.Note, &startedRaw, &heartbeatRaw, &completedAtRaw); err != nil {
			return nil, fmt.Errorf("scan run state: %w", err)
		}

		run.StartedAt, err = time.Parse(time.RFC3339Nano, startedRaw)
		if err != nil {
			return nil, fmt.Errorf("parse run started_at: %w", err)
		}
		run.HeartbeatAt, err = time.Parse(time.RFC3339Nano, heartbeatRaw)
		if err != nil {
			return nil, fmt.Errorf("parse run heartbeat_at: %w", err)
		}
		if completedAtRaw.Valid {
			run.CompletedAt, err = time.Parse(time.RFC3339Nano, completedAtRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse run completed_at: %w", err)
			}
		}

		out = append(out, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run states: %w", err)
	}

	return out, nil
}

func (s *SQLiteStore) ListEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, trace_id, repo_path, level, reason_code, message, created_at
		FROM events
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		var (
			event      Event
			createdRaw string
		)

		if err := rows.Scan(&event.ID, &event.TraceID, &event.RepoPath, &event.Level, &event.ReasonCode, &event.Message, &createdRaw); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		event.CreatedAt, err = time.Parse(time.RFC3339Nano, createdRaw)
		if err != nil {
			return nil, fmt.Errorf("parse event timestamp: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return events, nil
}

func (s *SQLiteStore) ListEventsByTrace(traceID string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, trace_id, repo_path, level, reason_code, message, created_at
		FROM events
		WHERE trace_id = ?
		ORDER BY id ASC
		LIMIT ?
	`, traceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query events by trace: %w", err)
	}
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		var (
			event      Event
			createdRaw string
		)

		if err := rows.Scan(&event.ID, &event.TraceID, &event.RepoPath, &event.Level, &event.ReasonCode, &event.Message, &createdRaw); err != nil {
			return nil, fmt.Errorf("scan event by trace: %w", err)
		}

		event.CreatedAt, err = time.Parse(time.RFC3339Nano, createdRaw)
		if err != nil {
			return nil, fmt.Errorf("parse event-by-trace timestamp: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events by trace: %w", err)
	}

	return events, nil
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) schemaVersion() (int, error) {
	row := s.db.QueryRow(`PRAGMA user_version;`)
	var version int
	if err := row.Scan(&version); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return version, nil
}

func (s *SQLiteStore) applyMigration(version int) error {
	switch version {
	case 1:
		if _, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS repo_state (
				repo_path TEXT PRIMARY KEY,
				last_status TEXT NOT NULL,
				last_error TEXT NOT NULL,
				last_sync_at TEXT,
				updated_at TEXT NOT NULL,
				current_hash TEXT NOT NULL
			);

			CREATE TABLE IF NOT EXISTS events (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				trace_id TEXT NOT NULL,
				repo_path TEXT NOT NULL,
				level TEXT NOT NULL,
				reason_code TEXT NOT NULL,
				message TEXT NOT NULL,
				created_at TEXT NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_events_repo_path ON events(repo_path);
			CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
		`); err != nil {
			return err
		}
	case 2:
		if _, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS run_state (
				run_id TEXT PRIMARY KEY,
				trace_id TEXT NOT NULL,
				repo_path TEXT NOT NULL,
				source_id TEXT NOT NULL,
				status TEXT NOT NULL,
				note TEXT NOT NULL,
				started_at TEXT NOT NULL,
				heartbeat_at TEXT NOT NULL,
				completed_at TEXT
			);

			CREATE INDEX IF NOT EXISTS idx_run_state_repo_path ON run_state(repo_path);
			CREATE INDEX IF NOT EXISTS idx_run_state_status ON run_state(status);
		`); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown migration version %d", version)
	}

	if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d;`, version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	return nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
