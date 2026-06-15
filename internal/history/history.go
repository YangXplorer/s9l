// Package history persists query history and saved queries in a SQLite database
// at ~/.config/s9l/history.db (honoring $XDG_CONFIG_HOME). It is s9l's own
// storage — distinct from the user databases reached via the driver package —
// so it uses database/sql with the pure-Go modernc.org/sqlite driver directly.
// See docs/PLAN.md and docs/TASKS.md F1–F3 for the schema.
package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a handle to the history database.
type Store struct {
	db *sql.DB
}

// HistoryEntry is one executed query.
type HistoryEntry struct {
	ID           int64
	ConnectionID string
	DatabaseName string
	SQL          string
	ExecutedAt   time.Time
	Duration     time.Duration
	RowsAffected int64
	Success      bool
	ErrorMessage string
}

// SavedQuery is a user-saved (favorited) query.
type SavedQuery struct {
	ID           int64
	Title        string
	Description  string
	ConnectionID string
	DatabaseName string
	SQL          string
	Tags         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DefaultPath returns the history.db path, honoring $XDG_CONFIG_HOME and
// falling back to ~/.config/s9l/history.db.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "s9l", "history.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("history: cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".config", "s9l", "history.db"), nil
}

// Open opens (creating if needed) the history database at path and applies
// migrations. The directory is created with 0700, the file with 0600.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("history: create dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("history: open %s: %w", path, err)
	}
	// Ensure 0600 in case the file was just created.
	_ = os.Chmod(path, 0o600)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// OpenDefault opens the history database at DefaultPath.
func OpenDefault() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS query_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connection_id TEXT NOT NULL,
    database_name TEXT,
    sql_text TEXT NOT NULL,
    executed_at DATETIME NOT NULL,
    duration_ms INTEGER,
    rows_affected INTEGER,
    success BOOLEAN NOT NULL,
    error_message TEXT
);
CREATE TABLE IF NOT EXISTS saved_queries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT,
    connection_id TEXT,
    database_name TEXT,
    sql_text TEXT NOT NULL,
    tags TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);`

// migrate creates tables if they do not exist. It is idempotent.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("history: migrate: %w", err)
	}
	return nil
}

// AddHistory records an executed query.
func (s *Store) AddHistory(ctx context.Context, e HistoryEntry) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO query_history
		 (connection_id, database_name, sql_text, executed_at, duration_ms, rows_affected, success, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ConnectionID, nullStr(e.DatabaseName), e.SQL, e.ExecutedAt.UTC(),
		e.Duration.Milliseconds(), e.RowsAffected, e.Success, nullStr(e.ErrorMessage),
	)
	if err != nil {
		return 0, fmt.Errorf("history: add history: %w", err)
	}
	return res.LastInsertId()
}

// ListHistory returns the most recent entries, newest first, limited to limit
// (0 means no limit).
func (s *Store) ListHistory(ctx context.Context, limit int) ([]HistoryEntry, error) {
	q := `SELECT id, connection_id, COALESCE(database_name,''), sql_text, executed_at,
	             COALESCE(duration_ms,0), COALESCE(rows_affected,0), success, COALESCE(error_message,'')
	      FROM query_history ORDER BY id DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("history: list history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var ms int64
		if err := rows.Scan(&e.ID, &e.ConnectionID, &e.DatabaseName, &e.SQL, &e.ExecutedAt,
			&ms, &e.RowsAffected, &e.Success, &e.ErrorMessage); err != nil {
			return nil, fmt.Errorf("history: scan history: %w", err)
		}
		e.Duration = time.Duration(ms) * time.Millisecond
		out = append(out, e)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
