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
	"github.com/YangXplorer/s9l/internal/dial"
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

	connTree *tview.TreeView
	schema   *tview.TreeView
	results  *tview.Table
	editor   *tview.TextArea
	status   *tview.TextView
	keybar   *tview.TextView

	theme     Theme
	currentDB string // database selected in the Connections tree (drives Schema)

	// Schema panel table list, retained so the table filter can re-render.
	schemaTables []string
	schemaFilter string

	// Connections panel database list, retained so / can filter databases.
	connDatabases []string         // databases under the connected (multi-db) connection
	connFilter    string           // active database filter term
	connDBOwner   string           // connID owning connDatabases (for dbNodeRef)
	connDBNode    *tview.TreeNode  // the connection node whose children are those databases

	filterTarget filterTarget // which panel the open filter input targets
	filterCol    int          // Results column index for the column filter (f)

	focusIdx     int
	helpOpen     bool
	historyOpen  bool
	savedOpen    bool
	filterOpen      bool
	connFormOpen    bool
	confirmOpen     bool
	exportOpen      bool
	cellValueOpen   bool
	cellEditOpen    bool
	confirmEditOpen bool

	// last result set, retained so the Results filter can re-render client-side.
	lastCols []string
	lastData [][]any
	filter   string
	viewRows [][]any // rows currently rendered (after filtering); maps table row → values

	// Source of the current result, for in-place cell edit (UPDATE write-back).
	resultTable    tableRef // single-table preview source (empty when not a preview)
	resultEditable bool     // true only when the result is a single-table preview

	running bool               // a query is executing
	cancel  context.CancelFunc // cancels the running query (Esc)

	onResult func() // test hook fired after a query completes (UI goroutine)

	conn       driver.Conn
	connClose  func() error // closes conn + any SSH tunnel
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
	a.theme.applyStyles()
	useRoundedBorders()
	a.buildLayout()
	a.populateConnections()

	a.app.SetInputCapture(a.onKey)

	if opts.Conn != "" {
		// Auto-connect shares the Enter path: connect + expand databases.
		if node := a.findConnNode(opts.Conn); node != nil {
			a.connTree.SetCurrentNode(node)
			a.onConnSelect(node)
		}
	}
	return a
}

func (a *App) buildLayout() {
	a.connTree = tview.NewTreeView().SetTopLevel(1).SetGraphics(false) // hide root + tree lines
	a.connTree.SetSelectedFunc(a.onConnSelect)
	a.titledPanel(a.connTree.Box, "[1] Connections")

	// SetTopLevel(1) hides the (database-name) root so tables render flat — no
	// tree indentation or colored root, just the current database's table list.
	a.schema = tview.NewTreeView().SetTopLevel(1).SetGraphics(false)
	a.schema.SetSelectedFunc(a.onSchemaSelect)
	a.titledPanel(a.schema.Box, "[2] Schema")

	a.results = tview.NewTable().SetBorders(false).SetFixed(1, 0)
	// Cell selection (rows + columns) so the cursor moves left/right between
	// cells; the selected cell drives "view value" and (later) in-place edit.
	a.results.SetSelectable(true, true)
	a.results.SetSelectionChangedFunc(a.onResultsCellChanged)
	a.titledPanel(a.results.Box, "[3] Results")

	a.editor = tview.NewTextArea().SetPlaceholder("Type SQL here, then press F5 to run…")
	a.titledPanel(a.editor.Box, "[4] SQL (F5 run)")

	// Selected-row highlight: a light bar with dark text (reverse under NO_COLOR).
	a.results.SetSelectedStyle(a.theme.selectionStyle())

	a.status = tview.NewTextView().SetDynamicColors(true)
	a.SetStatus(defaultStatus)

	// keybar is the static, always-visible lazygit-style shortcut line.
	a.keybar = tview.NewTextView().SetDynamicColors(true)
	a.keybar.SetText(" " + a.keyBar())

	// Connections is shorter than Schema (2:3): connections are usually few,
	// while the table list benefits from the extra room.
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.connTree, 0, 2, true).
		AddItem(a.schema, 0, 3, false)
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

// treeNode creates a tree node carrying the theme's selected-row style so the
// current node is highlighted readably.
func (a *App) treeNode(text string) *tview.TreeNode {
	return tview.NewTreeNode(text).SetSelectedTextStyle(a.theme.selectionStyle())
}

// navPanels are the focusable panels cycled by Tab / 1-4.
func (a *App) navPanels() []tview.Primitive {
	return []tview.Primitive{a.connTree, a.schema, a.results, a.editor}
}

