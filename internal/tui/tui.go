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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/history"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	sidebarWidth = 30
	// resultLimit caps rows fetched when browsing a table (full control via the
	// SQL editor lands in T-2a).
	resultLimit = 200
	// editorHeight is the SQL editor's fixed height in rows. tview gives the
	// panel a 2-row border, so this is ~10 editable lines — roughly double the
	// original 6 so multi-line statements have room.
	editorHeight = 12
)

// Options configures the TUI.
type Options struct {
	// Conn is an optional connection id to auto-open on launch.
	Conn string
	// Config is the loaded configuration. If nil, New loads it via config.Load.
	Config *config.Config
	// Store resolves password references. If nil, an in-memory store is used.
	Store secret.SecretStore
	// History records/queries history. If nil, history features are disabled
	// (New does no I/O); the cmd layer provides the real store.
	History *history.Store
}

// App is the s9l TUI application.
type App struct {
	app   *tview.Application
	pages *tview.Pages

	cfg   *config.Config
	store secret.SecretStore
	hist  *history.Store

	connList *tview.List
	schema   *tview.TreeView
	results  *tview.Table
	editor   *tview.TextArea
	status   *tview.TextView
	keybar   *tview.TextView

	theme Theme

	focusIdx    int
	helpOpen    bool
	historyOpen bool
	savedOpen   bool

	running bool               // a query is executing
	cancel  context.CancelFunc // cancels the running query (Esc)

	onResult func() // test hook fired after a query completes (UI goroutine)

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
		hist:  opts.History,
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

	a.theme = newTheme()
	useRoundedBorders()
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
	a.connList = tview.NewList().ShowSecondaryText(false)
	a.titledPanel(a.connList.Box, "[1] Connections")

	a.schema = tview.NewTreeView()
	a.schema.SetSelectedFunc(a.onSchemaSelect)
	a.titledPanel(a.schema.Box, "[2] Schema")

	a.results = tview.NewTable().SetBorders(false).SetFixed(1, 0)
	a.results.SetSelectable(true, false)
	a.titledPanel(a.results.Box, "[3] Results")

	a.editor = tview.NewTextArea().SetPlaceholder("Type SQL here, then press F5 to run…")
	a.titledPanel(a.editor.Box, "[4] SQL (F5 run)")

	// Selected-row highlight (skipped under NO_COLOR so tview's default reverse
	// styling keeps the selection visible on monochrome terminals).
	if !noColor() {
		a.connList.SetSelectedBackgroundColor(a.theme.Selection).SetSelectedTextColor(tcell.ColorBlack)
		a.results.SetSelectedStyle(tcell.StyleDefault.Background(a.theme.Selection).Foreground(tcell.ColorBlack))
	}

	a.status = tview.NewTextView().SetDynamicColors(true)
	a.SetStatus(defaultStatus)

	// keybar is the static, always-visible lazygit-style shortcut line.
	a.keybar = tview.NewTextView().SetDynamicColors(true)
	a.keybar.SetText(" " + a.keyBar())

	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.connList, 0, 1, true).
		AddItem(a.schema, 0, 1, false)
	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.results, 0, 1, false).
		AddItem(a.editor, editorHeight, 0, false)
	body := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, sidebarWidth, 0, true).
		AddItem(right, 0, 1, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 0, 1, true).
		AddItem(a.status, 1, 0, false).
		AddItem(a.keybar, 1, 0, false)

	a.pages = tview.NewPages().AddPage("main", root, true, true)
	a.app.SetRoot(a.pages, true).EnableMouse(true)
	a.focusPanel(0)
}

// titledPanel applies the bordered, titled look shared by every panel.
func (a *App) titledPanel(b *tview.Box, title string) {
	b.SetBorder(true).SetTitle(" " + title + " ").SetTitleColor(a.theme.Title)
}

// navPanels are the focusable panels cycled by Tab / 1-4.
func (a *App) navPanels() []tview.Primitive {
	return []tview.Primitive{a.connList, a.schema, a.results, a.editor}
}

// focusPanel moves focus to panel i and highlights its border.
func (a *App) focusPanel(i int) {
	a.focusIdx = i
	a.app.SetFocus(a.navPanels()[i])
	a.connList.SetBorderColor(a.theme.border(i == 0))
	a.schema.SetBorderColor(a.theme.border(i == 1))
	a.results.SetBorderColor(a.theme.border(i == 2))
	a.editor.SetBorderColor(a.theme.border(i == 3))
}

