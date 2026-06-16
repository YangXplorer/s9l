package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"

	"github.com/gdamore/tcell/v2"
)

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

func TestRunTableQueryFillsResults(t *testing.T) {
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

	a.runTableQuery("t")

	// Header row + 2 data rows.
	if got := a.results.GetRowCount(); got != 3 {
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
