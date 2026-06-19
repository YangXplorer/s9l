// Package drivertest provides a conformance suite that every driver.Driver
// implementation must pass. It is run against an in-process database (e.g.
// SQLite in-memory) on every `go test`, and against real engines (PostgreSQL,
// MySQL) in container-based integration tests. New drivers must pass this
// before anything else — it is the gate that validates the abstraction holds.
package drivertest

import (
	"context"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/google/go-cmp/cmp"
)

// OpenFunc returns a fresh connection to an empty, writable database.
type OpenFunc func(ctx context.Context) (driver.Conn, error)

// dialect captures the small SQL differences between engines so the conformance
// suite stays portable. The zero value (via defaults) reproduces the standard
// SQL used by SQLite/PostgreSQL/MySQL/SQL Server; engines that differ (e.g.
// ClickHouse, which needs an ENGINE clause and Nullable types) pass Options.
type dialect struct {
	intType          string // column type for integers
	textType         string // column type for text
	nullableText     string // column type for a nullable text column
	tableSuffix      string // appended after CREATE TABLE (...) — e.g. " ENGINE = Memory"
	skipRowsAffected bool   // engine doesn't report RowsAffected for INSERT
}

func defaults() dialect {
	return dialect{intType: "INTEGER", textType: "TEXT", nullableText: "TEXT"}
}

// Option customizes the conformance dialect.
type Option func(*dialect)

// WithTypes overrides the integer / text / nullable-text column types.
func WithTypes(intType, textType, nullableText string) Option {
	return func(d *dialect) { d.intType, d.textType, d.nullableText = intType, textType, nullableText }
}

// WithTableSuffix appends s after every "CREATE TABLE (...)" (e.g. an engine clause).
func WithTableSuffix(s string) Option { return func(d *dialect) { d.tableSuffix = s } }

// SkipRowsAffected disables the RowsAffected assertion for engines that don't
// report it (e.g. ClickHouse INSERT).
func SkipRowsAffected() Option { return func(d *dialect) { d.skipRowsAffected = true } }

// RunConformance exercises the core Driver/Conn/Rows contract.
func RunConformance(t *testing.T, open OpenFunc, opts ...Option) {
	t.Helper()
	d := defaults()
	for _, o := range opts {
		o(&d)
	}

	t.Run("exec_and_query", func(t *testing.T) {
		ctx := context.Background()
		c := mustOpen(t, open, ctx)
		defer func() { _ = c.Close() }()
		mustExec(t, c, ctx, `DROP TABLE IF EXISTS t`)
		mustExec(t, c, ctx, `CREATE TABLE t (id `+d.intType+`, name `+d.textType+`)`+d.tableSuffix)
		res, err := c.Exec(ctx, `INSERT INTO t (id, name) VALUES (1, 'a'), (2, 'b')`)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if !d.skipRowsAffected {
			if n, err := res.RowsAffected(); err != nil || n != 2 {
				t.Fatalf("RowsAffected = %d, err = %v, want 2, nil", n, err)
			}
		}
		cols, rows := queryAll(t, c, ctx, `SELECT id, name FROM t ORDER BY id`)
		if diff := cmp.Diff([]string{"id", "name"}, cols); diff != "" {
			t.Errorf("columns mismatch (-want +got):\n%s", diff)
		}
		if len(rows) != 2 {
			t.Fatalf("got %d rows, want 2", len(rows))
		}
	})

	t.Run("null_handling", func(t *testing.T) {
		ctx := context.Background()
		c := mustOpen(t, open, ctx)
		defer func() { _ = c.Close() }()
		mustExec(t, c, ctx, `DROP TABLE IF EXISTS t`)
		mustExec(t, c, ctx, `CREATE TABLE t (v `+d.nullableText+`)`+d.tableSuffix)
		mustExec(t, c, ctx, `INSERT INTO t (v) VALUES (NULL)`)
		_, rows := queryAll(t, c, ctx, `SELECT v FROM t`)
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0][0] != nil {
			t.Fatalf("expected NULL (nil), got %#v", rows[0][0])
		}
	})

	t.Run("empty_result", func(t *testing.T) {
		ctx := context.Background()
		c := mustOpen(t, open, ctx)
		defer func() { _ = c.Close() }()
		mustExec(t, c, ctx, `DROP TABLE IF EXISTS t`)
		mustExec(t, c, ctx, `CREATE TABLE t (id `+d.intType+`)`+d.tableSuffix)
		cols, rows := queryAll(t, c, ctx, `SELECT id FROM t`)
		if len(cols) != 1 {
			t.Fatalf("got %d columns, want 1", len(cols))
		}
		if len(rows) != 0 {
			t.Fatalf("expected no rows, got %d", len(rows))
		}
	})

	t.Run("context_cancel", func(t *testing.T) {
		ctx := context.Background()
		c := mustOpen(t, open, ctx)
		defer func() { _ = c.Close() }()
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := c.Query(cctx, `SELECT 1`); err == nil {
			t.Fatal("expected error querying with a cancelled context")
		}
	})
}

func mustOpen(t *testing.T, open OpenFunc, ctx context.Context) driver.Conn {
	t.Helper()
	c, err := open(ctx)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return c
}

func mustExec(t *testing.T, c driver.Conn, ctx context.Context, sql string) {
	t.Helper()
	if _, err := c.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func queryAll(t *testing.T, c driver.Conn, ctx context.Context, sql string) ([]string, [][]any) {
	t.Helper()
	rows, err := c.Query(ctx, sql)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	cols := rows.Columns()
	var out [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatalf("values: %v", err)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return cols, out
}
