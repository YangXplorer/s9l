package history_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/history"
)

func openTemp(t *testing.T) *history.Store {
	t.Helper()
	s, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s1, err := history.Open(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	_ = s1.Close()
	// Re-open applies migrations again; must not error.
	s2, err := history.Open(path)
	if err != nil {
		t.Fatalf("open 2 (re-migrate): %v", err)
	}
	_ = s2.Close()
}

func TestAddAndListHistory(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	if _, err := s.AddHistory(ctx, history.HistoryEntry{
		ConnectionID: "local", DatabaseName: "demo", SQL: "SELECT 1",
		ExecutedAt: time.Now(), Duration: 42 * time.Millisecond, RowsAffected: 1, Success: true,
	}); err != nil {
		t.Fatalf("add ok: %v", err)
	}
	if _, err := s.AddHistory(ctx, history.HistoryEntry{
		ConnectionID: "local", SQL: "SELECT bad", ExecutedAt: time.Now(),
		Success: false, ErrorMessage: "syntax error",
	}); err != nil {
		t.Fatalf("add fail: %v", err)
	}

	entries, err := s.ListHistory(ctx, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Newest first: the failing one.
	if entries[0].Success || entries[0].ErrorMessage != "syntax error" {
		t.Errorf("expected failing entry first, got %+v", entries[0])
	}
	if entries[1].Duration != 42*time.Millisecond {
		t.Errorf("duration round-trip = %v, want 42ms", entries[1].Duration)
	}

	// Limit.
	one, err := s.ListHistory(ctx, 1)
	if err != nil || len(one) != 1 {
		t.Fatalf("limit: got %d, %v; want 1", len(one), err)
	}
}

func TestSavedQueryCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id, err := s.SaveQuery(ctx, history.SavedQuery{
		Title: "recent orders", ConnectionID: "pg", SQL: "SELECT * FROM orders", Tags: "orders,daily",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetSaved(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "recent orders" || got.SQL != "SELECT * FROM orders" {
		t.Fatalf("unexpected saved query: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps should be set")
	}

	// Validation.
	if _, err := s.SaveQuery(ctx, history.SavedQuery{SQL: "x"}); err == nil {
		t.Error("missing title should error")
	}
	if _, err := s.SaveQuery(ctx, history.SavedQuery{Title: "x"}); err == nil {
		t.Error("missing sql should error")
	}

	// Search by tag and by connection.
	res, err := s.SearchSaved(ctx, "daily", "")
	if err != nil || len(res) != 1 {
		t.Fatalf("search tag: got %d, %v; want 1", len(res), err)
	}
	res, err = s.SearchSaved(ctx, "orders", "other")
	if err != nil || len(res) != 0 {
		t.Fatalf("search wrong conn: got %d, %v; want 0", len(res), err)
	}

	// Delete.
	ok, err := s.DeleteSaved(ctx, id)
	if err != nil || !ok {
		t.Fatalf("delete: %v, ok=%v", err, ok)
	}
	if _, err := s.GetSaved(ctx, id); err != history.ErrNotFound {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
	ok, _ = s.DeleteSaved(ctx, id)
	if ok {
		t.Error("delete again should report false")
	}
}
