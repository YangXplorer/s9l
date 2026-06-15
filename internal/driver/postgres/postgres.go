// Package postgres implements the s9l driver for PostgreSQL using the pure-Go
// jackc/pgx stdlib adapter (database/sql, no CGO). It mirrors the SQLite
// adapter's shape so the core stays driver-agnostic; PostgreSQL-specific
// dialect (DSN form, $1 placeholders, catalog queries) lives here.
package postgres

import (
	"context"
	"database/sql"

	"github.com/YangXplorer/s9l/internal/driver"

	// Pure-Go PostgreSQL database/sql driver, registered as "pgx".
	_ "github.com/jackc/pgx/v5/stdlib"
)

func init() {
	driver.Register(Driver{})
}

// Driver opens PostgreSQL databases. The DSN is a libpq/URL connection string,
// e.g. "postgres://user:pass@host:5432/db?sslmode=disable".
type Driver struct{}

// Name returns the driver identifier (also the DSN scheme).
func (Driver) Name() string { return "postgres" }

// Open establishes a connection to the PostgreSQL database at dsn.
func (Driver) Open(ctx context.Context, dsn string) (driver.Conn, error) {
	db, err := sql.Open("pgx", dsn)
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

// Databases implements driver.Metadata (\l).
func (c *conn) Databases(ctx context.Context) (driver.Rows, error) {
	return c.Query(ctx, `SELECT datname FROM pg_database
	                     WHERE datistemplate = false ORDER BY datname`)
}

// Tables implements driver.Metadata (\dt): tables in user schemas.
func (c *conn) Tables(ctx context.Context) (driver.Rows, error) {
	return c.Query(ctx, `SELECT tablename, schemaname FROM pg_catalog.pg_tables
	                     WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
	                     ORDER BY schemaname, tablename`)
}

// Columns implements driver.Metadata (\d <table>). The table name is bound as a
// parameter ($1), not interpolated.
func (c *conn) Columns(ctx context.Context, table string) (driver.Rows, error) {
	return c.Query(ctx, `SELECT column_name, data_type, is_nullable, column_default
	                     FROM information_schema.columns
	                     WHERE table_name = $1
	                     ORDER BY ordinal_position`, table)
}

// rowsAdapter adapts *sql.Rows to driver.Rows, normalizing []byte to string so
// downstream renderers (e.g. JSON) don't emit base64 for text/bytea columns.
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
	for i, v := range vals {
		if b, ok := v.([]byte); ok {
			vals[i] = string(b)
		}
	}
	return vals, nil
}
