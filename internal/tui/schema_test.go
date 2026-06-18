package tui

import (
	"context"
	"sort"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
)

func TestPreviewQuery(t *testing.T) {
	if got := previewQuery("mysql", "`t`", 200); got != "SELECT * FROM `t` LIMIT 200" {
		t.Errorf("mysql preview = %q", got)
	}
	if got := previewQuery("sqlite", `"t"`, 50); got != `SELECT * FROM "t" LIMIT 50` {
		t.Errorf("sqlite preview = %q", got)
	}
	// SQL Server has no LIMIT — it must use TOP.
	if got := previewQuery("sqlserver", `"t"`, 200); got != `SELECT TOP 200 * FROM "t"` {
		t.Errorf("sqlserver preview = %q", got)
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
	return nil, nil
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

func TestLoadSchemaMultiDB(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.conn = &fakeBrowserConn{dbs: map[string][]string{
		"app":  {"users", "orders"},
		"logs": {"events"},
	}}
	a.driverName = "mysql"
	a.connID = "my"

	a.loadSchema()
	dbNodes := a.schema.GetRoot().GetChildren()
	if len(dbNodes) != 2 {
		t.Fatalf("got %d database nodes, want 2", len(dbNodes))
	}
	// Sorted: app, logs. Database nodes carry dbRef and start collapsed/empty.
	app := dbNodes[0]
	if ref, ok := app.GetReference().(dbRef); !ok || ref.name != "app" {
		t.Fatalf("first node ref = %+v, want dbRef{app}", app.GetReference())
	}
	if len(app.GetChildren()) != 0 {
		t.Error("database node should start without loaded tables")
	}

	// Expanding "app" lazy-loads its tables, qualified with the database.
	a.onSchemaSelect(app)
	kids := app.GetChildren()
	if len(kids) != 2 {
		t.Fatalf("app expanded to %d tables, want 2", len(kids))
	}
	ref, ok := kids[0].GetReference().(tableRef)
	if !ok || ref.db != "app" || ref.name != "users" {
		t.Fatalf("table ref = %+v, want tableRef{app, users}", kids[0].GetReference())
	}
}
