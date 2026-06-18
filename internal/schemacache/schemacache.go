// Package schemacache persists table/column names per connection in a SQLite
// database at ~/.cache/s9l/schema.db (honoring $XDG_CACHE_HOME). It accelerates
// REPL completion: names from a previous session remain available, and stay
// usable when a live metadata lookup fails. It caches only schema names, never
// any credential — callers key by a stable connection id, not a DSN.
package schemacache

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store is a handle to the schema cache database.
type Store struct {
	db *sql.DB
}

// DefaultPath returns the schema.db path, honoring $XDG_CACHE_HOME and falling
// back to ~/.cache/s9l/schema.db.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "s9l", "schema.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("schemacache: cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "s9l", "schema.db"), nil
}

// Open opens (creating if needed) the cache database at path and migrates it.
// The directory is created with 0700, the file with 0600.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("schemacache: create dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("schemacache: open %s: %w", path, err)
	}
	_ = os.Chmod(path, 0o600)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// OpenDefault opens the cache database at DefaultPath.
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
CREATE TABLE IF NOT EXISTS cached_tables (
    connection_id TEXT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (connection_id, name)
);
CREATE TABLE IF NOT EXISTS cached_columns (
    connection_id TEXT NOT NULL,
    table_name TEXT NOT NULL,
    name TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    PRIMARY KEY (connection_id, table_name, name)
);`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("schemacache: migrate: %w", err)
	}
	return nil
}

// Tables returns the cached table names for connID, ordered by name.
func (s *Store) Tables(ctx context.Context, connID string) ([]string, error) {
	return s.queryNames(ctx,
		`SELECT name FROM cached_tables WHERE connection_id = ? ORDER BY name`, connID)
}

// SaveTables replaces the cached table list for connID.
func (s *Store) SaveTables(ctx context.Context, connID string, names []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("schemacache: save tables: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM cached_tables WHERE connection_id = ?`, connID); err != nil {
		return fmt.Errorf("schemacache: clear tables: %w", err)
	}
	for _, n := range names {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO cached_tables (connection_id, name) VALUES (?, ?)`,
			connID, n); err != nil {
			return fmt.Errorf("schemacache: insert table: %w", err)
		}
	}
	return tx.Commit()
}

// Columns returns the cached column names for connID's table, in stored order.
func (s *Store) Columns(ctx context.Context, connID, table string) ([]string, error) {
	return s.queryNames(ctx,
		`SELECT name FROM cached_columns WHERE connection_id = ? AND table_name = ? ORDER BY ordinal`,
		connID, table)
}

// SaveColumns replaces the cached columns for connID's table.
func (s *Store) SaveColumns(ctx context.Context, connID, table string, names []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("schemacache: save columns: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM cached_columns WHERE connection_id = ? AND table_name = ?`,
		connID, table); err != nil {
		return fmt.Errorf("schemacache: clear columns: %w", err)
	}
	for i, n := range names {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO cached_columns (connection_id, table_name, name, ordinal) VALUES (?, ?, ?, ?)`,
			connID, table, n, i); err != nil {
			return fmt.Errorf("schemacache: insert column: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) queryNames(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("schemacache: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("schemacache: scan: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
