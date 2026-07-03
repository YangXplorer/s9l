package tui

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"

	"github.com/rivo/tview"
)

func TestPreviewQuery(t *testing.T) {
	cases := []struct {
		name, driver, where string
		limit, offset       int
		want                string
	}{
		{"mysql first page", "mysql", "", 200, 0, "SELECT * FROM `t` LIMIT 200"},
		{"sqlite first page", "sqlite", "", 50, 0, `SELECT * FROM "t" LIMIT 50`},
		{"offset paging", "postgres", "", 200, 400, `SELECT * FROM "t" LIMIT 200 OFFSET 400`},
		{"where", "mysql", "age > 20 AND name LIKE 'a%'", 200, 0,
			"SELECT * FROM `t` WHERE age > 20 AND name LIKE 'a%' LIMIT 200"},
		{"where + offset", "sqlite", "id > 5", 100, 100,
			`SELECT * FROM "t" WHERE id > 5 LIMIT 100 OFFSET 100`},
		// SQL Server has no LIMIT — TOP on the first page, OFFSET…FETCH after.
		{"sqlserver first page", "sqlserver", "", 200, 0, `SELECT TOP 200 * FROM "t"`},
		{"sqlserver where", "sqlserver", "x = 1", 200, 0, `SELECT TOP 200 * FROM "t" WHERE x = 1`},
		{"sqlserver paging", "sqlserver", "x = 1", 200, 200,
			`SELECT * FROM "t" WHERE x = 1 ORDER BY (SELECT NULL) OFFSET 200 ROWS FETCH NEXT 200 ROWS ONLY`},
	}
	for _, c := range cases {
		q := "`t`"
		if c.driver != "mysql" {
			q = `"t"`
		}
		if got := previewQuery(c.driver, q, c.where, c.limit, c.offset); got != c.want {
			t.Errorf("%s:\n  got  %q\n  want %q", c.name, got, c.want)
		}
	}
}

func TestQualifyTable(t *testing.T) {
	if got := qualifyTable("mysql", tableRef{name: "users"}); got != "`users`" {
		t.Errorf("plain = %q", got)
	}
	if got := qualifyTable("mysql", tableRef{db: "app", name: "users"}); got != "`app`.`users`" {
		t.Errorf("qualified = %q", got)
	}
	if got := qualifyTable("postgres", tableRef{name: "t"}); got != `"t"` {
		t.Errorf("postgres = %q", got)
	}
}

// --- fake conn implementing driver.Conn + Metadata + databaseBrowser ---

type fakeRows struct {
	data [][]any
	i    int
}

func (r *fakeRows) Columns() []string { return []string{"name"} }
func (r *fakeRows) Next() bool        { r.i++; return r.i <= len(r.data) }
func (r *fakeRows) Values() ([]any, error) {
	return r.data[r.i-1], nil
}
func (r *fakeRows) Err() error   { return nil }
func (r *fakeRows) Close() error { return nil }

func nameRows(names []string) driver.Rows {
	data := make([][]any, len(names))
	for i, n := range names {
		data[i] = []any{n}
	}
	return &fakeRows{data: data}
}

type fakeBrowserConn struct {
	dbs map[string][]string // database -> tables
}

func (c *fakeBrowserConn) Query(context.Context, string, ...any) (driver.Rows, error) {
	return nameRows(nil), nil
}
func (c *fakeBrowserConn) Exec(context.Context, string, ...any) (driver.Result, error) {
	return nil, nil
}
func (c *fakeBrowserConn) Close() error { return nil }
func (c *fakeBrowserConn) Databases(context.Context) (driver.Rows, error) {
	names := make([]string, 0, len(c.dbs))
	for db := range c.dbs {
		names = append(names, db)
	}
	sort.Strings(names)
	return nameRows(names), nil
}
func (c *fakeBrowserConn) Tables(context.Context) (driver.Rows, error) { return nameRows(nil), nil }
func (c *fakeBrowserConn) Columns(context.Context, string) (driver.Rows, error) {
	return nameRows(nil), nil
}
func (c *fakeBrowserConn) TablesIn(_ context.Context, db string) (driver.Rows, error) {
	return nameRows(c.dbs[db]), nil
}

func newBrowserApp() *App {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.conn = &fakeBrowserConn{dbs: map[string][]string{
		"app":  {"users", "orders"},
		"logs": {"events"},
	}}
	a.driverName = "mysql"
	a.connID = "my"
	return a
}

