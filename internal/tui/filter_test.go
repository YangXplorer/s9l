package tui

import (
	"testing"

	"github.com/YangXplorer/s9l/internal/secret"
)

func sampleRows() [][]any {
	return [][]any{
		{1, "Alice", "alice@x.io"},
		{2, "Bob", nil},
		{3, "alicia", "a@y.io"},
	}
}

func TestFilterRows(t *testing.T) {
	data := sampleRows()

	if got := filterRows(data, ""); len(got) != 3 {
		t.Errorf("empty term should return all rows, got %d", len(got))
	}
	// Case-insensitive, across any column ("ali" matches Alice and alicia).
	if got := filterRows(data, "ALI"); len(got) != 2 {
		t.Errorf("term ALI matched %d rows, want 2", len(got))
	}
	// Matches a value in a non-first column.
	if got := filterRows(data, "y.io"); len(got) != 1 {
		t.Errorf("term y.io matched %d rows, want 1", len(got))
	}
	// NULL cells render as "NULL" and don't crash; no match for a missing term.
	if got := filterRows(data, "zzz"); len(got) != 0 {
		t.Errorf("term zzz matched %d rows, want 0", len(got))
	}
}

func TestApplyFilterUpdatesTable(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.setResults([]string{"id", "name", "email"}, sampleRows())

	// Header + 3 data rows.
	if got := a.results.GetRowCount(); got != 4 {
		t.Fatalf("initial rows = %d, want 4 (header+3)", got)
	}

	a.applyFilter("bob")
	if got := a.results.GetRowCount(); got != 2 { // header + 1 match
		t.Errorf("after filter rows = %d, want 2", got)
	}
	if a.filter != "bob" {
		t.Errorf("filter = %q, want bob", a.filter)
	}

	// Clearing restores the full set.
	a.applyFilter("")
	if got := a.results.GetRowCount(); got != 4 {
		t.Errorf("after clear rows = %d, want 4", got)
	}
}

func TestShowFilterNoResults(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.showFilter() // no result set yet
	if a.filterOpen {
		t.Error("filter overlay should not open with no results")
	}
}
