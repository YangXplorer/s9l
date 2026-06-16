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
	"strings"
	"sync"
	"time"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	sidebarWidth = 30
	// resultLimit caps rows fetched when browsing a table (full control via the
	// SQL editor lands in T-2a).
	resultLimit = 200
)

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
	app   *tview.Application
	pages *tview.Pages

	cfg   *config.Config
	store secret.SecretStore

	connList *tview.List
	schema   *tview.TreeView
	results  *tview.Table
	editor   *tview.TextArea
	status   *tview.TextView

	focusIdx int
	helpOpen bool

	conn       driver.Conn
	connID     string
	driverName string

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
	a.schema.SetSelectedFunc(a.onSchemaSelect)
	a.schema.SetBorder(true).SetTitle(" Schema ")

	a.results = tview.NewTable().SetBorders(false).SetFixed(1, 0)
	a.results.SetSelectable(true, false)
	a.results.SetBorder(true).SetTitle(" Results ")

	a.editor = tview.NewTextArea().SetPlaceholder("Type SQL here, then press F5 to run…")
	a.editor.SetBorder(true).SetTitle(" SQL (F5 run) ")

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

	a.pages = tview.NewPages().AddPage("main", root, true, true)
	a.app.SetRoot(a.pages, true).EnableMouse(true)
	a.focusPanel(0)
}

// navPanels are the focusable panels cycled by Tab / 1-4.
func (a *App) navPanels() []tview.Primitive {
	return []tview.Primitive{a.connList, a.schema, a.results, a.editor}
}

// focusPanel moves focus to panel i and highlights its border.
func (a *App) focusPanel(i int) {
	a.focusIdx = i
	a.app.SetFocus(a.navPanels()[i])
	a.connList.SetBorderColor(borderColor(i == 0))
	a.schema.SetBorderColor(borderColor(i == 1))
	a.results.SetBorderColor(borderColor(i == 2))
	a.editor.SetBorderColor(borderColor(i == 3))
}

func borderColor(focused bool) tcell.Color {
	if focused {
		return tcell.ColorYellow
	}
	return tcell.ColorWhite
}

func (a *App) showHelp() {
	help := tview.NewTextView().SetDynamicColors(true).SetText(helpText)
	help.SetBorder(true).SetTitle(" Help ")
	a.pages.AddPage("help", centered(help, 46, 12), true, true)
	a.helpOpen = true
}

func (a *App) hideHelp() {
	a.pages.RemovePage("help")
	a.helpOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// centered wraps p in spacers so it floats centered at the given size.
func centered(p tview.Primitive, width, height int) tview.Primitive {
	col := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(p, height, 0, true).
		AddItem(nil, 0, 1, false)
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(col, width, 0, true).
		AddItem(nil, 0, 1, false)
}

const helpText = `[::b]s9l TUI[::-]

  Tab / Shift-Tab   switch panel
  1 / 2 / 3 / 4      Connections / Schema / Results / SQL
  Enter             connect / preview table
  F5                run SQL editor
  Up / Down         navigate within a panel
  ?                 toggle this help
  q / Ctrl-C        quit

  [::d]press any key to close[::-]`

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
	a.driverName = cc.Driver
	a.SetStatus(fmt.Sprintf("connected: [::b]%s[::-] (%s)   %s", cc.ID, cc.Driver, defaultStatus))
	a.loadSchema()
	return nil
}

// onSchemaSelect handles Enter on a schema node: a table node (its reference is
// the table name) runs a preview query; the root node toggles expansion.
func (a *App) onSchemaSelect(node *tview.TreeNode) {
	table, ok := node.GetReference().(string)
	if !ok || table == "" {
		node.SetExpanded(!node.IsExpanded())
		return
	}
	a.runTableQuery(table)
}

// runTableQuery previews a table: SELECT * ... LIMIT resultLimit. The table name
// is quoted per dialect to tolerate reserved words / special characters.
func (a *App) runTableQuery(table string) {
	a.runQuery(fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoteIdent(a.driverName, table), resultLimit))
}

