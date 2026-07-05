package tui

import (
	"context"
	"slices"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/repl"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/gdamore/tcell/v2"
)

// fakeMetaConn is a Metadata-capable conn with fixed tables/columns and call
// counters, for exercising the completion wiring without a database.
type fakeMetaConn struct {
	fakeBrowserConn
	tables      []string
	cols        map[string][]string
	tableCalls  int
	columnCalls int
}

func (c *fakeMetaConn) Tables(context.Context) (driver.Rows, error) {
	c.tableCalls++
	return nameRows(c.tables), nil
}

func (c *fakeMetaConn) Columns(_ context.Context, table string) (driver.Rows, error) {
	c.columnCalls++
	return nameRows(c.cols[table]), nil
}

func TestBuildCompleterCompletesTablesAndColumns(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.conn = &fakeMetaConn{
		tables: []string{"users", "orders"},
		cols:   map[string][]string{"users": {"id", "last_name"}},
	}
	a.buildCompleter("") // "" = no persistence

	if a.completer == nil {
		t.Fatal("completer should be built after connect")
	}
	suffixes, _ := a.completer.Complete("select * from us", 16)
	if !containsSuffix(suffixes, "ers") {
		t.Errorf("table completion for 'us' = %v, want to include \"ers\"", suffixes)
	}
	// Qualified column: users.la → last_name.
	suffixes, _ = a.completer.Complete("select users.la", 15)
	if !containsSuffix(suffixes, "st_name") {
		t.Errorf("column completion for 'users.la' = %v, want to include \"st_name\"", suffixes)
	}
}

// The Schema panel's table list (which follows the picked database) takes
// precedence over connection-level metadata.
func TestCompleterPrefersSchemaPanelTables(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	conn := &fakeMetaConn{tables: []string{"conn_db_table"}}
	a.conn = conn
	a.buildCompleter("")

	a.schemaTables = []string{"picked_db_table"}
	suffixes, _ := a.completer.Complete("select * from picked", 20)
	if !containsSuffix(suffixes, "_db_table") {
		t.Errorf("completion should offer the Schema panel's tables, got %v", suffixes)
	}
	if conn.tableCalls != 0 {
		t.Errorf("metadata Tables() called %d times, want 0 (schemaTables preferred)", conn.tableCalls)
	}
}

// While a query runs, completion must not issue DB round-trips (the conn is
// shared with the running query); memory-only candidates still work.
func TestCompleterNoDBWhileRunning(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	conn := &fakeMetaConn{tables: []string{"users"}, cols: map[string][]string{"users": {"id"}}}
	a.conn = conn
	a.buildCompleter("")
	a.running = true

	base := repl.NewSchemaCache(context.Background(), conn, nil, "")
	if got := (tuiSchema{a: a, base: base}).Columns("users"); got != nil {
		t.Errorf("Columns while running = %v, want nil (no DB round-trip)", got)
	}
	if conn.columnCalls != 0 {
		t.Errorf("metadata Columns() called %d times while running, want 0", conn.columnCalls)
	}

	// The previewed table's columns are served from memory even while running.
	a.resultTable = tableRef{name: "users"}
	a.lastCols = []string{"id", "last_name"}
	if got := (tuiSchema{a: a}).Columns("users"); len(got) != 2 {
		t.Errorf("preview columns while running = %v, want lastCols", got)
	}
}

func TestCloseConnDropsCompleter(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.conn = &fakeMetaConn{}
	a.buildCompleter("")
	if a.completer == nil {
		t.Fatal("completer expected")
	}
	a.closeConn()
	if a.completer != nil {
		t.Error("closeConn should drop the completer")
	}
}

func containsSuffix(suffixes []string, want string) bool {
	return slices.Contains(suffixes, want)
}

func TestIdentifierPrefix(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"start of expression", "las", "las"},
		{"after operator", "id > 10 AND na", "na"},
		{"after space", "last_name ", ""},
		{"inside string literal", "name = 'ya", ""},
		{"after closed literal", "name = 'x' AND em", "em"},
		{"escaped quote keeps literal open", "note = 'it''s ya", ""},
		{"cjk identifier", "名前", "名前"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := identifierPrefix(c.in); got != c.want {
			t.Errorf("%s: identifierPrefix(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestWhereCandidates(t *testing.T) {
	cols := []string{"id", "last_name", "first_name", "name_kana"}
	// Prefix matches come before substring matches, case-insensitively.
	if got := whereCandidates(cols, "WHERE na"); len(got) != 3 || got[0] != "name_kana" {
		t.Errorf("candidates for 'na' = %v, want [name_kana last_name first_name]", got)
	}
	if got := whereCandidates(cols, "LAST"); len(got) != 1 || got[0] != "last_name" {
		t.Errorf("candidates for 'LAST' = %v, want [last_name]", got)
	}
	// No candidates: empty prefix, inside a literal, or only the exact word.
	for name, text := range map[string]string{
		"empty prefix":   "id = 1 ",
		"inside literal": "name = 'la",
		"exact word":     "last_name",
	} {
		if got := whereCandidates(cols, text); got != nil {
			t.Errorf("%s: candidates = %v, want nil", name, got)
		}
	}
}

// Enter has two states while the WHERE input is open: with the completion
// dropdown showing it goes to the input (candidate selection); with it hidden
// it applies the filter.
func TestWhereEnterTwoStates(t *testing.T) {
	a := previewApp()
	a.setResults([]string{"id", "last_name"}, [][]any{{int64(1), "x"}})
	a.focusPanel(2)
	a.showFilter()
	if !a.filterOpen || a.filterTarget != filterTgtResultsWhere {
		t.Fatal("WHERE input should be open")
	}

	// Dropdown showing → Enter passes through to the input.
	a.acOpen = true
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev == nil {
		t.Error("Enter with the dropdown open must go to the input, not apply")
	}
	if !a.filterOpen {
		t.Fatal("filter must stay open while selecting a candidate")
	}

	// Dropdown hidden → Enter applies.
	a.acOpen = false
	a.pendingWhere = "last_name = 'x'"
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Error("Enter with the dropdown hidden must be consumed (apply)")
	}
	if a.filterOpen {
		t.Error("filter should close on apply")
	}
	if a.resultWhere != "last_name = 'x'" {
		t.Errorf("applied where = %q", a.resultWhere)
	}
}
