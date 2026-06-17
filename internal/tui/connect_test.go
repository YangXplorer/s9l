package tui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/history"
	"github.com/YangXplorer/s9l/internal/secret"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"

	"github.com/gdamore/tcell/v2"
)

func tempHistory(t *testing.T) *history.Store {
	t.Helper()
	h, err := history.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func sqliteCfg(id, path string) *config.Config {
	return &config.Config{Connections: []config.ConnectionConfig{
		{ID: id, Driver: "sqlite", Database: path},
	}}
}

func TestConnectSQLite(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := sqliteCfg("demo", db)
	a := New(Options{Config: cfg, Store: secret.NewMemory()})
	defer a.closeConn()

	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if a.conn == nil {
		t.Fatal("conn should be set after a successful connect")
	}
	if a.connID != "demo" {
		t.Errorf("connID = %q, want demo", a.connID)
	}
}

func TestConnectErrorDoesNotCrash(t *testing.T) {
	cfg := sqliteCfg("bad", "/nonexistent_dir/x.db")
	a := New(Options{Config: cfg, Store: secret.NewMemory()})

	cc, _ := cfg.Get("bad")
	if err := a.connect(cc); err == nil {
		t.Fatal("expected connect error for an unopenable path")
	}
	if a.conn != nil {
		t.Fatal("conn should stay nil after a failed connect")
	}
}

func TestAutoConnect(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	a := New(Options{Conn: "demo", Config: sqliteCfg("demo", db), Store: secret.NewMemory()})
	defer a.closeConn()

	if a.conn == nil {
		t.Fatal("auto-connect should have opened the connection")
	}
	if a.connID != "demo" {
		t.Errorf("connID = %q, want demo", a.connID)
	}
}

func TestConnectionsPopulated(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	if got := a.connList.GetItemCount(); got != 1 {
		t.Fatalf("connList items = %d, want 1", got)
	}
}

func TestLoadSchemaShowsTables(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := sqliteCfg("demo", db)
	a := New(Options{Config: cfg, Store: secret.NewMemory()})
	defer a.closeConn()

	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ctx := context.Background()
	if _, err := a.conn.Exec(ctx, "create table users(id int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.conn.Exec(ctx, "create table orders(id int)"); err != nil {
		t.Fatal(err)
	}
	a.loadSchema()

	got := map[string]bool{}
	for _, n := range a.schema.GetRoot().GetChildren() {
		got[n.GetText()] = true
		if ref, _ := n.GetReference().(string); ref != n.GetText() {
			t.Errorf("table node %q should carry its name as reference, got %q", n.GetText(), ref)
		}
	}
	if !got["users"] || !got["orders"] {
		t.Fatalf("schema tree missing tables, got %v", got)
	}
}

// fetch + fillResults are the synchronous core of query execution (runQuery
// wraps them in a goroutine); test them directly without the event loop.
func TestFetchAndFillResults(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := sqliteCfg("demo", db)
	a := New(Options{Config: cfg, Store: secret.NewMemory()})
	defer a.closeConn()

	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ctx := context.Background()
	if _, err := a.conn.Exec(ctx, "create table t(id int, name text)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.conn.Exec(ctx, "insert into t values(1,'a'),(2,null)"); err != nil {
		t.Fatal(err)
	}

	res, err := fetch(ctx, a.conn, "select id, name from t order by id")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	a.fillResults(res.cols, res.data)

	if got := a.results.GetRowCount(); got != 3 { // header + 2 rows
		t.Fatalf("results row count = %d, want 3", got)
	}
	if h := a.results.GetCell(0, 0).Text; h != "id" {
		t.Errorf("header(0,0) = %q, want id", h)
	}
	if v := a.results.GetCell(1, 1).Text; v != "a" {
		t.Errorf("cell(1,1) = %q, want a", v)
	}
	if v := a.results.GetCell(2, 1).Text; v != "NULL" {
		t.Errorf("NULL cell(2,1) = %q, want NULL", v)
	}
}

func TestFetchCancelled(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := sqliteCfg("demo", db)
	a := New(Options{Config: cfg, Store: secret.NewMemory()})
	defer a.closeConn()
	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled
	if _, err := fetch(ctx, a.conn, "select 1"); err == nil {
		t.Fatal("fetch with cancelled context should error")
	}
}

func TestClassifyErr(t *testing.T) {
	if got := classifyErr(context.Canceled); got == nil || got.Error() != "query cancelled" {
		t.Errorf("canceled → %v, want 'query cancelled'", got)
	}
	if got := classifyErr(context.DeadlineExceeded); got == nil || got.Error() != "query timed out" {
		t.Errorf("deadline → %v, want 'query timed out'", got)
	}
	plain := errors.New("boom")
	if got := classifyErr(plain); got != plain {
		t.Errorf("other error should pass through unchanged, got %v", got)
	}
}

func TestEscCancelsRunningQuery(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	cancelled := false
	a.running = true
	a.cancel = func() { cancelled = true }

	ev := a.onKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !cancelled {
		t.Fatal("Esc should cancel a running query")
	}
	if ev != nil {
		t.Error("Esc should be consumed while a query is running")
	}
}

func TestEscPassesThroughWhenIdle(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev == nil {
		t.Fatal("Esc should pass through when no query is running")
	}
}

func TestRecordHistory(t *testing.T) {
	h := tempHistory(t)
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory(), History: h})
	a.connID = "demo"

	a.recordHistory("select 1", 5_000_000, 1, nil)
	a.recordHistory("select bad", 0, 0, errors.New("boom"))

	entries, err := h.ListHistory(context.Background(), 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("history entries = %d, want 2", len(entries))
	}
	// newest first: the failing one
	if entries[0].Success || entries[0].ErrorMessage != "boom" {
		t.Errorf("expected failing entry first, got %+v", entries[0])
	}
}

