package state

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db     *sql.DB
	dbPath string
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

	// Limit to a single writer connection to avoid SQLITE_BUSY contention.
	// Readers share the same connection pool; WAL mode allows concurrent reads.
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db, dbPath: dbPath}
	if err := store.EnsureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) BackupTo(backupPath string, overwrite bool) error {
	if strings.TrimSpace(backupPath) == "" {
		return errors.New("backup path is required")
	}
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	if _, err := os.Stat(backupPath); err == nil {
		if !overwrite {
			return fmt.Errorf("backup file already exists: %s", backupPath)
		}
		if removeErr := os.Remove(backupPath); removeErr != nil {
			return fmt.Errorf("remove existing backup file: %w", removeErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check backup path: %w", err)
	}

	escapedPath := strings.ReplaceAll(backupPath, "'", "''")
	query := fmt.Sprintf("VACUUM INTO '%s';", escapedPath)
	if _, err := s.db.Exec(query); err == nil {
		return nil
	}

	if err := copyFile(s.dbPath, backupPath); err != nil {
		return fmt.Errorf("copy sqlite file backup fallback: %w", err)
	}
	return nil
}

func (s *SQLiteStore) IntegrityCheck() error {
	rows, err := s.db.Query(`PRAGMA integrity_check;`)
	if err != nil {
		return fmt.Errorf("run integrity check: %w", err)
	}
	defer rows.Close()

	failures := make([]string, 0)
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("scan integrity row: %w", err)
		}
		if strings.EqualFold(strings.TrimSpace(result), "ok") {
			continue
		}
		failures = append(failures, result)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate integrity rows: %w", err)
	}
	if len(failures) > 0 {
		return fmt.Errorf("integrity check failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func RestoreSQLiteDB(dbPath, backupPath string) error {
	if strings.TrimSpace(dbPath) == "" {
		return errors.New("db path is required")
	}
	if strings.TrimSpace(backupPath) == "" {
		return errors.New("backup path is required")
	}
	if dbPath == backupPath {
		return errors.New("backup path must differ from db path")
	}

	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file is unavailable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	tmp := dbPath + ".restore.tmp"
	if err := copyFile(backupPath, tmp); err != nil {
		return fmt.Errorf("copy backup to temp file: %w", err)
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		return fmt.Errorf("replace db with restored backup: %w", err)
	}

	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	return nil
}

func copyFile(src, dst string) error {
	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()

	to, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer to.Close()

	if _, err := io.Copy(to, from); err != nil {
		return err
	}
	return to.Sync()
}

func (s *SQLiteStore) EnsureSchema() error {
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("set journal mode: %w", err)
	}
	// NORMAL durability is safe with WAL and dramatically reduces fsync overhead
	// on the hot-path event-append workload.
	if _, err := s.db.Exec(`PRAGMA synchronous=NORMAL;`); err != nil {
		return fmt.Errorf("set synchronous mode: %w", err)
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

func (s *SQLiteStore) UpsertDiscoveredRepo(repo DiscoveredRepo) error {
	if repo.SourceID == "" {
		return errors.New("source id is required")
	}
	if repo.FullName == "" {
		return errors.New("full name is required")
	}

	now := time.Now().UTC()
	if repo.DiscoveredAt.IsZero() {
		repo.DiscoveredAt = now
	}

	_, err := s.db.Exec(`
		INSERT INTO discovered_repos (provider, source_id, full_name, clone_url, default_branch, is_archived, size_kb, discovered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, full_name) DO UPDATE SET
			provider=excluded.provider,
			clone_url=excluded.clone_url,
			default_branch=excluded.default_branch,
			is_archived=excluded.is_archived,
			size_kb=excluded.size_kb,
			discovered_at=excluded.discovered_at
	`, repo.Provider, repo.SourceID, repo.FullName, repo.CloneURL, repo.DefaultBranch, boolToInt(repo.IsArchived), repo.SizeKB, repo.DiscoveredAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert discovered repo: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetDiscoveredRepo(sourceID, fullName string) (DiscoveredRepo, bool, error) {
	row := s.db.QueryRow(`
		SELECT provider, source_id, full_name, clone_url, default_branch, is_archived, size_kb, discovered_at
		FROM discovered_repos
		WHERE source_id = ? AND full_name = ?
	`, sourceID, fullName)

	var (
		repo          DiscoveredRepo
		isArchivedInt int
		discoveredRaw string
	)

	err := row.Scan(&repo.Provider, &repo.SourceID, &repo.FullName, &repo.CloneURL, &repo.DefaultBranch, &isArchivedInt, &repo.SizeKB, &discoveredRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return DiscoveredRepo{}, false, nil
	}
	if err != nil {
		return DiscoveredRepo{}, false, fmt.Errorf("query discovered repo: %w", err)
	}

	repo.IsArchived = isArchivedInt != 0
	repo.DiscoveredAt, err = time.Parse(time.RFC3339Nano, discoveredRaw)
	if err != nil {
		return DiscoveredRepo{}, false, fmt.Errorf("parse discovered_at: %w", err)
	}

	return repo, true, nil
}

func (s *SQLiteStore) ListDiscoveredRepos(sourceID string, limit int) ([]DiscoveredRepo, error) {
	if limit <= 0 {
		limit = 500
	}

	var rows *sql.Rows
	var err error

	if sourceID == "" {
		rows, err = s.db.Query(`
			SELECT provider, source_id, full_name, clone_url, default_branch, is_archived, size_kb, discovered_at
			FROM discovered_repos
			ORDER BY discovered_at DESC
			LIMIT ?
		`, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT provider, source_id, full_name, clone_url, default_branch, is_archived, size_kb, discovered_at
			FROM discovered_repos
			WHERE source_id = ?
			ORDER BY discovered_at DESC
			LIMIT ?
		`, sourceID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("list discovered repos: %w", err)
	}
	defer rows.Close()

	out := make([]DiscoveredRepo, 0, limit)
	for rows.Next() {
		var (
			repo          DiscoveredRepo
			isArchivedInt int
			discoveredRaw string
		)

		if err := rows.Scan(&repo.Provider, &repo.SourceID, &repo.FullName, &repo.CloneURL, &repo.DefaultBranch, &isArchivedInt, &repo.SizeKB, &discoveredRaw); err != nil {
			return nil, fmt.Errorf("scan discovered repo row: %w", err)
		}

		repo.IsArchived = isArchivedInt != 0
		repo.DiscoveredAt, err = time.Parse(time.RFC3339Nano, discoveredRaw)
		if err != nil {
			return nil, fmt.Errorf("parse list discovered_at: %w", err)
		}

		out = append(out, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate discovered repo rows: %w", err)
	}

	return out, nil
}

func (s *SQLiteStore) DeleteDiscoveredReposBySource(sourceID string) error {
	if sourceID == "" {
		return errors.New("source id is required")
	}

	_, err := s.db.Exec(`DELETE FROM discovered_repos WHERE source_id = ?`, sourceID)
	if err != nil {
		return fmt.Errorf("delete discovered repos by source: %w", err)
	}

	return nil
}

func (s *SQLiteStore) RecordCloneOperation(op *CloneOperation) error {
	if op.SourceID == "" {
		return errors.New("source id is required")
	}
	if op.RepoFullName == "" {
		return errors.New("repo full name is required")
	}
	if op.TargetPath == "" {
		return errors.New("target path is required")
	}

	now := time.Now().UTC()
	if op.StartedAt.IsZero() {
		op.StartedAt = now
	}

	result, err := s.db.Exec(`
		INSERT INTO clone_operations (trace_id, source_id, repo_full_name, target_path, status, started_at, completed_at, error_message, retry_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.TraceID, op.SourceID, op.RepoFullName, op.TargetPath, op.Status, op.StartedAt.Format(time.RFC3339Nano), nullableTime(op.CompletedAt), op.ErrorMessage, op.RetryCount)
	if err != nil {
		return fmt.Errorf("insert clone operation: %w", err)
	}

	op.ID, _ = result.LastInsertId()
	return nil
}

func (s *SQLiteStore) UpdateCloneOperation(id int64, status, errorMessage string, completedAt time.Time, retryCount int) error {
	if id <= 0 {
		return errors.New("operation id is required")
	}

	_, err := s.db.Exec(`
		UPDATE clone_operations
		SET status = ?, error_message = ?, completed_at = ?, retry_count = ?
		WHERE id = ?
	`, status, errorMessage, nullableTime(completedAt), retryCount, id)
	if err != nil {
		return fmt.Errorf("update clone operation: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetCloneOperation(id int64) (CloneOperation, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, trace_id, source_id, repo_full_name, target_path, status, started_at, completed_at, error_message, retry_count
		FROM clone_operations
		WHERE id = ?
	`, id)

	var (
		op           CloneOperation
		startedRaw   sql.NullString
		completedRaw sql.NullString
	)

	err := row.Scan(&op.ID, &op.TraceID, &op.SourceID, &op.RepoFullName, &op.TargetPath, &op.Status, &startedRaw, &completedRaw, &op.ErrorMessage, &op.RetryCount)
	if errors.Is(err, sql.ErrNoRows) {
		return CloneOperation{}, false, nil
	}
	if err != nil {
		return CloneOperation{}, false, fmt.Errorf("query clone operation: %w", err)
	}

	if startedRaw.Valid {
		op.StartedAt, err = time.Parse(time.RFC3339Nano, startedRaw.String)
		if err != nil {
			return CloneOperation{}, false, fmt.Errorf("parse started_at: %w", err)
		}
	}

	if completedRaw.Valid {
		op.CompletedAt, err = time.Parse(time.RFC3339Nano, completedRaw.String)
		if err != nil {
			return CloneOperation{}, false, fmt.Errorf("parse completed_at: %w", err)
		}
	}

	return op, true, nil
}

func (s *SQLiteStore) ListCloneOperations(limit int) ([]CloneOperation, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, trace_id, source_id, repo_full_name, target_path, status, started_at, completed_at, error_message, retry_count
		FROM clone_operations
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list clone operations: %w", err)
	}
	defer rows.Close()

	ops := make([]CloneOperation, 0, limit)
	for rows.Next() {
		var (
			op           CloneOperation
			startedRaw   sql.NullString
			completedRaw sql.NullString
		)

		if err := rows.Scan(&op.ID, &op.TraceID, &op.SourceID, &op.RepoFullName, &op.TargetPath, &op.Status, &startedRaw, &completedRaw, &op.ErrorMessage, &op.RetryCount); err != nil {
			return nil, fmt.Errorf("scan clone operation row: %w", err)
		}

		if startedRaw.Valid {
			op.StartedAt, err = time.Parse(time.RFC3339Nano, startedRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse list started_at: %w", err)
			}
		}

		if completedRaw.Valid {
			op.CompletedAt, err = time.Parse(time.RFC3339Nano, completedRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse list completed_at: %w", err)
			}
		}

		ops = append(ops, op)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clone operation rows: %w", err)
	}

	return ops, nil
}

func (s *SQLiteStore) ListCloneOperationsByTrace(traceID string, limit int) ([]CloneOperation, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, trace_id, source_id, repo_full_name, target_path, status, started_at, completed_at, error_message, retry_count
		FROM clone_operations
		WHERE trace_id = ?
		ORDER BY id ASC
		LIMIT ?
	`, traceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list clone operations by trace: %w", err)
	}
	defer rows.Close()

	ops := make([]CloneOperation, 0, limit)
	for rows.Next() {
		var (
			op           CloneOperation
			startedRaw   sql.NullString
			completedRaw sql.NullString
		)

		if err := rows.Scan(&op.ID, &op.TraceID, &op.SourceID, &op.RepoFullName, &op.TargetPath, &op.Status, &startedRaw, &completedRaw, &op.ErrorMessage, &op.RetryCount); err != nil {
			return nil, fmt.Errorf("scan clone operation by trace row: %w", err)
		}

		if startedRaw.Valid {
			op.StartedAt, err = time.Parse(time.RFC3339Nano, startedRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse by-trace started_at: %w", err)
			}
		}

		if completedRaw.Valid {
			op.CompletedAt, err = time.Parse(time.RFC3339Nano, completedRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse by-trace completed_at: %w", err)
			}
		}

		ops = append(ops, op)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clone operations by trace: %w", err)
	}

	return ops, nil
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
	case 3:
		if _, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS discovered_repos (
				provider TEXT NOT NULL,
				source_id TEXT NOT NULL,
				full_name TEXT NOT NULL,
				clone_url TEXT NOT NULL,
				default_branch TEXT NOT NULL,
				is_archived INTEGER NOT NULL DEFAULT 0,
				size_kb INTEGER NOT NULL DEFAULT 0,
				discovered_at TEXT NOT NULL,
				PRIMARY KEY (source_id, full_name)
			);

			CREATE INDEX IF NOT EXISTS idx_discovered_repos_provider ON discovered_repos(provider);
			CREATE INDEX IF NOT EXISTS idx_discovered_repos_source_id ON discovered_repos(source_id);
			CREATE INDEX IF NOT EXISTS idx_discovered_repos_is_archived ON discovered_repos(is_archived);

			CREATE TABLE IF NOT EXISTS clone_operations (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				trace_id TEXT NOT NULL,
				source_id TEXT NOT NULL,
				repo_full_name TEXT NOT NULL,
				target_path TEXT NOT NULL,
				status TEXT NOT NULL,
				started_at TEXT,
				completed_at TEXT,
				error_message TEXT NOT NULL DEFAULT '',
				retry_count INTEGER NOT NULL DEFAULT 0
			);

			CREATE INDEX IF NOT EXISTS idx_clone_operations_trace_id ON clone_operations(trace_id);
			CREATE INDEX IF NOT EXISTS idx_clone_operations_source_id ON clone_operations(source_id);
			CREATE INDEX IF NOT EXISTS idx_clone_operations_status ON clone_operations(status);
			CREATE INDEX IF NOT EXISTS idx_clone_operations_repo_full_name ON clone_operations(repo_full_name);
		`); err != nil {
			return err
		}
	case 4:
		// Data migration: strip embedded PAT tokens from clone_url values in
		// discovered_repos.  URLs were stored as https://<token>@<host>/…;
		// we rewrite them to the clean https://<host>/… form.
		if err := s.stripPATsFromDiscoveredRepos(); err != nil {
			return err
		}
	case 5:
		// Performance indexes: add composite and covering indexes that were
		// missing from the initial schema and are hit on every scheduler cycle.
		if _, err := s.db.Exec(`
			-- Fast trace-id lookup used by ListEventsByTrace (previously full-scan).
			CREATE INDEX IF NOT EXISTS idx_events_trace_id ON events(trace_id);

			-- Fast in-flight run-state queries: WHERE completed_at IS NULL.
			CREATE INDEX IF NOT EXISTS idx_run_state_completed_at ON run_state(completed_at);

			-- Covering index for repo_state ordered listing (most-recent-first).
			CREATE INDEX IF NOT EXISTS idx_repo_state_updated_at ON repo_state(updated_at DESC);
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

// stripPATsFromDiscoveredRepos reads every row from discovered_repos and
// rewrites any clone_url that contains an embedded credential.
// This runs as part of schema migration version 4.
func (s *SQLiteStore) stripPATsFromDiscoveredRepos() error {
	rows, err := s.db.Query(`SELECT source_id, full_name, clone_url FROM discovered_repos`)
	if err != nil {
		return fmt.Errorf("read discovered_repos: %w", err)
	}
	defer rows.Close()

	type repoRow struct {
		sourceID string
		fullName string
		cloneURL string
	}
	var toFix []repoRow

	for rows.Next() {
		var r repoRow
		if err := rows.Scan(&r.sourceID, &r.fullName, &r.cloneURL); err != nil {
			return fmt.Errorf("scan discovered_repos row: %w", err)
		}
		if cleaned, changed := removeEmbeddedCredentialFromURL(r.cloneURL); changed {
			r.cloneURL = cleaned
			toFix = append(toFix, r)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate discovered_repos rows: %w", err)
	}
	rows.Close()

	for _, r := range toFix {
		if _, err := s.db.Exec(
			`UPDATE discovered_repos SET clone_url = ? WHERE source_id = ? AND full_name = ?`,
			r.cloneURL, r.sourceID, r.fullName,
		); err != nil {
			return fmt.Errorf("update clone_url for %s/%s: %w", r.sourceID, r.fullName, err)
		}
	}

	return nil
}

// removeEmbeddedCredentialFromURL strips the userinfo (PAT token) from an
// https:// URL.
//
//	"https://ghp_TOKEN@github.com/owner/repo.git" → "https://github.com/owner/repo.git", true
//	"https://github.com/owner/repo.git"           → unchanged, false
func removeEmbeddedCredentialFromURL(rawURL string) (string, bool) {
	const prefix = "https://"
	if !strings.HasPrefix(rawURL, prefix) {
		return rawURL, false
	}

	rest := rawURL[len(prefix):]
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		return rawURL, false
	}

	// Guard: the host part after "@" must contain a "." (e.g. "github.com")
	// so we don't accidentally strip a genuine "org@" prefix that is part
	// of the hostname rather than a credential.
	hostPart := rest[atIdx+1:]
	hostOnly := strings.SplitN(hostPart, "/", 2)[0]
	if !strings.Contains(hostOnly, ".") {
		return rawURL, false
	}

	return prefix + hostPart, true
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
