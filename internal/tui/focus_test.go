package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// A mouse click focuses a panel directly through tview (bypassing focusPanel);
// syncFocus must still update focusIdx and the borders, or every panel-scoped
// key (v/f/c/Enter/]/[…) acts on the previously focused panel.
func TestMouseFocusSyncsPanelState(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.focusPanel(0)

	a.app.SetFocus(a.results) // as a mouse click on Results would
	if a.focusIdx != 2 {
		t.Fatalf("focusIdx = %d after direct SetFocus, want 2", a.focusIdx)
	}
	if a.results.GetBorderColor() != a.theme.Focus {
		t.Error("Results border should show focus after a mouse click")
	}
	if a.connTree.GetBorderColor() != a.theme.Border {
		t.Error("Connections border should drop focus after a mouse click elsewhere")
	}
}

// Closing an overlay must give focus back to the panel it was opened from:
// tview transiently refocuses the root (Connections) while the overlay page is
// removed, and syncFocus must ignore that.
func TestOverlayCloseKeepsPanelFocus(t *testing.T) {
	a := previewApp()
	a.focusPanel(2)
	a.showFilter()
	a.hideFilter(false)
	if a.focusIdx != 2 {
		t.Errorf("focusIdx = %d after closing the filter, want 2 (Results)", a.focusIdx)
	}
}

// ] pages the preview when Results got focus by any means.
func TestPagingKeyAfterDirectFocus(t *testing.T) {
	a := previewApp()
	a.lastData = make([][]any, resultLimit) // full page → next allowed
	a.app.SetFocus(a.results)

	if ev := a.onKey(tcell.NewEventKey(tcell.KeyRune, ']', tcell.ModNone)); ev != nil {
		t.Error("] should be consumed on the Results panel")
	}
	if a.resultPage != 1 {
		t.Errorf("] → page %d, want 1", a.resultPage)
	}
}

// Enter on a Results cell opens the same edit input as c (single-table preview
// only); outside a preview it only reports why, without opening.
func TestEnterOpensCellEdit(t *testing.T) {
	a := previewApp()
	a.setResults([]string{"id", "name"}, [][]any{{int64(1), "x"}})
	a.focusPanel(2)

	if ev := a.onKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); ev != nil {
		t.Error("Enter should be consumed on the Results panel")
	}
	if !a.cellEditOpen {
		t.Fatal("Enter on a preview cell should open the edit input")
	}
	a.hideCellEdit()

	// Arbitrary result: not editable, no overlay.
	b := New(Options{Config: sqliteCfg("demo", "x.db")})
	b.setResults([]string{"id"}, [][]any{{int64(1)}})
	b.focusPanel(2)
	b.onKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if b.cellEditOpen {
		t.Error("Enter must not open the editor outside a single-table preview")
	}
}
