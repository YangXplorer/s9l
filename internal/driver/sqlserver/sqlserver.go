// Package sqlserver implements the s9l driver for Microsoft SQL Server using the
// pure-Go microsoft/go-mssqldb adapter (database/sql, no CGO). It mirrors the
// SQLite/PostgreSQL/MySQL adapters; SQL Server-specific dialect (DSN form,
// INFORMATION_SCHEMA queries, @p1 placeholders) lives here, keeping the core
// driver-agnostic.
package sqlserver

import (
	"context"
	"database/sql"

	"github.com/YangXplorer/s9l/internal/driver"

	// Pure-Go SQL Server database/sql driver, registered as "sqlserver".
	_ "github.com/microsoft/go-mssqldb"
)

func init() {
	driver.Register(Driver{})
}

// Driver opens SQL Server databases. The DSN is a go-mssqldb URL, e.g.
// "sqlserver://user:pass@host:1433?database=db&encrypt=disable".
type Driver struct{}

// Name returns the driver identifier (also the DSN scheme).
func (Driver) Name() string { return "sqlserver" }

// Open establishes a connection to the SQL Server database at dsn.
func (Driver) Open(ctx context.Context, dsn string) (driver.Conn, error) {
	db, err := sql.Open("sqlserver", dsn)
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
	return c.Query(ctx, `SELECT name FROM sys.databases ORDER BY name`)
}

// Tables implements driver.Metadata (\dt): base tables in the current database.
// INFORMATION_SCHEMA is per-database, so this lists the connected database.
func (c *conn) Tables(ctx context.Context) (driver.Rows, error) {
	return c.Query(ctx, `SELECT TABLE_NAME AS name FROM INFORMATION_SCHEMA.TABLES
	                     WHERE TABLE_TYPE = 'BASE TABLE'
	                     ORDER BY TABLE_NAME`)
}

// Columns implements driver.Metadata (\d <table>). The table name is bound as a
// parameter (@p1), not interpolated.
func (c *conn) Columns(ctx context.Context, table string) (driver.Rows, error) {
	return c.Query(ctx, `SELECT COLUMN_NAME AS name, DATA_TYPE AS type,
	                            IS_NULLABLE AS nullable, COLUMN_DEFAULT AS dflt
	                     FROM INFORMATION_SCHEMA.COLUMNS
	                     WHERE TABLE_NAME = @p1
	                     ORDER BY ORDINAL_POSITION`, table)
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
