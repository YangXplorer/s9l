// Package clickhouse implements the s9l driver for ClickHouse using the pure-Go
// ClickHouse/clickhouse-go/v2 adapter (database/sql, no CGO). It mirrors the
// other adapters; ClickHouse-specific dialect (DSN form, system-table metadata)
// lives here, keeping the core driver-agnostic.
package clickhouse

import (
	"context"
	"database/sql"

	"github.com/YangXplorer/s9l/internal/driver"

	// Pure-Go ClickHouse database/sql driver, registered as "clickhouse".
	_ "github.com/ClickHouse/clickhouse-go/v2"
)

func init() {
	driver.Register(Driver{})
}

// Driver opens ClickHouse databases. The DSN is a clickhouse-go URL, e.g.
// "clickhouse://user:pass@host:9000/db".
type Driver struct{}

// Name returns the driver identifier (also the DSN scheme).
func (Driver) Name() string { return "clickhouse" }

// Open establishes a connection to the ClickHouse database at dsn.
func (Driver) Open(ctx context.Context, dsn string) (driver.Conn, error) {
	db, err := sql.Open("clickhouse", dsn)
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
	return c.Query(ctx, `SELECT name FROM system.databases ORDER BY name`)
}

// Tables implements driver.Metadata (\dt): tables in the current database.
func (c *conn) Tables(ctx context.Context) (driver.Rows, error) {
	return c.Query(ctx, `SELECT name FROM system.tables
	                     WHERE database = currentDatabase()
	                     ORDER BY name`)
}

// Columns implements driver.Metadata (\d <table>). The table name is bound as a
// parameter, not interpolated.
func (c *conn) Columns(ctx context.Context, table string) (driver.Rows, error) {
	return c.Query(ctx, `SELECT name, type FROM system.columns
	                     WHERE database = currentDatabase() AND table = ?
	                     ORDER BY position`, table)
}

// rowsAdapter adapts *sql.Rows to driver.Rows, normalizing []byte to string so
// downstream renderers (e.g. JSON) don't emit base64 for text columns.
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