// runQuery executes sql on the current connection and renders the result into
// the Results table. Errors go to the status bar. (Async execution and
// cancellation arrive in T-2b; history recording in T-3.)
func (a *App) runQuery(sql string) {
	if a.conn == nil {
		a.setError("not connected")
		return
	}
	start := time.Now()
	rows, err := a.conn.Query(context.Background(), sql)
	if err != nil {
		a.setError(err.Error())
		return
	}
	cols, data, err := drainRows(rows)
	if err != nil {
		a.setError(err.Error())
		return
	}
	a.fillResults(cols, data)
	a.SetStatus(fmt.Sprintf("%d rows · %s   %s", len(data), time.Since(start).Round(time.Millisecond), defaultStatus))
}

// fillResults renders columns + rows into the Results table (header fixed).
func (a *App) fillResults(cols []string, data [][]any) {
	a.results.Clear()
	for c, name := range cols {
		a.results.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold).
			SetTextColor(tcell.ColorYellow))
	}
	for r, row := range data {
		for c := range cols {
			var s string
			if c < len(row) {
				s = cellString(row[c])
			}
			a.results.SetCell(r+1, c, tview.NewTableCell(s))
		}
	}
	a.results.ScrollToBeginning()
}

func drainRows(rows driver.Rows) ([]string, [][]any, error) {
	defer func() { _ = rows.Close() }()
	cols := rows.Columns()
	var data [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, nil, err
		}
		data = append(data, vals)
	}
	return cols, data, rows.Err()
}

func cellString(v any) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprintf("%v", v)
}

// quoteIdent quotes a SQL identifier for the given driver (backticks for MySQL,
// double quotes otherwise), escaping the quote char.
func quoteIdent(driverName, name string) string {
	if driverName == "mysql" {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// loadSchema populates the schema tree with the connected database's tables via
// the driver's Metadata capability. Table nodes carry the table name as their
// reference (used by T-1c to run SELECT). Multi-database browsing (switching
// databases) is a later enhancement — a single connection sees one database.
func (a *App) loadSchema() {
	root := tview.NewTreeNode(a.connID).SetColor(tcell.ColorYellow)
	a.schema.SetRoot(root).SetCurrentNode(root)

	md, ok := a.conn.(driver.Metadata)
	if !ok {
		root.AddChild(tview.NewTreeNode("(driver has no metadata)"))
		return
	}
	rows, err := md.Tables(context.Background())
	if err != nil {
		a.setError("list tables: " + err.Error())
		return
	}
	tables, err := collectFirstColumn(rows)
	if err != nil {
		a.setError("read tables: " + err.Error())
		return
	}
	for _, name := range tables {
		root.AddChild(tview.NewTreeNode(name).SetReference(name))
	}
}

// collectFirstColumn reads the first column of every row as a string, closing
// the rows. Used for metadata listings (table/database names).
func collectFirstColumn(rows driver.Rows) ([]string, error) {
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		if len(vals) > 0 {
			out = append(out, fmt.Sprintf("%v", vals[0]))
		}
	}
	return out, rows.Err()
}

func (a *App) onKey(ev *tcell.EventKey) *tcell.EventKey {
	// While help is open, any key dismisses it.
	if a.helpOpen {
		a.hideHelp()
		return nil
	}

	// Keys that work everywhere, including while typing in the SQL editor.
	switch ev.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyF5:
		a.runEditor()
		return nil
	case tcell.KeyTab:
		a.focusPanel((a.focusIdx + 1) % len(a.navPanels()))
		return nil
	case tcell.KeyBacktab:
		a.focusPanel((a.focusIdx + len(a.navPanels()) - 1) % len(a.navPanels()))
		return nil
	}

	// When the SQL editor is focused, let everything else through so the user
	// can type freely (letters, digits, '?', 'q' are text, not shortcuts).
	if a.app.GetFocus() == a.editor {
		return ev
	}

	if ev.Key() == tcell.KeyRune {
		switch ev.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case '?':
			a.showHelp()
			return nil
		case '1', '2', '3', '4':
			a.focusPanel(int(ev.Rune() - '1'))
			return nil
		}
	}
	return ev
}

// runEditor runs the SQL currently in the editor (F5). Empty input is a no-op
// with a hint; results/errors land in the Results table / status bar.
func (a *App) runEditor() {
	sql := strings.TrimSpace(a.editor.GetText())
	if sql == "" {
		a.SetStatus("SQL editor is empty   " + defaultStatus)
		return
	}
	a.runQuery(sql)
}

const defaultStatus = "[::b]Tab[::-] panel  [::b]Enter[::-] open  [::b]F5[::-] run  [::b]?[::-] help  [::b]q[::-] quit"

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
