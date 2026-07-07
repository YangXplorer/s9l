package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
)

// TestCompleterAdapterOverSQLite checks the readline bridge end to end against
// a real SQLite connection: "us" completes to the table "users". (The
// schema-cache behavior itself is tested in internal/repl.)
func TestCompleterAdapterOverSQLite(t *testing.T) {
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
		t.Errorf("expected table suffix \"ers\" among %d candidates", len(suffixes))
	}
}
