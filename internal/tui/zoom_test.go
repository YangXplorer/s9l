package tui

import (
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/config"

	"github.com/gdamore/tcell/v2"
)

// F6 grows the SQL editor to most of the right column and back; the zoom must
// survive re-renders (setResults) because the layout is resized, not rebuilt.
func TestEditorZoomToggle(t *testing.T) {
	a := New(Options{Config: &config.Config{}})
	a.SetScreen(tcell.NewSimulationScreen(""))

	ready := make(chan struct{})
	a.OnReady(func() { close(ready) })
	runErr := make(chan error, 1)
	go func() { runErr <- a.Run() }()

	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		a.Stop()
		t.Fatal("TUI did not become ready within 10s")
	}

	// ui runs fn on the UI goroutine (with a redraw) and waits for it.
	ui := func(fn func()) {
		t.Helper()
		done := make(chan struct{})
		a.app.QueueUpdateDraw(func() { fn(); close(done) })
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			a.Stop()
			t.Fatal("UI update did not run within 10s")
		}
	}
	editorHeightNow := func() (h int) {
		ui(func() { _, _, _, h = a.editor.GetRect() })
		return h
	}
	pressF6 := func() {
		ui(func() { a.onKey(tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone)) })
	}

	normal := editorHeightNow()
	pressF6()
	zoomed := editorHeightNow()
	if zoomed <= normal {
		t.Errorf("zoomed editor height = %d, want > normal %d", zoomed, normal)
	}
	ui(func() {
		if !a.editorZoomed {
			t.Error("editorZoomed should be true after F6")
		}
		a.setResults([]string{"id"}, [][]any{{1}}) // re-render must not reset the zoom
		if !a.editorZoomed {
			t.Error("zoom must survive a result re-render")
		}
	})
	if h := editorHeightNow(); h != zoomed {
		t.Errorf("editor height after re-render = %d, want %d (zoom kept)", h, zoomed)
	}

	pressF6()
	if h := editorHeightNow(); h != normal {
		t.Errorf("restored editor height = %d, want %d", h, normal)
	}

	a.Stop()
	if err := <-runErr; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
