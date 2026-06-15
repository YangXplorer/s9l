// Package sqlite implements the s9l driver for SQLite using the pure-Go
// modernc.org/sqlite driver (CGO-free), keeping cross-compilation and
// single-binary distribution intact.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/YangXplorer/s9l/internal/driver"

	// Pure-Go SQLite database/sql driver, registered under the name "sqlite".
	_ "modernc.org/sqlite"
)

func init() {
	driver.Register(Driver{})
}

// Driver opens SQLite databases. The DSN is a file path, or ":memory:" for an
// in-memory database.
type Driver struct{}

// Name returns the driver identifier.
func (Driver) Name() string { return "sqlite" }

// Open establishes a connection to the SQLite database at dsn.
func (Driver) Open(ctx context.Context, dsn string) (driver.Conn, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &conn{db: db}, nil
}

type conn struct{ db *sql.DB }

func (c *conn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	cols, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return nil, err
	}
	return &rowsAdapter{rows: rows, cols: cols}, nil
}

func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

func (c *conn) Close() error { return c.db.Close() }

// rowsAdapter adapts *sql.Rows to driver.Rows, exposing generic []any values
// so the renderer need not know column types ahead of time.
type rowsAdapter struct {
	rows *sql.Rows
	cols []string
}

func (r *rowsAdapter) Columns() []string { return r.cols }
func (r *rowsAdapter) Next() bool        { return r.rows.Next() }
func (r *rowsAdapter) Err() error        { return r.rows.Err() }
func (r *rowsAdapter) Close() error      { return r.rows.Close() }

func (r *rowsAdapter) Values() ([]any, error) {
	vals := make([]any, len(r.cols))
	ptrs := make([]any, len(r.cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := r.rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	// Normalize []byte (TEXT/BLOB) to string for display friendliness.
	for i, v := range vals {
		if b, ok := v.([]byte); ok {
			vals[i] = string(b)
		}
	}
	return vals, nil
}
