// Package tui implements s9l's full-screen, lazygit-style terminal UI (Phase T).
//
// T-0 is the scaffold: a full-screen app with a placeholder layout that quits
// on q / Ctrl-C. The connection list, schema tree, results table and SQL editor
// land in later T tasks (see docs/TUI.md). tview wiring is kept thin and behind
// testing seams (SetScreen/OnReady/SendKey) so the app can be driven by a tcell
// SimulationScreen in tests without a real terminal.
package tui

import (
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Options configures the TUI.
type Options struct {
	// Conn is an optional connection id/DSN to auto-open (used from T-1a).
	Conn string
}

// App is the s9l TUI application.
type App struct {
	app    *tview.Application
	root   *tview.Flex
	status *tview.TextView
	ready  sync.Once
}

// New builds the TUI application and its root layout.
func New(opts Options) *App {
	a := &App{app: tview.NewApplication()}

	body := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("s9l — terminal database client\n\n" +
			"TUI scaffold (Phase T): connections, schema tree, results and\n" +
			"the SQL editor arrive in upcoming slices.")
	body.SetBorder(true).SetTitle(" s9l ")

	a.status = tview.NewTextView().SetDynamicColors(true)
	a.SetStatus("[::b]?[::-] help   [::b]q[::-] quit")

	a.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 0, 1, true).
		AddItem(a.status, 1, 0, false)

	a.app.SetRoot(a.root, true).EnableMouse(true)
	a.app.SetInputCapture(a.onKey)
	return a
}

// onKey handles global keys. Panel-local navigation is added in later slices.
func (a *App) onKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyCtrlC || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q') {
		a.app.Stop()
		return nil
	}
	return ev
}

// SetStatus updates the bottom status/help bar (supports tview color tags).
func (a *App) SetStatus(s string) { a.status.SetText(" " + s) }

// Run starts the UI loop and blocks until the user quits.
func (a *App) Run() error { return a.app.Run() }

// Stop terminates the UI loop.
func (a *App) Stop() { a.app.Stop() }

// --- testing seams ---

// SetScreen injects a screen (e.g. a tcell SimulationScreen) before Run.
func (a *App) SetScreen(s tcell.Screen) { a.app.SetScreen(s) }

// OnReady registers fn to run exactly once after the first draw, letting tests
// drive the app deterministically without sleeps.
func (a *App) OnReady(fn func()) {
	a.app.SetAfterDrawFunc(func(tcell.Screen) {
		a.ready.Do(fn)
	})
}

// SendKey queues a rune key event (testing helper).
func (a *App) SendKey(r rune) {
	a.app.QueueEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}