// focusPanel moves focus to panel i and highlights its border.
func (a *App) focusPanel(i int) {
	a.focusIdx = i
	a.app.SetFocus(a.navPanels()[i])
	a.connTree.SetBorderColor(a.theme.border(i == 0))
	a.schema.SetBorderColor(a.theme.border(i == 1))
	a.results.SetBorderColor(a.theme.border(i == 2))
	a.editor.SetBorderColor(a.theme.border(i == 3))
}

// keyBar renders the static bottom shortcut line with accent-colored keys.
func (a *App) keyBar() string {
	open := a.theme.tag(a.theme.Accent) + "[::b]"
	closing := "[::-]" + a.theme.reset()
	keys := []struct{ key, label string }{
		{"Tab", "panel"}, {"n", "new"}, {"F5", "run"}, {"/", "filter"},
		{"^R", "history"}, {"^F", "saved"}, {"?", "help"}, {"q", "quit"},
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
	a.pages.AddPage("help", centered(help, 46, 17), true, true)
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
  Enter             drill in: connect+databases · pick database · preview table
  n / e / d         new / edit / delete connection (Connections panel only)
  F5                run SQL editor
  /                 filter (Connections: databases · Schema: tables · Results: all-column fuzzy)
  f                 Results: filter by the selected column
  v                 Results: view the selected cell's full value
  c                 Results: edit the selected cell (single-table preview only)
  h / l · ← / →     move left/right (Results: between cells)
  Ctrl-R            query history (Enter loads it)
  Ctrl-F            saved queries (Enter runs it)
  Ctrl-S            save editor SQL as a favorite
  Ctrl-E            export the current results to a file (csv/json/tsv)
  Up / Down · j / k navigate within a panel
  ?                 toggle this help
  q / Ctrl-C        quit

  [::d]press any key to close[::-]`

// connNodeRef marks a connection node in the Connections tree.
type connNodeRef struct{ cc config.ConnectionConfig }

// dbNodeRef marks a database node (a child of a connection node).
type dbNodeRef struct {
	connID string
	db     string
}

// populateConnections (re)builds the Connections tree from config: each
// connection is a top-level node (icon + display name); Enter connects it and,
// for multi-database engines, expands it to its databases.
func (a *App) populateConnections() {
	// Rebuilding the tree invalidates any retained database node/list.
	a.connDBNode = nil
	a.connDatabases = nil
	a.connFilter = ""
	root := tview.NewTreeNode("")
	a.connTree.SetRoot(root)
	for _, cc := range a.cfg.Connections {
		n := a.treeNode("").SetReference(connNodeRef{cc: cc})
		a.setConnNodeLabel(n, cc)
		root.AddChild(n)
	}
	if len(a.cfg.Connections) == 0 {
		root.AddChild(a.treeNode("(no connections — press n to add)"))
	}
	if kids := root.GetChildren(); len(kids) > 0 {
		a.connTree.SetCurrentNode(kids[0])
	}
}

// setConnNodeLabel sets a connection node's text: an expand indicator (▾/▸, only
// once it has database children) + the database-type icon + the display name.
func (a *App) setConnNodeLabel(node *tview.TreeNode, cc config.ConnectionConfig) {
	prefix := "  "
	if len(node.GetChildren()) > 0 {
		if node.IsExpanded() {
			prefix = "▾ "
		} else {
			prefix = "▸ "
		}
	}
	node.SetText(prefix + connIcon(cc.Driver) + connDisplayName(cc))
}

// onConnSelect handles Enter in the Connections tree: a connection node connects
// and (for multi-database engines) expands to its databases — or, if already
// connected, just toggles. A database node becomes the current database and
// refreshes the Schema panel.
func (a *App) onConnSelect(node *tview.TreeNode) {
	switch ref := node.GetReference().(type) {
	case connNodeRef:
		if len(node.GetChildren()) == 0 {
			if err := a.connect(ref.cc); err != nil {
				return
			}
			a.loadConnDatabases(node, ref.cc)
			node.SetExpanded(true)
			// Drill into the sub-part: multi-db engines now have database
			// children → move the cursor onto the first one to pick; single-db
			// engines loaded Schema directly → focus the Schema panel.
			if kids := node.GetChildren(); len(kids) > 0 {
				a.connTree.SetCurrentNode(kids[0])
			} else {
				a.focusPanel(1)
			}
		} else {
			node.SetExpanded(!node.IsExpanded())
		}
		a.setConnNodeLabel(node, ref.cc)
	case dbNodeRef:
		a.currentDB = ref.db
		a.loadSchema()
		a.SetStatus(fmt.Sprintf("database: [::b]%s[::-]", ref.db))
		a.focusPanel(1) // drill into the sub-part: the database's tables (Schema)
	default:
		node.SetExpanded(!node.IsExpanded())
	}
}

// loadConnDatabases fills a just-connected connection node. Multi-database
// engines list their databases as child nodes (pick one to drive Schema);
// others have no database level, so Schema shows the connected database's
// tables directly.
func (a *App) loadConnDatabases(node *tview.TreeNode, cc config.ConnectionConfig) {
	a.currentDB = ""
	node.ClearChildren()
	// Reset retained database state; single-database engines leave it empty so
	// the Connections / filter reports "no databases to filter".
	a.connDatabases = nil
	a.connDBNode = nil
	a.connFilter = ""

	md, isMeta := a.conn.(driver.Metadata)
	if _, multi := a.conn.(databaseBrowser); !multi || !isMeta {
		a.loadSchema() // single-database engines: tables of the connected db
		return
	}
	dbs, err := namesFrom(md.Databases(context.Background()))
	if err != nil {
		a.setError("list databases: " + err.Error())
		return
	}
	a.connDatabases = dbs
	a.connDBOwner = cc.ID
	a.connDBNode = node
	a.renderConnDatabases()
	a.schemaPlaceholder("select a database")
}

// renderConnDatabases (re)builds the database child nodes of the current
// connection's node, applying the active connection filter. Each child is a
// plain (uncolored) list item; the expand toggle stays on the parent.
func (a *App) renderConnDatabases() {
	if a.connDBNode == nil {
		return
	}
	a.connDBNode.ClearChildren()
	for _, db := range filterTables(a.connDatabases, a.connFilter) {
		a.connDBNode.AddChild(a.treeNode(db).
			SetReference(dbNodeRef{connID: a.connDBOwner, db: db}))
	}
}

// applyConnFilter re-renders the Connections database list keeping databases
// matching term and reports the match count.
func (a *App) applyConnFilter(term string) {
	a.connFilter = term
	a.renderConnDatabases()
	if a.connDBNode != nil {
		a.connDBNode.SetExpanded(true) // keep the filtered list visible
	}
	if term == "" {
		a.SetStatus(fmt.Sprintf("%d databases", len(a.connDatabases)))
	} else {
		a.SetStatus(fmt.Sprintf("databases %d/%d", len(filterTables(a.connDatabases, term)), len(a.connDatabases)))
	}
}

// findConnNode returns the Connections-tree node for the connection id, or nil.
func (a *App) findConnNode(id string) *tview.TreeNode {
	if a.connTree.GetRoot() == nil {
		return nil
	}
	for _, n := range a.connTree.GetRoot().GetChildren() {
		if ref, ok := n.GetReference().(connNodeRef); ok && ref.cc.ID == id {
			return n
		}
	}
	return nil
}

// connect resolves the connection's password, opens it, and updates status.
// Errors are surfaced in the status bar; the UI never crashes on a bad connect.
func (a *App) connect(cc config.ConnectionConfig) error {
	conn, closer, err := dial.Open(context.Background(), cc, a.store)
	if err != nil {
		a.setError(err.Error())
		return err
	}

	a.closeConn() // close any previous connection + tunnel
	a.conn = conn
	a.connClose = closer
	a.connID = cc.ID
	a.driverName = cc.Driver
	a.currentDB = ""
	a.SetStatus(fmt.Sprintf("connected: [::b]%s[::-] (%s)", cc.ID, cc.Driver))
	// What to show after connecting (databases vs tables) is decided by the
	// caller via loadConnDatabases, so auto-connect and Enter share one path.
	return nil
}

// databaseBrowser is an optional capability matched structurally (so the core
// driver package is untouched): a conn that can list tables in a *named*
// database. It enables the database→table tree. MySQL implements it; engines
// that can't browse other databases on a single connection (e.g. PostgreSQL)
// don't, and fall back to listing the connected database's tables.
type databaseBrowser interface {
	TablesIn(ctx context.Context, database string) (driver.Rows, error)
}

// tableRef marks a table node in the Schema panel; db=="" means the connected
// database (single-database engines), otherwise the database picked in the
// Connections tree.
type tableRef struct{ db, name string }

// onSchemaSelect handles Enter on a schema node: a table node runs a preview
// query; anything else toggles expansion.
func (a *App) onSchemaSelect(node *tview.TreeNode) {
	if ref, ok := node.GetReference().(tableRef); ok {
		a.runTableQuery(ref)
		a.focusPanel(2) // drill into the sub-part: the table's rows (Results)
		return
	}
	node.SetExpanded(!node.IsExpanded())
}

// runTableQuery previews a table: the first resultLimit rows. The name is quoted
// per dialect (and database-qualified when browsing another database).
func (a *App) runTableQuery(ref tableRef) {
	a.runQuery(previewQuery(a.driverName, qualifyTable(a.driverName, ref), resultLimit))
	// A single-table preview is editable; runQuery cleared these for the generic
	// case, so mark editability after it returns (it only spawns a goroutine).
	a.resultTable = ref
	a.resultEditable = true
}

// qualifyTable builds the quoted, optionally database-qualified table name.
func qualifyTable(driverName string, ref tableRef) string {
	t := quoteIdent(driverName, ref.name)
	if ref.db != "" {
		t = quoteIdent(driverName, ref.db) + "." + t
	}
	return t
}

// previewQuery builds a "first N rows" SELECT, dialect-aware: SQL Server has no
// LIMIT and uses TOP instead.
func previewQuery(driverName, qualified string, n int) string {
	if driverName == "sqlserver" {
		return fmt.Sprintf("SELECT TOP %d * FROM %s", n, qualified)
	}
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d", qualified, n)
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
	// Default: an arbitrary query result is not cell-editable; runTableQuery
	// re-marks single-table previews as editable after this returns.
	a.resultEditable = false
	a.resultTable = tableRef{}

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
				a.setResults(res.cols, res.data)
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

// setResults stores a fresh result set (clearing any active filter) and renders
// it. The retained copy lets the filter re-render client-side without re-querying.
func (a *App) setResults(cols []string, data [][]any) {
	a.lastCols = cols
	a.lastData = data
	a.filter = ""
	a.fillResults(cols, data)
}

// onResultsCellChanged shows the selected cell's position (row · column) in the
// status bar as the cursor moves through the Results table.
func (a *App) onResultsCellChanged(row, col int) {
	if row <= 0 || col < 0 || col >= len(a.lastCols) {
		return // header row or out of range
	}
	a.SetStatus(fmt.Sprintf("row %d · col [::b]%s[::-]", row, a.lastCols[col]))
}

// showCellValue pops up the full value of the selected Results cell (useful for
// long text, NULL, or values wider than the column). Read-only.
func (a *App) showCellValue() {
	if len(a.lastCols) == 0 {
		return
	}
	row, col := a.results.GetSelection()
	if row <= 0 || col < 0 { // header or nothing selected
		return
	}
	rows := filterRows(a.lastData, a.filter)
	if row-1 >= len(rows) || col >= len(rows[row-1]) {
		return
	}
	val := cellString(rows[row-1][col])
	title := fmt.Sprintf(" %s ", a.lastCols[col])
	view := tview.NewTextView().SetText(val).SetWrap(true).SetScrollable(true)
	view.SetTextColor(a.theme.FieldText)
	view.SetBackgroundColor(a.theme.Surface)
	view.SetBorder(true).SetTitle(title).SetTitleColor(a.theme.Title).SetBorderColor(a.theme.Focus)
	a.pages.AddPage("cellvalue", centered(view, 70, 12), true, true)
	a.app.SetFocus(view)
	a.cellValueOpen = true
}

// hideCellValue closes the cell-value overlay.
func (a *App) hideCellValue() {
	a.pages.RemovePage("cellvalue")
	a.cellValueOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// placeholderTUI returns the positional parameter placeholder for the driver
// (pg $n, sqlserver @pN, others ?). Mirrors the import command's dialect rules.
func placeholderTUI(driverName string, n int) string {
	switch driverName {
	case "postgres":
		return fmt.Sprintf("$%d", n)
	case "sqlserver":
		return fmt.Sprintf("@p%d", n)
	default:
		return "?"
	}
}

// buildUpdate builds a single-cell UPDATE: SET the edited column to newVal, with
// a WHERE matching every original column value (IS NULL for NULL cells). Returns
// the SQL and its positional args (newVal first, then the non-NULL where values).
func buildUpdate(driverName, qualified string, cols []string, editCol int, newVal string, rowVals []any) (string, []any) {
	args := []any{newVal}
	n := 1
	set := quoteIdent(driverName, cols[editCol]) + " = " + placeholderTUI(driverName, n)
	where := make([]string, 0, len(cols))
	for i, c := range cols {
		if i >= len(rowVals) {
			continue
		}
		if rowVals[i] == nil {
			where = append(where, quoteIdent(driverName, c)+" IS NULL")
			continue
		}
		n++
		where = append(where, quoteIdent(driverName, c)+" = "+placeholderTUI(driverName, n))
		args = append(args, rowVals[i])
	}
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s", qualified, set, strings.Join(where, " AND "))
	return sql, args
}

// showCellEdit opens an input to edit the selected Results cell (c). Only single-
// table previews are editable; otherwise it reports why and does nothing.
func (a *App) showCellEdit() {
	if !a.resultEditable {
		a.SetStatus("cell editing needs a single-table preview")
		return
	}
	row, col := a.results.GetSelection()
	if row <= 0 || col < 0 || row-1 >= len(a.viewRows) || col >= len(a.lastCols) {
		return
	}
	rowVals := a.viewRows[row-1]
	cur := ""
	if col < len(rowVals) {
		cur = cellString(rowVals[col])
	}
	in := tview.NewInputField().SetLabel(" = ").SetText(cur)
	in.SetFieldBackgroundColor(a.theme.Field).SetFieldTextColor(a.theme.FieldText)
	in.SetBorder(true).SetTitle(fmt.Sprintf(" Edit %s — Enter: next · Esc: cancel ", a.lastCols[col])).SetBorderColor(a.theme.Focus)
	in.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			a.hideCellEdit()
			return
		}
		newVal := in.GetText()
		a.hideCellEdit()
		sql, args := buildUpdate(a.driverName, qualifyTable(a.driverName, a.resultTable), a.lastCols, col, newVal, rowVals)
		a.confirmCellUpdate(sql, args)
	})
	a.pages.AddPage("celledit", centered(in, 60, 3), true, true)
	a.app.SetFocus(in)
	a.cellEditOpen = true
}

func (a *App) hideCellEdit() {
	a.pages.RemovePage("celledit")
	a.cellEditOpen = false
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// confirmCellUpdate shows the generated UPDATE for confirmation before running
// it (the operation mutates data and there is no transaction to undo it).
func (a *App) confirmCellUpdate(sql string, args []any) {
	modal := tview.NewModal().
		SetText("Run this update?\n\n" + sql).
		AddButtons([]string{"Cancel", "Update"}).
		SetDoneFunc(func(_ int, label string) {
			a.pages.RemovePage("confirmedit")
			a.confirmEditOpen = false
			a.app.SetFocus(a.navPanels()[a.focusIdx])
			if label == "Update" {
				a.execCellUpdate(sql, args)
			}
		})
	modal.SetBackgroundColor(a.theme.Field).SetTextColor(a.theme.FieldText)
	a.pages.AddPage("confirmedit", modal, true, true)
	a.app.SetFocus(modal)
	a.confirmEditOpen = true
}

// execCellUpdate runs the UPDATE off the UI goroutine, then refreshes the
// preview. Errors land in the status bar; a single Exec is auto-committed.
func (a *App) execCellUpdate(sql string, args []any) {
	if a.conn == nil {
		a.setError("not connected")
		return
	}
	conn := a.conn
	ref := a.resultTable
	a.SetStatus("updating…")
	go func() {
		res, err := conn.Exec(context.Background(), sql, args...)
		var n int64
		if err == nil && res != nil {
			n, _ = res.RowsAffected()
		}
		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.setError("update: " + err.Error())
				return
			}
			a.SetStatus(fmt.Sprintf("updated %d row(s)", n))
			a.runTableQuery(ref) // refresh from the DB
		})
	}()
}

// fuzzyMatch reports whether term is a case-insensitive subsequence of text:
// every rune of term appears in text, in order (not necessarily contiguous).
// An empty term always matches. This is looser than substring (e.g. "ae"
// matches "alice"), giving / a forgiving full-field fuzzy search.
func fuzzyMatch(text, term string) bool {
	if term == "" {
		return true
	}
	t := []rune(strings.ToLower(term))
	i := 0
	for _, r := range strings.ToLower(text) {
		if r == t[i] {
			i++
			if i == len(t) {
				return true
			}
		}
	}
	return false
}

// filterRows keeps the rows where term fuzzy-matches any cell (case-insensitive
// subsequence, across all columns). An empty term returns data unchanged.
func filterRows(data [][]any, term string) [][]any {
	if term == "" {
		return data
	}
	out := make([][]any, 0, len(data))
	for _, row := range data {
		for _, cell := range row {
			if fuzzyMatch(cellString(cell), term) {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// applyFilter re-renders the Results table with rows matching term and reports
// the match count in the status bar.
func (a *App) applyFilter(term string) {
	a.filter = term
	rows := filterRows(a.lastData, term)
	a.fillResults(a.lastCols, rows)
	if term == "" {
		a.SetStatus(fmt.Sprintf("%d rows", len(a.lastData)))
	} else {
		a.SetStatus(fmt.Sprintf("filtered %d/%d", len(rows), len(a.lastData)))
	}
}

// filterTarget identifies which panel an open / filter input acts on. The
// focused panel decides: Connections filters databases, Schema filters tables,
// everything else filters the Results rows.
type filterTarget int

const (
	filterTgtResults filterTarget = iota
	filterTgtSchema
	filterTgtConn
	filterTgtResultsCol
)

// filterRowsByColumn keeps rows whose cell in column colIdx fuzzy-matches term
// (case-insensitive subsequence). An empty term returns data unchanged.
func filterRowsByColumn(data [][]any, colIdx int, term string) [][]any {
	if term == "" {
		return data
	}
	out := make([][]any, 0, len(data))
	for _, row := range data {
		if colIdx >= 0 && colIdx < len(row) && fuzzyMatch(cellString(row[colIdx]), term) {
			out = append(out, row)
		}
	}
	return out
}

// applyColFilter re-renders the Results table keeping rows whose selected
// column matches term, reporting the match count for that column.
func (a *App) applyColFilter(term string) {
	a.filter = term
	rows := filterRowsByColumn(a.lastData, a.filterCol, term)
	a.fillResults(a.lastCols, rows)
	name := ""
	if a.filterCol >= 0 && a.filterCol < len(a.lastCols) {
		name = a.lastCols[a.filterCol]
	}
	if term == "" {
		a.SetStatus(fmt.Sprintf("%d rows", len(a.lastData)))
	} else {
		a.SetStatus(fmt.Sprintf("col %s: %d/%d", name, len(rows), len(a.lastData)))
	}
}

// showColFilter opens a filter input scoped to the currently selected Results
// column (f), as opposed to / which matches across all columns.
func (a *App) showColFilter() {
	if len(a.lastData) == 0 {
		a.SetStatus("no results to filter")
		return
	}
	_, col := a.results.GetSelection()
	if col < 0 || col >= len(a.lastCols) {
		return
	}
	a.filterCol = col
	a.filterTarget = filterTgtResultsCol
	a.openFilterInput(fmt.Sprintf(" Filter col %s — Enter: keep · Esc: clear ", a.lastCols[col]), a.filter, a.applyColFilter)
}

// openFilterInput shows the shared single-line filter overlay.
func (a *App) openFilterInput(title, initial string, onChange func(string)) {
	in := tview.NewInputField().SetLabel(" / ").SetText(initial)
	in.SetChangedFunc(onChange)
	in.SetFieldBackgroundColor(a.theme.Field).SetFieldTextColor(a.theme.FieldText)
	in.SetBorder(true).SetTitle(title).SetBorderColor(a.theme.Focus)
	a.pages.AddPage("filter", centered(in, 60, 3), true, true)
	a.app.SetFocus(in)
	a.filterOpen = true
}

// showFilter opens the / filter input for the focused panel. Typing filters
// live; Enter keeps the filter, Esc clears it. The target follows the panel:
// Connections → databases, Schema → tables, Results → rows.
func (a *App) showFilter() {
	var (
		title    string
		initial  string
		onChange func(string)
	)
	switch a.focusIdx {
	case 0: // Connections → databases of the connected (multi-db) connection
		a.filterTarget = filterTgtConn
		if a.connDBNode == nil || len(a.connDatabases) == 0 {
			a.SetStatus("no databases to filter")
			return
		}
		title = " Filter databases — Enter: keep · Esc: clear "
		initial = a.connFilter
		onChange = a.applyConnFilter
	case 1: // Schema → tables
		a.filterTarget = filterTgtSchema
		if len(a.schemaTables) == 0 {
			a.SetStatus("no tables to filter")
			return
		}
		title = " Filter tables — Enter: keep · Esc: clear "
		initial = a.schemaFilter
		onChange = a.applySchemaFilter
	default: // Results → rows
		a.filterTarget = filterTgtResults
		if len(a.lastData) == 0 {
			a.SetStatus("no results to filter")
			return
		}
		title = " Filter results — Enter: keep · Esc: clear "
		initial = a.filter
		onChange = a.applyFilter
	}
	a.openFilterInput(title, initial, onChange)
}

// hideFilter closes the filter overlay. When clear is true the active filter is
// reset (restoring the full table or result set).
func (a *App) hideFilter(clear bool) {
	a.pages.RemovePage("filter")
	a.filterOpen = false
	if clear {
		switch a.filterTarget {
		case filterTgtConn:
			a.applyConnFilter("")
		case filterTgtSchema:
			a.applySchemaFilter("")
		case filterTgtResultsCol:
			a.applyColFilter("")
		default:
			a.applyFilter("")
		}
	}
	a.app.SetFocus(a.navPanels()[a.focusIdx])
}

// fillResults renders columns + rows into the Results table (header fixed).
func (a *App) fillResults(cols []string, data [][]any) {
	a.viewRows = data // retained so a selected table row maps back to its values
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

// loadSchema fetches the tables of the current database — the one picked in the
// Connections tree (currentDB) for multi-database engines, otherwise the
// connection's own database — and renders them (clearing any table filter).
func (a *App) loadSchema() {
	if a.conn == nil {
		return
	}
	a.schemaFilter = ""
	a.schemaTables = nil

	md, ok := a.conn.(driver.Metadata)
	if !ok {
		a.schemaPlaceholder("driver has no metadata")
		return
	}
	var (
		tables []string
		err    error
	)
	if b, multi := a.conn.(databaseBrowser); multi && a.currentDB != "" {
		tables, err = namesFrom(b.TablesIn(context.Background(), a.currentDB))
	} else {
		tables, err = namesFrom(md.Tables(context.Background()))
	}
	if err != nil {
		a.setError("list tables: " + err.Error())
		return
	}
	a.schemaTables = tables
	a.renderSchema()
}

// renderSchema (re)builds the Schema tree from the retained table list, applying
// the active table filter. Selecting a table previews it.
func (a *App) renderSchema() {
	label := a.connID
	if a.currentDB != "" {
		label = a.currentDB
	}
	root := tview.NewTreeNode(label).SetColor(a.theme.Accent)
	a.schema.SetRoot(root)
	for _, name := range filterTables(a.schemaTables, a.schemaFilter) {
		root.AddChild(a.treeNode(name).SetReference(tableRef{db: a.currentDB, name: name}))
	}
	// Root is hidden (SetTopLevel(1)); highlight the first visible table instead.
	if kids := root.GetChildren(); len(kids) > 0 {
		a.schema.SetCurrentNode(kids[0])
	} else {
		a.schema.SetCurrentNode(root)
	}
}

// filterTables keeps the names containing term (case-insensitive); an empty term
// returns all names.
func filterTables(names []string, term string) []string {
	if term == "" {
		return names
	}
	lower := strings.ToLower(term)
	out := make([]string, 0, len(names))
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), lower) {
			out = append(out, n)
		}
	}
	return out
}

// applySchemaFilter re-renders the Schema tree with tables matching term and
// reports the match count.
func (a *App) applySchemaFilter(term string) {
	a.schemaFilter = term
	a.renderSchema()
	if term == "" {
		a.SetStatus(fmt.Sprintf("%d tables", len(a.schemaTables)))
	} else {
		a.SetStatus(fmt.Sprintf("tables %d/%d", len(filterTables(a.schemaTables, term)), len(a.schemaTables)))
	}
}

// schemaPlaceholder shows a one-line hint in the Schema panel (e.g. before a
// database is chosen) and clears the retained table list.
func (a *App) schemaPlaceholder(msg string) {
	a.schemaTables = nil
	a.schemaFilter = ""
	root := tview.NewTreeNode(a.connID).SetColor(a.theme.Accent)
	hint := tview.NewTreeNode("(" + msg + ")")
	root.AddChild(hint)
	// Root is hidden (SetTopLevel(1)); the hint child is the only visible line.
	a.schema.SetRoot(root).SetCurrentNode(hint)
}

// namesFrom collects the first column of rows (a name listing), folding the
// query error in for terse call sites.
func namesFrom(rows driver.Rows, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	return collectFirstColumn(rows)
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

	// While the filter input is open, Enter keeps the filter and Esc clears it;
	// every other key (including j/k) is literal text for the input. Handled
	// before vim-nav so typing isn't translated to arrows.
	if a.filterOpen {
		switch ev.Key() {
		case tcell.KeyEnter:
			a.hideFilter(false)
			return nil
		case tcell.KeyEscape:
			a.hideFilter(true)
			return nil
		}
		return ev
	}

	// While the new/edit-connection form is open, Esc cancels; everything else is
	// handled by the form (field navigation, typing). Before vim-nav so typing
	// stays literal.
	if a.connFormOpen {
		if ev.Key() == tcell.KeyEscape {
			a.hideConnForm()
			return nil
		}
		return ev
	}

	// While the delete-confirmation modal is open, Esc cancels; arrows/Enter go
	// to the modal's buttons.
	if a.confirmOpen {
		if ev.Key() == tcell.KeyEscape {
			a.pages.RemovePage("confirmdel")
			a.confirmOpen = false
			a.app.SetFocus(a.navPanels()[a.focusIdx])
			return nil
		}
		return ev
	}

	// While the export input is open, the InputField's done func handles
	// Enter/Esc; pass everything (incl. j/k) through as literal text.
	if a.exportOpen {
		return ev
	}

	// While the cell-value overlay is open, Esc / v / q close it; other keys
	// (arrows, j/k) scroll the text view.
	if a.cellValueOpen {
		if ev.Key() == tcell.KeyEscape || ev.Rune() == 'v' || ev.Rune() == 'q' {
			a.hideCellValue()
			return nil
		}
		return ev
	}

	// While the cell-edit input is open, the InputField's done func handles
	// Enter/Esc; pass everything (incl. j/k) through as literal text.
	if a.cellEditOpen {
		return ev
	}

	// While the update-confirmation modal is open, Esc cancels; arrows/Enter go
	// to the modal's buttons.
	if a.confirmEditOpen {
		if ev.Key() == tcell.KeyEscape {
			a.pages.RemovePage("confirmedit")
			a.confirmEditOpen = false
			a.app.SetFocus(a.navPanels()[a.focusIdx])
			return nil
		}
		return ev
	}

	// Vim-style navigation: h/j/k/l → Left/Down/Up/Right in any focused widget
	// except the SQL editor (where they are text). Applies in panels (incl.
	// Results cell movement) and in the list overlays.
	if ev.Key() == tcell.KeyRune && a.app.GetFocus() != a.editor {
		switch ev.Rune() {
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		case 'h':
			return tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
		case 'l':
			return tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)
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
	case tcell.KeyCtrlE:
		a.showExport()
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
		case '/':
			a.showFilter()
			return nil
		case 'n':
			// new/edit/delete connection only act when the Connections panel is
			// focused; elsewhere the key falls through (e.g. as filter text).
			if a.focusIdx == 0 {
				a.showConnForm(nil)
				return nil
			}
		case 'e':
			if a.focusIdx == 0 {
				a.editSelectedConn()
				return nil
			}
		case 'd':
			if a.focusIdx == 0 {
				a.confirmDeleteSelectedConn()
				return nil
			}
		case 'v':
			if a.focusIdx == 2 { // Results: view the selected cell's full value
				a.showCellValue()
				return nil
			}
		case 'f':
			if a.focusIdx == 2 { // Results: filter by the selected column
				a.showColFilter()
				return nil
			}
		case 'c':
			if a.focusIdx == 2 { // Results: change (edit) the selected cell
				a.showCellEdit()
				return nil
			}
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
	if a.connClose != nil {
		_ = a.connClose()
		a.connClose = nil
	}
	a.conn = nil
}

// --- testing seams ---

// SetScreen injects a screen (e.g. a tcell SimulationScreen) before Run.
func (a *App) SetScreen(s tcell.Screen) { a.app.SetScreen(s) }

// OnReady registers fn to run exactly once after the first draw, letting tests
// drive the app deterministically without sleeps.
func (a *App) OnReady(fn func()) {
	a.app.SetAfterDrawFunc(func(tcell.Screen) {
		a.ready.Do(func() {
			// Run fn via the event queue rather than inside the afterDraw
			// callback: that callback holds the Application lock, and fn may call
			// SetFocus (which re-locks) — e.g. the Enter drill-down focuses the
			// sub-panel. Queuing makes fn run in the same safe context as real
			// input handlers. A goroutine is needed because QueueUpdateDraw blocks
			// until the (currently drawing) loop drains it.
			go a.app.QueueUpdateDraw(fn)
		})
	})
}

// SendKey queues a rune key event (testing helper).
func (a *App) SendKey(r rune) {
	a.app.QueueEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}