// setConnNodeLabel adds an expand indicator only once a connection has database
// children, and toggles ▾/▸ with the expanded state.
func TestConnNodeExpandIndicator(t *testing.T) {
	a := newBrowserApp()
	cc := config.ConnectionConfig{ID: "my", Driver: "mysql"}
	node := a.treeNode("").SetReference(connNodeRef{cc: cc})

	a.setConnNodeLabel(node, cc)
	if strings.HasPrefix(node.GetText(), "▾") || strings.HasPrefix(node.GetText(), "▸") {
		t.Errorf("no children → no triangle, got %q", node.GetText())
	}

	a.loadConnDatabases(node, cc) // adds db children
	node.SetExpanded(true)
	a.setConnNodeLabel(node, cc)
	if !strings.HasPrefix(node.GetText(), "▾ ") {
		t.Errorf("expanded with children → ▾, got %q", node.GetText())
	}
	node.SetExpanded(false)
	a.setConnNodeLabel(node, cc)
	if !strings.HasPrefix(node.GetText(), "▸ ") {
		t.Errorf("collapsed with children → ▸, got %q", node.GetText())
	}
}

// Connections tree: a connected multi-database engine lists its databases as
// child nodes (each a dbNodeRef), sorted.
func TestLoadConnDatabases(t *testing.T) {
	a := newBrowserApp()
	node := tview.NewTreeNode("my")
	a.loadConnDatabases(node, config.ConnectionConfig{ID: "my", Driver: "mysql"})

	kids := node.GetChildren()
	if len(kids) != 2 {
		t.Fatalf("got %d database nodes, want 2 (app, logs)", len(kids))
	}
	ref, ok := kids[0].GetReference().(dbNodeRef)
	if !ok || ref.db != "app" || ref.connID != "my" {
		t.Fatalf("first db node ref = %+v, want dbNodeRef{my, app}", kids[0].GetReference())
	}
	// Database nodes carry no Accent color (no "colored tree" look); the expand
	// toggle lives on the parent connection node instead.
	if got := kids[0].GetColor(); got == a.theme.Accent {
		t.Errorf("db node color = %v, want non-Accent (plain) so it doesn't read as a colored tree", got)
	}
	if a.currentDB != "" {
		t.Errorf("currentDB = %q after listing databases, want empty until one is picked", a.currentDB)
	}
}

func TestFilterTables(t *testing.T) {
	names := []string{"users", "orders", "order_items", "products"}
	if got := filterTables(names, ""); len(got) != 4 {
		t.Errorf("empty term should return all, got %d", len(got))
	}
	// Case-insensitive substring across the list.
	got := filterTables(names, "ORDER")
	if len(got) != 2 || got[0] != "orders" || got[1] != "order_items" {
		t.Errorf("filter ORDER = %v, want [orders order_items]", got)
	}
	if got := filterTables(names, "zzz"); len(got) != 0 {
		t.Errorf("no match should be empty, got %v", got)
	}
}

func TestApplySchemaFilterRerenders(t *testing.T) {
	a := newBrowserApp()
	a.currentDB = "app"
	a.loadSchema() // app → users, orders
	if got := len(a.schema.GetRoot().GetChildren()); got != 2 {
		t.Fatalf("schema tables = %d, want 2", got)
	}
	a.applySchemaFilter("user")
	kids := a.schema.GetRoot().GetChildren()
	if len(kids) != 1 || kids[0].GetText() != "users" {
		t.Fatalf("after filter user = %v, want [users]", nodeTexts(kids))
	}
	// Clearing restores the full list.
	a.applySchemaFilter("")
	if got := len(a.schema.GetRoot().GetChildren()); got != 2 {
		t.Errorf("after clear = %d, want 2", got)
	}
}

func nodeTexts(nodes []*tview.TreeNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.GetText()
	}
	return out
}

// Schema panel: with a database picked (currentDB), it lists that database's
// tables (via TablesIn), each a db-qualified tableRef.
func TestLoadSchemaForCurrentDB(t *testing.T) {
	a := newBrowserApp()
	a.currentDB = "app"
	a.loadSchema()

	kids := a.schema.GetRoot().GetChildren()
	if len(kids) != 2 {
		t.Fatalf("schema tables = %d, want 2 (users, orders)", len(kids))
	}
	ref, ok := kids[0].GetReference().(tableRef)
	if !ok || ref.db != "app" || ref.name != "users" {
		t.Fatalf("table ref = %+v, want tableRef{app, users}", kids[0].GetReference())
	}
}
