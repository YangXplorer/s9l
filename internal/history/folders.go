package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrFolderNotFound is returned when a folder does not exist.
var ErrFolderNotFound = errors.New("history: folder not found")

// CreateFolder inserts a folder and returns its id. Names are unique.
func (s *Store) CreateFolder(ctx context.Context, name string) (int64, error) {
	if name == "" {
		return 0, errors.New("history: folder name is required")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO query_folders (name, created_at) VALUES (?, ?)`,
		name, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("history: create folder: %w", err)
	}
	return res.LastInsertId()
}

// ListFolders returns all folders ordered by name.
func (s *Store) ListFolders(ctx context.Context) ([]Folder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM query_folders ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("history: list folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("history: scan folder: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFolder removes a folder, reporting whether it existed. Saved queries in
// the folder are kept but unfiled (folder_id reset to NULL).
func (s *Store) DeleteFolder(ctx context.Context, id int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("history: delete folder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE saved_queries SET folder_id = NULL WHERE folder_id = ?`, id); err != nil {
		return false, fmt.Errorf("history: unfile queries: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM query_folders WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("history: delete folder: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("history: delete folder commit: %w", err)
	}
	return n > 0, nil
}

// SetSavedFolder assigns a saved query to a folder (folderID 0 unfiles it).
// It reports ErrNotFound if the query is missing and ErrFolderNotFound if a
// non-zero folder does not exist.
func (s *Store) SetSavedFolder(ctx context.Context, savedID, folderID int64) error {
	if folderID != 0 {
		var exists int
		err := s.db.QueryRowContext(ctx,
			`SELECT 1 FROM query_folders WHERE id = ?`, folderID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFolderNotFound
		}
		if err != nil {
			return fmt.Errorf("history: check folder: %w", err)
		}
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE saved_queries SET folder_id = ?, updated_at = ? WHERE id = ?`,
		nullInt64(folderID), time.Now().UTC(), savedID)
	if err != nil {
		return fmt.Errorf("history: set folder: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListSavedByFolder returns saved queries in the given folder, newest first.
// folderID 0 returns unfiled queries (folder_id IS NULL).
func (s *Store) ListSavedByFolder(ctx context.Context, folderID int64) ([]SavedQuery, error) {
	if folderID == 0 {
		return s.querySaved(ctx, savedSelect+" WHERE folder_id IS NULL ORDER BY id DESC")
	}
	return s.querySaved(ctx, savedSelect+" WHERE folder_id = ? ORDER BY id DESC", folderID)
}
