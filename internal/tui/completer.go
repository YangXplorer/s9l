package tui

import (
	"context"
	"strings"
	"unicode"

	"github.com/YangXplorer/s9l/internal/repl"
	"github.com/YangXplorer/s9l/internal/schemacache"

	"github.com/rivo/tview"
)

// tuiSchema adapts the TUI's state to repl.Schema. It prefers the Schema
// panel's already-loaded table list (which follows the database picked in the
// Connections tree — the connection-level metadata only sees the connected
// database), and it never issues a DB round-trip while a query is running:
// completion shares a.conn with the running query and driver implementations
// don't promise concurrent use (plan R4).
type tuiSchema struct {
	a    *App
	base repl.Schema // metadata-backed cache; nil when the driver lacks Metadata
}

func (s tuiSchema) Tables() []string {
	if len(s.a.schemaTables) > 0 {
		return s.a.schemaTables
	}
	if s.base == nil || s.a.running {
		return nil
	}
	return s.base.Tables()
}

func (s tuiSchema) Columns(table string) []string {
	// The previewed table's columns are already in memory.
	if table == s.a.resultTable.name && len(s.a.lastCols) > 0 {
		return s.a.lastCols
	}
	if s.base == nil || s.a.running {
		return nil
	}
	return s.base.Columns(table)
}

// identifierPrefix returns the identifier being typed at the end of text, or
// "" when the cursor sits inside a single-quoted string literal (no column
// completion there — the user is typing a value). Quote parity handles the ”
// escape naturally (it adds two quotes).
func identifierPrefix(text string) string {
	quotes := 0
	for _, r := range text {
		if r == '\'' {
			quotes++
		}
	}
	if quotes%2 == 1 {
		return ""
	}
	rs := []rune(text)
	start := len(rs)
	for start > 0 && isIdentRune(rs[start-1]) {
		start--
	}
	return string(rs[start:])
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// matchColumns returns the columns matching prefix, case-insensitively:
// prefix matches first, then substring matches. An empty prefix offers
// nothing (a full dropdown on every keystroke is noise).
func matchColumns(cols []string, prefix string) []string {
	if prefix == "" {
		return nil
	}
	p := strings.ToLower(prefix)
	var pre, sub []string
	for _, c := range cols {
		lc := strings.ToLower(c)
		switch {
		case strings.HasPrefix(lc, p):
			pre = append(pre, c)
		case strings.Contains(lc, p):
			sub = append(sub, c)
		}
	}
	return append(pre, sub...)
}

// whereCandidates returns the column-name candidates for the WHERE input given
// the text typed so far. It offers nothing inside string literals and when the
// only match is exactly the word already typed (so adopting a candidate does
// not immediately reopen the list).
func whereCandidates(cols []string, text string) []string {
	pfx := identifierPrefix(text)
	entries := matchColumns(cols, pfx)
	if len(entries) == 1 && strings.EqualFold(entries[0], pfx) {
		return nil
	}
	return entries
}

// attachColumnCompletion offers the preview's column names while a WHERE
// expression is typed. Tab (or Enter/click while the list is showing) adopts
// the selected candidate; acOpen tells onKey to hand Enter/Esc to the input
// while the list shows (Enter otherwise applies the filter).
func (a *App) attachColumnCompletion(in *tview.InputField) {
	in.SetAutocompleteFunc(func(current string) []string {
		entries := whereCandidates(a.lastCols, current)
		a.acOpen = len(entries) > 0
		return entries
	})
	in.SetAutocompletedFunc(func(text string, index, source int) bool {
		if source != tview.AutocompletedNavigate {
			cur := in.GetText()
			prefix := identifierPrefix(cur)
			in.SetText(cur[:len(cur)-len(prefix)] + text)
		}
		done := source == tview.AutocompletedEnter ||
			source == tview.AutocompletedClick ||
			source == tview.AutocompletedTab
		if done {
			a.acOpen = false
		}
		return done
	})
}

// buildCompleter (re)creates the SQL completer for the current connection.
// The persistent schema cache is opened lazily and best-effort: without it (or
// without Metadata support) completion degrades to keywords + loaded tables.
func (a *App) buildCompleter(connID string) {
	if a.compStore == nil {
		a.compStore, _ = schemacache.OpenDefault() // nil store = no persistence
	}
	base := repl.NewSchemaCache(context.Background(), a.conn, a.compStore, connID)
	a.completer = repl.NewCompleter(tuiSchema{a: a, base: base})
}