// keyBar renders the static bottom shortcut line with accent-colored keys.
func (a *App) keyBar() string {
	open := a.theme.tag(a.theme.Accent) + "[::b]"
	closing := "[::-]" + a.theme.reset()
	keys := []struct{ key, label string }{
		{"Tab", "panel"}, {"F5", "run"}, {"^R", "history"},
		{"^F", "saved"}, {"^S", "save"}, {"?", "help"}, {"q", "quit"},
	}
	var b strings.Builder
	for i, e := range keys {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(open + e.key + closing + " " + e.label)
	}
	return b.String()
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
  Ctrl-R            query history (Enter loads it)
  Ctrl-F            saved queries (Enter runs it)
  Ctrl-S            save editor SQL as a favorite
  Up / Down · j / k navigate within a panel
  ?                 toggle this help
  q / Ctrl-C        quit

  [::d]press any key to close[::-]`

// populateConnections fills the list from config; Enter connects. Each row is a
// single line: a database-type icon plus the connection's display name.
func (a *App) populateConnections() {
	a.connList.Clear()
	for _, cc := range a.cfg.Connections {
		a.connList.AddItem(connIcon(cc.Driver)+connDisplayName(cc), "", 0, func() {
			_ = a.connect(cc)
		})
	}
	if a.connList.GetItemCount() == 0 {
		a.connList.AddItem("(no connections — run: s9l conn add)", "", 0, nil)
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
	a.SetStatus(fmt.Sprintf("connected: [::b]%s[::-] (%s)", cc.ID, cc.Driver))
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

// queryResult holds a fetched result set.
type queryResult struct {
	cols []string
	data [][]any
}

// runQuery executes sql off the UI goroutine so the interface stays responsive,
// pushing the result back via QueueUpdateDraw. The query is cancellable with Esc
// and recorded in history (best-effort).
func (a *App) runQuery(sql string) {
	if a.conn == nil {
		a.setError("not connected")
		return
	}
	if a.running {
		a.SetStatus("a query is already running… ([::b]Esc[::-] to cancel)")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	conn := a.conn
	a.running = true
	a.cancel = cancel
	a.SetStatus("running… ([::b]Esc[::-] to cancel)")
	start := time.Now()

	go func() {
		res, err := fetch(ctx, conn, sql)
		elapsed := time.Since(start)
		a.app.QueueUpdateDraw(func() {
			a.running = false
			a.cancel = nil
			cancel()
			if err != nil {
				cerr := classifyErr(err)
				a.recordHistory(sql, elapsed, 0, cerr)
				a.setError(cerr.Error())
			} else {
				a.fillResults(res.cols, res.data)
				a.recordHistory(sql, elapsed, len(res.data), nil)
				a.SetStatus(fmt.Sprintf("%d rows · %s", len(res.data), elapsed.Round(time.Millisecond)))
			}
			if a.onResult != nil {
				a.onResult()
			}
		})
	}()
}

// fetch runs sql and drains the rows. It is synchronous and UI-independent so it
// can be unit-tested without the event loop.
func fetch(ctx context.Context, conn driver.Conn, sql string) (queryResult, error) {
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return queryResult{}, err
	}
	cols, data, err := drainRows(rows)
	return queryResult{cols: cols, data: data}, err
}

// classifyErr turns context errors into clear messages (mirrors the CLI's B3).
func classifyErr(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("query cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("query timed out")
	default:
		return err
	}
}

// recordHistory best-effort records an executed query (no-op if history is
// disabled). Failures are ignored so they never disrupt the UI.
func (a *App) recordHistory(sql string, dur time.Duration, rows int, qerr error) {
	if a.hist == nil {
		return
	}
	e := history.HistoryEntry{
		ConnectionID: a.connID,
		SQL:          sql,
		ExecutedAt:   time.Now(),
		Duration:     dur,
		RowsAffected: int64(rows),
		Success:      qerr == nil,
	}
	if qerr != nil {
		e.ErrorMessage = qerr.Error()
	}
	_, _ = a.hist.AddHistory(context.Background(), e)
}

// showHistory opens an overlay listing recent queries; Enter loads one into the
// editor. Ctrl-R / Esc close it.
func (a *App) showHistory() {
	if a.hist == nil {
		a.SetStatus("history unavailable")
		return
	}
	entries, err := a.hist.ListHistory(context.Background(), 100)
	if err != nil {
		a.setError("history: " + err.Error())
		return
	}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" History — Enter: load · Esc: close ")
	if len(entries) == 0 {
		list.AddItem("(no history yet)", "", 0, nil)
	}
	for _, e := range entries {
		status := "ok "
		if !e.Success {
			status = "ERR"
		}
		label := fmt.Sprintf("%s  %s  %s", e.ExecutedAt.Local().Format("01-02 15:04:05"), status, oneLine(e.SQL))
		sql := e.SQL
		list.AddItem(label, "", 0, func() { a.useHistorySQL(sql) })
	}

	a.pages.AddPage("history", centered(list, 90, 22), true, true)
	a.app.SetFocus(list)
	a.historyOpen = true
}

func (a *App) hideHistory() {
	a.pages.RemovePage("history")
	a.historyOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// useHistorySQL loads a history entry's SQL into the editor (does not auto-run,
// so the user can review then F5).
func (a *App) useHistorySQL(sql string) {
	a.hideHistory()
	a.editor.SetText(sql, true)
	a.focusPanel(3)
}

// oneLine collapses whitespace runs to single spaces for compact list display.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// saveCurrent saves the editor's SQL as a favorite (Ctrl-S). The title is
// derived from the SQL; editing the title is a later enhancement.
func (a *App) saveCurrent() {
	if a.hist == nil {
		a.SetStatus("history unavailable")
		return
	}
	sql := strings.TrimSpace(a.editor.GetText())
	if sql == "" {
		a.SetStatus("nothing to save: SQL editor is empty")
		return
	}
	id, err := a.hist.SaveQuery(context.Background(), history.SavedQuery{
		Title:        titleFrom(sql),
		ConnectionID: a.connID,
		SQL:          sql,
	})
	if err != nil {
		a.setError("save: " + err.Error())
		return
	}
	a.SetStatus(fmt.Sprintf("saved query #%d", id))
}

// showSaved opens an overlay of saved queries (Ctrl-F); Enter runs one.
// Ctrl-F / Esc close it.
func (a *App) showSaved() {
	if a.hist == nil {
		a.SetStatus("history unavailable")
		return
	}
	items, err := a.hist.ListSaved(context.Background())
	if err != nil {
		a.setError("saved: " + err.Error())
		return
	}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" Saved — Enter: run · Esc: close ")
	if len(items) == 0 {
		list.AddItem("(no saved queries — Ctrl-S to save the editor)", "", 0, nil)
	}
	for _, q := range items {
		meta := q.ConnectionID
		if q.Tags != "" {
			meta += " [" + q.Tags + "]"
		}
		label := fmt.Sprintf("%s  %s  %s", q.Title, meta, oneLine(q.SQL))
		sql := q.SQL
		list.AddItem(label, "", 0, func() { a.runSavedQuery(sql) })
	}

	a.pages.AddPage("saved", centered(list, 90, 22), true, true)
	a.app.SetFocus(list)
	a.savedOpen = true
}

func (a *App) hideSaved() {
	a.pages.RemovePage("saved")
	a.savedOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// runSavedQuery closes the overlay and runs the chosen saved query.
func (a *App) runSavedQuery(sql string) {
	a.hideSaved()
	a.editor.SetText(sql, true)
	a.runQuery(sql)
}

// titleFrom derives a saved-query title from its SQL (first 50 runes, one line).
func titleFrom(sql string) string {
	r := []rune(oneLine(sql))
	if len(r) > 50 {
		return string(r[:50])
	}
	return string(r)
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

	// Vim-style navigation: j/k → Down/Up in any focused widget except the SQL
	// editor (where they are text). Applies in panels and in the list overlays.
	if ev.Key() == tcell.KeyRune && a.app.GetFocus() != a.editor {
		switch ev.Rune() {
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
	}

	// While the history overlay is open, Esc / Ctrl-R close it; other keys
	// (arrows, Enter) go to the list.
	if a.historyOpen {
		if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyCtrlR {
			a.hideHistory()
			return nil
		}
		return ev
	}

	// While the saved overlay is open, Esc / Ctrl-F close it; other keys go
	// to the list.
	if a.savedOpen {
		if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyCtrlF {
			a.hideSaved()
			return nil
		}
		return ev
	}

	// Esc cancels a running query; when idle it passes through (e.g. to widgets).
	if ev.Key() == tcell.KeyEscape {
		if a.running {
			if a.cancel != nil {
				a.cancel()
			}
			a.SetStatus("cancelling…")
			return nil
		}
		return ev
	}

	// Keys that work everywhere, including while typing in the SQL editor.
	switch ev.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyF5:
		a.runEditor()
		return nil
	case tcell.KeyCtrlR:
		a.showHistory()
		return nil
	case tcell.KeyCtrlS:
		a.saveCurrent()
		return nil
	case tcell.KeyCtrlF:
		a.showSaved()
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
		a.SetStatus("SQL editor is empty")
		return
	}
	a.runQuery(sql)
}

// defaultStatus is the idle status-line message; the shortcut keys live in the
// always-visible keybar below it (see keyBar).
const defaultStatus = "ready"

// SetStatus updates the bottom status/help bar (supports tview color tags).
func (a *App) SetStatus(s string) { a.status.SetText(" " + s) }

func (a *App) setError(msg string) {
	a.SetStatus(a.theme.tag(a.theme.Error) + msg + a.theme.reset())
}

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