func TestRecordHistoryDisabledIsNoop(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()}) // no History
	a.recordHistory("select 1", 0, 1, nil)                                          // must not panic
}

func TestShowHistoryAndUseSQL(t *testing.T) {
	h := tempHistory(t)
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory(), History: h})
	a.connID = "demo"
	a.recordHistory("select 42 as answer", 0, 1, nil)

	a.showHistory()
	if !a.historyOpen || !a.pages.HasPage("history") {
		t.Fatal("Ctrl-R should open the history overlay")
	}

	// Loading an entry fills the editor and closes the overlay.
	a.useHistorySQL("select 42 as answer")
	if a.historyOpen || a.pages.HasPage("history") {
		t.Fatal("loading a history entry should close the overlay")
	}
	if got := a.editor.GetText(); got != "select 42 as answer" {
		t.Errorf("editor text = %q, want the loaded SQL", got)
	}
	if a.focusIdx != 3 {
		t.Errorf("focus should move to the SQL editor (3), got %d", a.focusIdx)
	}
}

func TestShowHistoryDisabled(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()}) // no History
	a.showHistory()
	if a.historyOpen {
		t.Fatal("history overlay must not open when history is disabled")
	}
}

func TestSaveCurrent(t *testing.T) {
	h := tempHistory(t)
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory(), History: h})
	a.connID = "demo"
	a.editor.SetText("select * from orders where total > 100", false)

	a.saveCurrent()

	items, err := h.ListSaved(context.Background())
	if err != nil {
		t.Fatalf("list saved: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("saved queries = %d, want 1", len(items))
	}
	if items[0].SQL != "select * from orders where total > 100" {
		t.Errorf("saved SQL = %q", items[0].SQL)
	}
	if items[0].Title == "" || items[0].ConnectionID != "demo" {
		t.Errorf("saved meta unexpected: %+v", items[0])
	}
}

