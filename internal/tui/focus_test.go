package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/secret"

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

// A CJK input method emits full-width forms for the bracket keys; those (and
// the shifted >/< mnemonics) must page too, or paging looks dead whenever the
// IME is on.
func TestPagingKeyAliases(t *testing.T) {
	a := previewApp()
	a.lastData = make([][]any, resultLimit)
	a.app.SetFocus(a.results)

	for i, r := range []rune{'］', '>', '＞'} {
		a.onKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		if a.resultPage != i+1 {
			t.Fatalf("%q → page %d, want %d", r, a.resultPage, i+1)
		}
	}
	for i, r := range []rune{'［', '<', '＜'} {
		a.onKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		if want := 2 - i; a.resultPage != want {
			t.Fatalf("%q → page %d, want %d", r, a.resultPage, want)
		}
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

// The running build must stay identifiable: the startup status names it, and
// the always-visible keybar keeps naming it even after auto-connect (or any
// later action) rewrites the status line.
func TestVersionVisibleInStatusAndKeybar(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Version: "dev-abc1234"})
	if got := a.status.GetText(false); !strings.Contains(got, "s9l dev-abc1234") {
		t.Errorf("startup status = %q, want it to contain the version", got)
	}
	if got := a.keybar.GetText(false); !strings.Contains(got, "s9l dev-abc1234") {
		t.Errorf("keybar = %q, want it to contain the version", got)
	}

	// Auto-connect overwrites the status line ("connected: …"), but the keybar
	// still names the build — the scenario that motivated this feature.
	db := filepath.Join(t.TempDir(), "v.db")
	b := New(Options{Conn: "demo", Config: sqliteCfg("demo", db), Store: secret.NewMemory(), Version: "dev-abc1234"})
	defer b.closeConn()
	if got := b.status.GetText(false); strings.Contains(got, "dev-abc1234") {
		t.Logf("status after auto-connect still shows version: %q (fine, not required)", got)
	}
	if got := b.keybar.GetText(false); !strings.Contains(got, "s9l dev-abc1234") {
		t.Errorf("keybar after auto-connect = %q, want it to keep the version", got)
	}
}
