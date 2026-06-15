// Package driver defines the database driver abstraction for s9l.
//
// All database-specific behavior lives behind these interfaces; the core
// (cli/repl/render/history) depends only on them. Adding a new database means
// adding one driver package that implements Driver and registers itself via
// Register — without touching the core. This is the extensibility hinge
// described in docs/PLAN.md, so change it deliberately.
package driver

import (
	"context"
	"fmt"
	"sort"
)

// Driver opens connections to a specific database system.
type Driver interface {
	// Name is the unique identifier of the driver, e.g. "sqlite", "postgres".
	//
	// It doubles as the DSN scheme: callers select a driver by this name (see
	// Open/Get). An earlier design split this into Name() and Scheme(), but a
	// single value keeps registration and selection unambiguous and avoids a
	// second source of truth; we merge them deliberately.
	Name() string
	// Open establishes a connection using a driver-specific DSN.
	Open(ctx context.Context, dsn string) (Conn, error)
}

// Conn is a live database connection.
type Conn interface {
	// Query runs a statement that returns rows; rows are streamed.
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	// Exec runs a statement that does not return rows.
	Exec(ctx context.Context, query string, args ...any) (Result, error)
	// Close releases the connection.
	Close() error
}

// Rows is a forward-only, streaming cursor. Callers must Close it.
type Rows interface {
	// Columns returns the column names of the result set.
	Columns() []string
	// Next advances to the next row, returning false at end-of-rows or on
	// error (check Err afterwards).
	Next() bool
	// Values returns the current row's column values. A SQL NULL is
	// represented as a nil any.
	Values() ([]any, error)
	// Err returns the first non-EOF error encountered during iteration.
	Err() error
	// Close releases resources held by the cursor.
	Close() error
}

// Result reports the outcome of an Exec.
type Result interface {
	// RowsAffected returns the number of rows affected by the statement.
	RowsAffected() (int64, error)
}

// Metadata is an optional capability a Conn may also implement to introspect
// the schema, backing the \l, \dt and \d meta-commands. Each result is a normal
// streaming Rows so it renders through the same path as a query. Drivers that
// don't implement it simply don't support those commands. The dialect-specific
// SQL lives in each driver, keeping these differences out of the core.
type Metadata interface {
	// Databases lists databases/schemas (\l).
	Databases(ctx context.Context) (Rows, error)
	// Tables lists tables in the current database (\dt).
	Tables(ctx context.Context) (Rows, error)
	// Columns describes the columns of a table (\d <table>).
	Columns(ctx context.Context, table string) (Rows, error)
}

var registry = map[string]Driver{}

// Register makes a driver available by its Name. It is intended to be called
// from driver package init functions. It panics on a nil driver or duplicate
// name, both of which are programmer errors detectable at startup.
func Register(d Driver) {
	if d == nil {
		panic("driver: Register called with nil Driver")
	}
	name := d.Name()
	if _, dup := registry[name]; dup {
		panic("driver: Register called twice for driver " + name)
	}
	registry[name] = d
}

// Get returns the driver registered under name.
func Get(name string) (Driver, error) {
	d, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("driver: unknown driver %q (registered: %v)", name, Names())
	}
	return d, nil
}

// Open looks up the named driver and opens a connection with the given DSN.
func Open(ctx context.Context, name, dsn string) (Conn, error) {
	d, err := Get(name)
	if err != nil {
		return nil, err
	}
	return d.Open(ctx, dsn)
}

// Names returns the sorted list of registered driver names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