func TestSaveCurrentEmptyDoesNotSave(t *testing.T) {
	h := tempHistory(t)
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory(), History: h})
	a.editor.SetText("   ", false)
	a.saveCurrent()

	items, _ := h.ListSaved(context.Background())
	if len(items) != 0 {
		t.Fatalf("empty editor must not save, got %d", len(items))
	}
}

func TestShowSavedOverlay(t *testing.T) {
	h := tempHistory(t)
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory(), History: h})
	if _, err := h.SaveQuery(context.Background(), history.SavedQuery{Title: "recent", SQL: "select 1"}); err != nil {
		t.Fatal(err)
	}

	a.showSaved()
	if !a.savedOpen || !a.pages.HasPage("saved") {
		t.Fatal("Ctrl-F should open the saved overlay")
	}
	a.hideSaved()
	if a.savedOpen || a.pages.HasPage("saved") {
		t.Fatal("Esc/Ctrl-F should close the saved overlay")
	}
}

func TestSaveDisabledIsNoop(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()}) // no History
	a.editor.SetText("select 1", false)
	a.saveCurrent() // must not panic
	a.showSaved()
	if a.savedOpen {
		t.Fatal("saved overlay must not open when history is disabled")
	}
}

func TestTitleFrom(t *testing.T) {
	if got := titleFrom("  select\n  1  "); got != "select 1" {
		t.Errorf("titleFrom collapse = %q", got)
	}
	long := titleFrom("select aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if len([]rune(long)) > 50 {
		t.Errorf("titleFrom should cap at 50 runes, got %d", len([]rune(long)))
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := quoteIdent("mysql", "tab`le"); got != "`tab``le`" {
		t.Errorf("mysql quote = %q", got)
	}
	if got := quoteIdent("postgres", `ta"ble`); got != `"ta""ble"` {
		t.Errorf("postgres quote = %q", got)
	}
}

func TestFocusPanelCycle(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	if a.focusIdx != 0 {
		t.Fatalf("initial focusIdx = %d, want 0 (connections)", a.focusIdx)
	}
	a.focusPanel(1)
	if a.focusIdx != 1 || a.app.GetFocus() != a.schema {
		t.Fatal("focusPanel(1) should focus the schema tree")
	}
	a.focusPanel(2)
	if a.app.GetFocus() != a.results {
		t.Fatal("focusPanel(2) should focus the results table")
	}
}

func TestHelpToggle(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	if a.helpOpen || a.pages.HasPage("help") {
		t.Fatal("help should start closed")
	}
	// '?' opens help; any key closes it.
	a.onKey(tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone))
	if !a.helpOpen || !a.pages.HasPage("help") {
		t.Fatal("'?' should open the help overlay")
	}
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone)); ev != nil {
		t.Error("key should be consumed while dismissing help")
	}
	if a.helpOpen || a.pages.HasPage("help") {
		t.Fatal("any key should dismiss the help overlay")
	}
}

func TestRunEditorEmptyHint(t *testing.T) {
	// Empty editor must not start a query (no goroutine), just hint in status.
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.editor.SetText("   \n  ", false)
	a.runEditor()
	if a.running {
		t.Fatal("empty editor must not start a query")
	}
}

func TestEditorTypingPassesThrough(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.focusPanel(3) // focus the SQL editor

	// 'q' / '1' / '?' must NOT be treated as shortcuts while editing.
	for _, r := range []rune{'q', '1', '?'} {
		if ev := a.onKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)); ev == nil {
			t.Fatalf("rune %q should pass through to the editor, not be consumed", r)
		}
	}
	if a.helpOpen {
		t.Error("'?' while editing must not open help")
	}
}

func TestTabCyclesFocus(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.onKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if a.focusIdx != 1 {
		t.Fatalf("Tab should advance focus to 1, got %d", a.focusIdx)
	}
	a.onKey(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone))
	if a.focusIdx != 0 {
		t.Fatalf("Shift-Tab should return focus to 0, got %d", a.focusIdx)
	}
}
