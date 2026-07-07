package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/gdamore/tcell/v2"
)

// editorApp returns an app with a Metadata-backed completer and the editor
// focused, ready for completion tests.
func editorApp() *App {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.conn = &fakeMetaConn{
		tables: []string{"users", "orders", "名簿"},
		cols:   map[string][]string{"users": {"id", "last_name"}},
	}
	a.buildCompleter("")
	a.focusPanel(3)
	return a
}

func TestEditorCompletionAutoOpensAndAdopts(t *testing.T) {
	a := editorApp()

	// Typing "us" (≥2 runes) auto-opens the popup with the table candidate.
	a.editor.SetText("select * from us", true) // fires the changed hook
	if !a.completionOpen {
		t.Fatal("popup should auto-open for a 2-rune word")
	}
	found := false
	for _, c := range a.compCands {
		if strings.EqualFold(c, "users") {
			found = true
		}
	}
	if !found {
		t.Fatalf("candidates = %v, want to include users", a.compCands)
	}

	// Tab adopts the selected candidate and closes the popup.
	for !strings.EqualFold(a.compCands[a.compList.GetCurrentItem()], "users") {
		a.onKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	}
	a.onKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if a.completionOpen {
		t.Error("popup should close after adopting")
	}
	got := a.editor.GetText()
	if !strings.EqualFold(got, "select * from users") {
		t.Errorf("editor text = %q, want the adopted table name", got)
	}
	// The cursor lands at the end of the adopted word.
	if _, cur, _ := a.editor.GetSelection(); cur != len(got) {
		t.Errorf("cursor at byte %d after adopting, want %d (word end)", cur, len(got))
	}
}

// The arrow keys have two states: with the popup open they move its selection
// (both directions); with it closed they pass through to the editor cursor.
func TestEditorCompletionArrowRouting(t *testing.T) {
	a := editorApp()
	a.editor.SetText("select * from o", true) // below threshold: popup closed
	if a.completionOpen {
		t.Fatal("popup unexpectedly open")
	}
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)); ev == nil {
		t.Error("Down with the popup closed must pass through to the editor")
	}
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)); ev == nil {
		t.Error("Up with the popup closed must pass through to the editor")
	}

	a.editor.SetText("select * from or", true) // or → OR / ORDER BY / orders…
	if !a.completionOpen || len(a.compCands) < 2 {
		t.Fatalf("popup with ≥2 candidates expected, got open=%v cands=%v", a.completionOpen, a.compCands)
	}
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)); ev != nil {
		t.Error("Down with the popup open must be consumed")
	}
	if got := a.compList.GetCurrentItem(); got != 1 {
		t.Errorf("selection after Down = %d, want 1", got)
	}
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)); ev != nil {
		t.Error("Up with the popup open must be consumed")
	}
	if got := a.compList.GetCurrentItem(); got != 0 {
		t.Errorf("selection after Up = %d, want 0", got)
	}
}

// Multi-byte safety (plan R3): completing a CJK table name must replace the
// right bytes.
func TestEditorCompletionMultibyte(t *testing.T) {
	a := editorApp()
	a.editor.SetText("select * from 名", true)
	a.updateEditorCompletion(false) // manual: below the auto threshold
	if !a.completionOpen {
		t.Fatal("manual completion should open for 名")
	}
	a.onKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if got := a.editor.GetText(); got != "select * from 名簿" {
		t.Errorf("editor text = %q, want %q", got, "select * from 名簿")
	}
}

func TestEditorCompletionEscAndShortWords(t *testing.T) {
	a := editorApp()

	// One rune: no auto popup.
	a.editor.SetText("select * from u", true)
	if a.completionOpen {
		t.Error("popup must not auto-open below the threshold")
	}

	// Esc closes the popup without touching the text.
	a.editor.SetText("select * from us", true)
	if !a.completionOpen {
		t.Fatal("popup expected")
	}
	if ev := a.onKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); ev != nil {
		t.Error("Esc should be consumed by the popup")
	}
	if a.completionOpen {
		t.Error("Esc should close the popup")
	}
	if got := a.editor.GetText(); got != "select * from us" {
		t.Errorf("Esc must not change the text, got %q", got)
	}

	// F5 with the popup open: closes it and runs the SQL as typed — it must
	// not adopt a candidate into the text.
	a.editor.SetText("select * from ord", true)
	if !a.completionOpen {
		t.Fatal("popup expected")
	}
	a.onKey(tcell.NewEventKey(tcell.KeyF5, 0, tcell.ModNone))
	if a.completionOpen {
		t.Error("F5 should close the popup")
	}
	if got := a.editor.GetText(); got != "select * from ord" {
		t.Errorf("F5 must not adopt a candidate, text = %q", got)
	}
	if !a.running {
		t.Error("F5 should run the editor SQL (query running)")
	}
}

// The popup follows the editor: it must not appear when the editor is not
// focused (e.g. history loads SQL into it), and it closes when focus leaves.
func TestEditorCompletionFollowsFocus(t *testing.T) {
	a := editorApp()
	a.focusPanel(2)
	a.editor.SetText("select * from us", true)
	if a.completionOpen {
		t.Error("popup must not open while the editor is unfocused")
	}

	a.focusPanel(3)
	a.editor.SetText("select * from ord", true)
	if !a.completionOpen {
		t.Fatal("popup expected")
	}
	a.focusPanel(2) // leaving the editor closes the popup
	if a.completionOpen {
		t.Error("popup should close when the editor loses focus")
	}
}

// Keystroke smoke test through the real event loop: connect to SQLite, focus
// the editor with '4', type a partial table name, adopt the completion with
// Tab, and check the editor text — the whole pipeline a user drives.
func TestEditorCompletionKeystrokeE2E(t *testing.T) {
	db := filepath.Join(t.TempDir(), "e.db")
	seed, err := driver.Open(context.Background(), "sqlite", db)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := seed.Exec(context.Background(), "create table widgets(id integer)"); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()

	a := New(Options{Conn: "demo", Config: sqliteCfg("demo", db), Store: secret.NewMemory()})
	sim := tcell.NewSimulationScreen("")
	a.SetScreen(sim)
	ready := make(chan struct{})
	a.OnReady(func() { close(ready) })
	runErr := make(chan error, 1)
	go func() { runErr <- a.Run() }()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		a.Stop()
		t.Fatal("not ready in 10s")
	}

	waitFor := func(desc string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			ok := false
			done := make(chan struct{})
			a.app.QueueUpdate(func() { ok = cond(); close(done) })
			<-done
			if ok {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("timeout waiting for %s", desc)
	}

	sim.InjectKey(tcell.KeyRune, '4', tcell.ModNone) // focus the SQL editor
	waitFor("editor focus", func() bool { return a.focusIdx == 3 })
	for _, r := range "select * from wid" {
		sim.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
	waitFor("completion popup", func() bool { return a.completionOpen })
	sim.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitFor("adoption", func() bool {
		return strings.EqualFold(a.editor.GetText(), "select * from widgets")
	})

	a.Stop()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// Without a completer (not connected) typing must not panic or open anything.
func TestEditorCompletionNoCompleter(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.focusPanel(3)
	a.editor.SetText("select * from us", true)
	if a.completionOpen {
		t.Error("no completer → no popup")
	}
}
