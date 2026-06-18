package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a saved query does not exist.
var ErrNotFound = errors.New("history: saved query not found")

// SaveQuery inserts a saved query and returns its id. CreatedAt/UpdatedAt are
// set to now if zero.
func (s *Store) SaveQuery(ctx context.Context, q SavedQuery) (int64, error) {
	if q.Title == "" {
		return 0, errors.New("history: saved query title is required")
	}
	if q.SQL == "" {
		return 0, errors.New("history: saved query sql is required")
	}
	now := time.Now().UTC()
	if q.CreatedAt.IsZero() {
		q.CreatedAt = now
	}
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = now
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO saved_queries
		 (title, description, connection_id, database_name, sql_text, tags, folder_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.Title, nullStr(q.Description), nullStr(q.ConnectionID), nullStr(q.DatabaseName),
		q.SQL, nullStr(q.Tags), nullInt64(q.FolderID), q.CreatedAt, q.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("history: save query: %w", err)
	}
	return res.LastInsertId()
}

// GetSaved returns the saved query with the given id, or ErrNotFound.
func (s *Store) GetSaved(ctx context.Context, id int64) (SavedQuery, error) {
	row := s.db.QueryRowContext(ctx, savedSelect+" WHERE id = ?", id)
	q, err := scanSaved(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedQuery{}, ErrNotFound
	}
	return q, err
}

// ListSaved returns all saved queries, newest first.
func (s *Store) ListSaved(ctx context.Context) ([]SavedQuery, error) {
	return s.querySaved(ctx, savedSelect+" ORDER BY id DESC")
}

// SearchSaved returns saved queries whose title, tags or sql match the term
// (case-insensitive substring), optionally filtered by connectionID ("" = any).
func (s *Store) SearchSaved(ctx context.Context, term, connectionID string) ([]SavedQuery, error) {
	like := "%" + term + "%"
	q := savedSelect + ` WHERE (title LIKE ? OR COALESCE(tags,'') LIKE ? OR sql_text LIKE ?)`
	args := []any{like, like, like}
	if connectionID != "" {
		q += " AND connection_id = ?"
		args = append(args, connectionID)
	}
	q += " ORDER BY id DESC"
	return s.querySaved(ctx, q, args...)
}

// DeleteSaved removes a saved query, reporting whether it existed.
func (s *Store) DeleteSaved(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saved_queries WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("history: delete saved: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

const savedSelect = `SELECT id, title, COALESCE(description,''), COALESCE(connection_id,''),
	COALESCE(database_name,''), sql_text, COALESCE(tags,''), COALESCE(folder_id,0), created_at, updated_at
	FROM saved_queries`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSaved(r rowScanner) (SavedQuery, error) {
	var q SavedQuery
	err := r.Scan(&q.ID, &q.Title, &q.Description, &q.ConnectionID, &q.DatabaseName,
		&q.SQL, &q.Tags, &q.FolderID, &q.CreatedAt, &q.UpdatedAt)
	return q, err
}

func (s *Store) querySaved(ctx context.Context, query string, args ...any) ([]SavedQuery, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("history: query saved: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SavedQuery
	for rows.Next() {
		q, err := scanSaved(rows)
		if err != nil {
			return nil, fmt.Errorf("history: scan saved: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}
