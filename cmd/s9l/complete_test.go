package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/schemacache"
)

func openTempCache(t *testing.T) *schemacache.Store {
	t.Helper()
	s, err := schemacache.Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

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

	schema := newSchemaCache(ctx, conn, nil, "")
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
	comp := newCompleter(ctx, conn, nil, "")
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
	schema := newSchemaCache(context.Background(), errMetadata{tablesCalls: &calls}, nil, "")
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

// TestSchemaCacheWriteThrough checks that a successful live lookup is persisted
// to the on-disk cache for a named connection.
func TestSchemaCacheWriteThrough(t *testing.T) {
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

	store := openTempCache(t)
	schema := newSchemaCache(ctx, conn, store, "app")
	_ = schema.Tables()         // triggers live fetch + write-through
	_ = schema.Columns("users") // ditto for columns

	tables, _ := store.Tables(ctx, "app")
	if len(tables) != 1 || tables[0] != "users" {
		t.Errorf("persisted tables = %v, want [users]", tables)
	}
	cols, _ := store.Columns(ctx, "app", "users")
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Errorf("persisted columns = %v, want [id name]", cols)
	}
}

// TestSchemaCacheDiskFallback checks that when the live lookup fails, the
// last-known schema is served from the on-disk cache.
func TestSchemaCacheDiskFallback(t *testing.T) {
	ctx := context.Background()
	store := openTempCache(t)
	if err := store.SaveTables(ctx, "x", []string{"cached_tbl"}); err != nil {
		t.Fatalf("seed tables: %v", err)
	}
	if err := store.SaveColumns(ctx, "x", "cached_tbl", []string{"c1", "c2"}); err != nil {
		t.Fatalf("seed columns: %v", err)
	}

	calls := 0
	schema := newSchemaCache(ctx, errMetadata{tablesCalls: &calls}, store, "x")
	if got := schema.Tables(); len(got) != 1 || got[0] != "cached_tbl" {
		t.Errorf("fallback tables = %v, want [cached_tbl]", got)
	}
	if got := schema.Columns("cached_tbl"); len(got) != 2 {
		t.Errorf("fallback columns = %v, want 2 entries", got)
	}
}
