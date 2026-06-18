package repl_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/YangXplorer/s9l/internal/repl"
)

// fakeSchema is a static Schema for testing.
type fakeSchema struct {
	tables  []string
	columns map[string][]string
}

func (f fakeSchema) Tables() []string          { return f.tables }
func (f fakeSchema) Columns(t string) []string { return f.columns[t] }

func newTestCompleter() *repl.Completer {
	return repl.NewCompleter(fakeSchema{
		tables: []string{"users", "orders"},
		columns: map[string][]string{
			"users":  {"id", "name", "email"},
			"orders": {"id", "user_id", "total"},
		},
	})
}

func TestCompleteMetaCommands(t *testing.T) {
	c := newTestCompleter()
	got, n := c.Complete(`\d`, 2)
	if n != 2 {
		t.Errorf("prefixLen = %d, want 2", n)
	}
	// \d and \dt both start with \d; suffixes are "" (skipped, equal) and "t".
	if diff := cmp.Diff([]string{"t"}, got); diff != "" {
		t.Errorf("meta suffixes mismatch (-want +got):\n%s", diff)
	}
}

func TestCompleteKeyword(t *testing.T) {
	c := newTestCompleter()
	got, n := c.Complete("SEL", 3)
	if n != 3 {
		t.Errorf("prefixLen = %d, want 3", n)
	}
	if diff := cmp.Diff([]string{"ECT"}, got); diff != "" {
		t.Errorf("keyword suffixes mismatch (-want +got):\n%s", diff)
	}
}

func TestCompleteTableName(t *testing.T) {
	c := newTestCompleter()
	// "us" should match the "users" table (suffix "ers").
	got, _ := c.Complete("select * from us", 16)
	if !contains(got, "ers") {
		t.Errorf("expected table suffix \"ers\" for users, got %v", got)
	}
}

func TestCompleteQualifiedColumn(t *testing.T) {
	c := newTestCompleter()
	// users.<prefix> completes that table's columns.
	got, n := c.Complete("select users.na", 15)
	if n != 2 {
		t.Errorf("prefixLen = %d, want 2", n)
	}
	if diff := cmp.Diff([]string{"me"}, got); diff != "" {
		t.Errorf("qualified column mismatch (-want +got):\n%s", diff)
	}
}

func TestCompleteColumnsOfReferencedTable(t *testing.T) {
	c := newTestCompleter()
	// With "orders" referenced in the line, its columns become candidates.
	// "to" matches "total" (suffix "tal").
	got, _ := c.Complete("select to from orders", 9)
	if !contains(got, "tal") {
		t.Errorf("expected column suffix \"tal\" for total, got %v", got)
	}
	// A column of an unreferenced table must not appear: "email" (users) for
	// prefix "em" should be absent since users isn't in the line.
	got2, _ := c.Complete("select em from orders", 9)
	if contains(got2, "ail") {
		t.Errorf("did not expect users.email suffix when users not referenced, got %v", got2)
	}
}

func TestCompleteNilSchema(t *testing.T) {
	c := repl.NewCompleter(nil)
	// Keywords still work without a schema.
	got, _ := c.Complete("FRO", 3)
	if !contains(got, "M") {
		t.Errorf("expected keyword FROM suffix \"M\", got %v", got)
	}
	// No tables, no panic.
	if names, _ := c.Complete("select * from x", 15); len(names) != 0 {
		t.Errorf("expected no completions for unknown table prefix, got %v", names)
	}
}

func contains(xs []string, want string) bool {
	return slices.Contains(xs, want)
}
