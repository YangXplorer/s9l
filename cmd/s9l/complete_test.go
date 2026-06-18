package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
)

// TestSchemaCacheOverSQLite checks the metadata-backed Schema against a real
// SQLite connection: tables and columns are discovered and the completer wires
// up end to end.
func TestSchemaCacheOverSQLite(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "c.db")
	conn, err := driver.Open(ctx, "sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Query(ctx, "create table users(id integer, name text)"); err != nil {
		t.Fatalf("create: %v", err)
	}

	schema := newSchemaCache(ctx, conn)
	if schema == nil {
		t.Fatal("sqlite conn should provide Metadata-backed schema")
	}

	if got := schema.Tables(); len(got) != 1 || got[0] != "users" {
		t.Fatalf("Tables() = %v, want [users]", got)
	}
	cols := schema.Columns("users")
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("Columns(users) = %v, want [id name]", cols)
	}

	// Completion through the readline adapter: "us" -> table "users".
	comp := newCompleter(ctx, conn)
	suffixes, prefixLen := comp.Do([]rune("select * from us"), 16)
	if prefixLen != 2 {
		t.Errorf("prefixLen = %d, want 2", prefixLen)
	}
	var found bool
	for _, s := range suffixes {
		if string(s) == "ers" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected table suffix \"ers\" among %q", runesToStrings(suffixes))
	}
}

func runesToStrings(rs [][]rune) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

// errMetadata is a Conn whose Metadata methods always fail, exercising the
// schemaCache error path (cache nil, do not refetch).
type errMetadata struct {
	tablesCalls *int
}

func (errMetadata) Query(context.Context, string, ...any) (driver.Rows, error) {
	return nil, errors.New("query unused")
}
func (errMetadata) Exec(context.Context, string, ...any) (driver.Result, error) {
	return nil, errors.New("exec unused")
}
func (errMetadata) Close() error { return nil }
func (errMetadata) Databases(context.Context) (driver.Rows, error) {
	return nil, errors.New("no databases")
}
func (e errMetadata) Tables(context.Context) (driver.Rows, error) {
	*e.tablesCalls++
	return nil, errors.New("no tables")
}
func (errMetadata) Columns(context.Context, string) (driver.Rows, error) {
	return nil, errors.New("no columns")
}

func TestSchemaCacheErrorIsCached(t *testing.T) {
	calls := 0
	schema := newSchemaCache(context.Background(), errMetadata{tablesCalls: &calls})
	if schema == nil {
		t.Fatal("errMetadata implements Metadata; schema should be non-nil")
	}
	// First call errors → nil; second call must reuse the cache (no refetch).
	if got := schema.Tables(); got != nil {
		t.Errorf("Tables() on error = %v, want nil", got)
	}
	if got := schema.Tables(); got != nil {
		t.Errorf("second Tables() = %v, want nil", got)
	}
	if calls != 1 {
		t.Errorf("md.Tables called %d times, want 1 (cached after error)", calls)
	}
	// Columns error path also yields nil without panicking.
	if got := schema.Columns("anything"); got != nil {
		t.Errorf("Columns() on error = %v, want nil", got)
	}
}
