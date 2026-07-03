package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/gdamore/tcell/v2"
)

// previewApp returns an app marked as showing a single-table preview, with no
// real connection: runQuery bails out synchronously ("not connected"), which
// lets the preview state machine be asserted without the event loop.
func previewApp() *App {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.driverName = "sqlite"
	a.resultTable = tableRef{name: "t"}
	a.resultEditable = true
	return a
}

func TestApplyWhereSetsStateAndResetsPage(t *testing.T) {
	a := previewApp()
	a.resultPage = 3

	a.applyWhere("id > 10")
	if a.resultWhere != "id > 10" || a.resultPage != 0 {
		t.Errorf("where=%q page=%d, want %q page 0", a.resultWhere, a.resultPage, "id > 10")
	}
	title := a.results.GetTitle()
	if !strings.Contains(title, "WHERE id > 10") || !strings.Contains(title, "t") {
		t.Errorf("title %q should show table and WHERE", title)
	}

	a.applyWhere("")
	if a.resultWhere != "" {
		t.Errorf("cleared where = %q, want empty", a.resultWhere)
	}
	if strings.Contains(a.results.GetTitle(), "WHERE") {
		t.Errorf("title %q should drop WHERE after clear", a.results.GetTitle())
	}
}

func TestApplyWhereGuards(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.applyWhere("id > 1") // not a preview → no-op
	if a.resultWhere != "" {
		t.Errorf("where applied outside a preview: %q", a.resultWhere)
	}

	a = previewApp()
	a.resultWhere = "id > 1"
	a.resultPage = 2
	a.applyWhere("id > 1") // unchanged → no re-query, page kept
	if a.resultPage != 2 {
		t.Errorf("unchanged WHERE reset page to %d", a.resultPage)
	}
}

func TestPagingBounds(t *testing.T) {
	a := previewApp()

	a.prevPage() // already at the first page
	if a.resultPage != 0 {
		t.Errorf("prevPage at 0 → page %d, want 0", a.resultPage)
	}

	a.lastData = make([][]any, 5) // short page → no further pages
	a.nextPage()
	if a.resultPage != 0 {
		t.Errorf("nextPage on short page → page %d, want 0", a.resultPage)
	}

	a.lastData = make([][]any, resultLimit) // full page → next allowed
	a.nextPage()
	if a.resultPage != 1 {
		t.Errorf("nextPage on full page → page %d, want 1", a.resultPage)
	}
	if !strings.Contains(a.results.GetTitle(), "page 2") {
		t.Errorf("title %q should show page 2", a.results.GetTitle())
	}

	a.prevPage()
	if a.resultPage != 0 {
		t.Errorf("prevPage → page %d, want 0", a.resultPage)
	}

	// Outside a preview, paging is a no-op.
	a.resultTable = tableRef{}
	a.lastData = make([][]any, resultLimit)
	a.nextPage()
	if a.resultPage != 0 {
		t.Errorf("paging outside a preview → page %d, want 0", a.resultPage)
	}
}

// The / filter on a table preview opens the WHERE input (applied on Enter, not
// per keystroke); on an arbitrary result it stays the client-side fuzzy filter.
func TestShowFilterPreviewUsesWhere(t *testing.T) {
	a := previewApp()
	a.focusPanel(2)
	a.showFilter()
	if !a.filterOpen || a.filterTarget != filterTgtResultsWhere {
		t.Fatalf("preview / → filterOpen=%v target=%v, want WHERE input", a.filterOpen, a.filterTarget)
	}

	a.pendingWhere = "name = 'x'"
	a.hideFilter(false) // Enter applies
	if a.resultWhere != "name = 'x'" {
		t.Errorf("Enter applied where = %q", a.resultWhere)
	}

	a.showFilter()
	a.hideFilter(true) // Esc clears
	if a.resultWhere != "" {
		t.Errorf("Esc left where = %q", a.resultWhere)
	}
}

func TestRunQueryClearsPreviewState(t *testing.T) {
	a := previewApp()
	a.conn = &fakeBrowserConn{}
	a.resultWhere = "id > 1"
	a.resultPage = 2
	a.setResultsTitle()

	a.runQuery("SELECT 1") // arbitrary SQL is not a preview
	if a.resultTable.name != "" || a.resultWhere != "" || a.resultPage != 0 {
		t.Errorf("preview state not cleared: table=%q where=%q page=%d",
			a.resultTable.name, a.resultWhere, a.resultPage)
	}
	if got := a.results.GetTitle(); strings.Contains(got, "WHERE") || strings.Contains(got, "page") {
		t.Errorf("title %q should reset for arbitrary SQL", got)
	}
}

// End-to-end on SQLite through the real event loop: preview a 250-row table,
// page forward ([ ]), then filter server-side with a WHERE expression.
func TestPreviewWherePagingE2E(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	seed, err := driver.Open(context.Background(), "sqlite", db)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if _, err := seed.Exec(context.Background(), "create table nums(id integer primary key)"); err != nil {
		t.Fatal(err)
	}
	var ins strings.Builder
	ins.WriteString("insert into nums(id) values (1)")
	for i := 2; i <= 250; i++ {
		fmt.Fprintf(&ins, ",(%d)", i)
	}
	if _, err := seed.Exec(context.Background(), ins.String()); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()

	a := New(Options{Conn: "demo", Config: sqliteCfg("demo", db), Store: secret.NewMemory()})
	a.SetScreen(tcell.NewSimulationScreen(""))

	done := make(chan struct{}, 1)
	a.onResult = func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
	a.OnReady(func() {
		if kids := a.schema.GetRoot().GetChildren(); len(kids) > 0 {
			a.onSchemaSelect(kids[0]) // preview page 0
		}
	})

	runErr := make(chan error, 1)
	go func() { runErr <- a.Run() }()

	wait := func(step string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			a.Stop()
			t.Fatalf("%s did not complete within 10s", step)
		}
	}
	// snapshot reads UI state on the UI goroutine to avoid races.
	type snap struct {
		rows, page int
		where      string
	}
	snapshot := func() snap {
		ch := make(chan snap, 1)
		a.app.QueueUpdateDraw(func() {
			ch <- snap{rows: len(a.lastData), page: a.resultPage, where: a.resultWhere}
		})
		return <-ch
	}

	wait("preview")
	if s := snapshot(); s.rows != resultLimit || s.page != 0 {
		t.Errorf("page 0: rows=%d page=%d, want %d/0", s.rows, s.page, resultLimit)
	}

	a.app.QueueUpdateDraw(func() { a.nextPage() })
	wait("next page")
	if s := snapshot(); s.rows != 50 || s.page != 1 {
		t.Errorf("page 1: rows=%d page=%d, want 50/1", s.rows, s.page)
	}

	a.app.QueueUpdateDraw(func() { a.applyWhere("id <= 10") })
	wait("where")
	if s := snapshot(); s.rows != 10 || s.page != 0 || s.where != "id <= 10" {
		t.Errorf("where: rows=%d page=%d where=%q, want 10/0/%q", s.rows, s.page, s.where, "id <= 10")
	}

	a.Stop()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
