package sqlite_test

import (
	"context"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"
)

func TestMetadata(t *testing.T) {
	ctx := context.Background()
	conn, err := driver.Open(ctx, "sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	md, ok := conn.(driver.Metadata)
	if !ok {
		t.Fatal("sqlite conn should implement driver.Metadata")
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("tables", func(t *testing.T) {
		rows, err := md.Tables(ctx)
		if err != nil {
			t.Fatalf("Tables: %v", err)
		}
		names := firstColumn(t, rows)
		if len(names) != 1 || names[0] != "users" {
			t.Fatalf("Tables = %v, want [users]", names)
		}
	})

	t.Run("columns", func(t *testing.T) {
		rows, err := md.Columns(ctx, "users")
		if err != nil {
			t.Fatalf("Columns: %v", err)
		}
		cols := firstColumn(t, rows)
		if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
			t.Fatalf("Columns = %v, want [id name]", cols)
		}
	})

	t.Run("databases", func(t *testing.T) {
		rows, err := md.Databases(ctx)
		if err != nil {
			t.Fatalf("Databases: %v", err)
		}
		dbs := firstColumn(t, rows)
		if len(dbs) == 0 || dbs[0] != "main" {
			t.Fatalf("Databases = %v, want first 'main'", dbs)
		}
	})
}

// firstColumn collects the first column of every row as a string.
func firstColumn(t *testing.T, rows driver.Rows) []string {
	t.Helper()
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatalf("values: %v", err)
		}
		if s, ok := vals[0].(string); ok {
			out = append(out, s)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return out
}
