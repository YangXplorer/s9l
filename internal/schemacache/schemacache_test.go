package schemacache_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/YangXplorer/s9l/internal/schemacache"
)

func openTemp(t *testing.T) *schemacache.Store {
	t.Helper()
	s, err := schemacache.Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.db")
	s1, err := schemacache.Open(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	_ = s1.Close()
	s2, err := schemacache.Open(path)
	if err != nil {
		t.Fatalf("open 2 (re-migrate): %v", err)
	}
	_ = s2.Close()
}

func TestTablesRoundTripAndReplace(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	// Empty when nothing cached.
	if got, err := s.Tables(ctx, "pg"); err != nil || len(got) != 0 {
		t.Fatalf("empty tables: got %v err %v", got, err)
	}

	if err := s.SaveTables(ctx, "pg", []string{"users", "orders"}); err != nil {
		t.Fatalf("save tables: %v", err)
	}
	got, err := s.Tables(ctx, "pg")
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	if diff := cmp.Diff([]string{"orders", "users"}, got); diff != "" { // ordered by name
		t.Fatalf("tables mismatch (-want +got):\n%s", diff)
	}

	// Replace overwrites the previous set.
	if err := s.SaveTables(ctx, "pg", []string{"products"}); err != nil {
		t.Fatalf("replace tables: %v", err)
	}
	got, _ = s.Tables(ctx, "pg")
	if diff := cmp.Diff([]string{"products"}, got); diff != "" {
		t.Fatalf("after replace (-want +got):\n%s", diff)
	}

	// Different connection ids are isolated.
	if got, _ := s.Tables(ctx, "other"); len(got) != 0 {
		t.Fatalf("other conn should be empty, got %v", got)
	}
}

func TestColumnsPreserveOrder(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	cols := []string{"id", "name", "email"}
	if err := s.SaveColumns(ctx, "pg", "users", cols); err != nil {
		t.Fatalf("save columns: %v", err)
	}
	got, err := s.Columns(ctx, "pg", "users")
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	if diff := cmp.Diff(cols, got); diff != "" { // ordinal order, not alphabetical
		t.Fatalf("columns mismatch (-want +got):\n%s", diff)
	}
}
