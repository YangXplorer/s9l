// Package tui implements s9l's full-screen, lazygit-style terminal UI (Phase T).
//
// It is a thin presentation layer over the existing driver/config/secret/history
// packages — the core is not modified. tview wiring is kept behind testing seams
// (SetScreen/OnReady/SendKey) so the app can be driven by a tcell SimulationScreen
// in tests, and connection logic is exercised white-box without the event loop.
// See docs/TUI.md.
package tui

import (
	"context"
	"fmt"
	"sync"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const sidebarWidth = 30

// Options configures the TUI.
type Options struct {
	// Conn is an optional connection id to auto-open on launch.
	Conn string
	// Config is the loaded configuration. If nil, New loads it via config.Load.
	Config *config.Config
	// Store resolves password references. If nil, an in-memory store is used.
	Store secret.SecretStore
}

// App is the s9l TUI application.
type App struct {
	app *tview.Application

	cfg   *config.Config
	store secret.SecretStore

	connList *tview.List
	schema   *tview.TreeView
	results  *tview.Table
	editor   *tview.TextView
	status   *tview.TextView

	conn   driver.Conn
	connID string

	ready sync.Once
}

// New builds the TUI application and its layout, populating the connection list.
func New(opts Options) *App {
	a := &App{
		app:   tview.NewApplication(),
		cfg:   opts.Config,
		store: opts.Store,
	}
	if a.cfg == nil {
		if cfg, err := config.Load(); err == nil {
			a.cfg = cfg
		} else {
			a.cfg = &config.Config{}
		}
	}
	if a.store == nil {
		a.store = secret.NewMemory()
	}

	a.buildLayout()
	a.populateConnections()

	a.app.SetInputCapture(a.onKey)

	if opts.Conn != "" {
		if cc, ok := a.cfg.Get(opts.Conn); ok {
			_ = a.connect(cc)
		}
	}
	return a
}

func (a *App) buildLayout() {
	a.connList = tview.NewList().ShowSecondaryText(true)
	a.connList.SetBorder(true).SetTitle(" Connections ")

	a.schema = tview.NewTreeView()
	a.schema.SetBorder(true).SetTitle(" Schema ")

	a.results = tview.NewTable().SetBorders(false).SetFixed(1, 0)
	a.results.SetBorder(true).SetTitle(" Results ")

	a.editor = tview.NewTextView()
	a.editor.SetBorder(true).SetTitle(" SQL ")

	a.status = tview.NewTextView().SetDynamicColors(true)
	a.SetStatus(defaultStatus)

	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.connList, 0, 1, true).
		AddItem(a.schema, 0, 1, false)
	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.results, 0, 1, false).
		AddItem(a.editor, 6, 0, false)
	body := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, sidebarWidth, 0, true).
		AddItem(right, 0, 1, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 0, 1, true).
		AddItem(a.status, 1, 0, false)

	a.app.SetRoot(root, true).EnableMouse(true)
}

// populateConnections fills the list from config; Enter connects.
func (a *App) populateConnections() {
	a.connList.Clear()
	for _, cc := range a.cfg.Connections {
		a.connList.AddItem(cc.ID, connSummary(cc), 0, func() {
			_ = a.connect(cc)
		})
	}
	if a.connList.GetItemCount() == 0 {
		a.connList.AddItem("(no connections)", "add one with: s9l conn add", 0, nil)
	}
}

// connect resolves the connection's password, opens it, and updates status.
// Errors are surfaced in the status bar; the UI never crashes on a bad connect.
func (a *App) connect(cc config.ConnectionConfig) error {
	password, err := secret.Resolve(a.store, cc.PasswordRef)
	if err != nil {
		a.setError(fmt.Sprintf("connection %q: %v", cc.ID, err))
		return err
	}
	dsn, err := cc.DSN(password)
	if err != nil {
		a.setError(err.Error())
		return err
	}
	conn, err := driver.Open(context.Background(), cc.Driver, dsn)
	if err != nil {
		a.setError(err.Error())
		return err
	}

	if a.conn != nil {
		_ = a.conn.Close()
	}
	a.conn = conn
	a.connID = cc.ID
	a.SetStatus(fmt.Sprintf("connected: [::b]%s[::-] (%s)   %s", cc.ID, cc.Driver, defaultStatus))
	return nil
}

func (a *App) onKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyCtrlC || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q') {
		a.app.Stop()
		return nil
	}
	return ev
}

const defaultStatus = "[::b]?[::-] help   [::b]q[::-] quit"

// SetStatus updates the bottom status/help bar (supports tview color tags).
func (a *App) SetStatus(s string) { a.status.SetText(" " + s) }

func (a *App) setError(msg string) { a.SetStatus("[red]" + msg + "[-]") }

// Run starts the UI loop and blocks until the user quits. The open connection
// (if any) is closed on exit.
func (a *App) Run() error {
	defer a.closeConn()
	return a.app.Run()
}

// Stop terminates the UI loop.
func (a *App) Stop() { a.app.Stop() }

func (a *App) closeConn() {
	if a.conn != nil {
		_ = a.conn.Close()
		a.conn = nil
	}
}

func connSummary(cc config.ConnectionConfig) string {
	if cc.Driver == "sqlite" {
		return "sqlite " + cc.Database
	}
	return fmt.Sprintf("%s %s@%s:%d/%s", cc.Driver, cc.User, cc.Host, cc.Port, cc.Database)
}

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
