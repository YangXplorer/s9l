package tui

import (
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// completionHost wraps the main layout and draws the editor's completion
// popup on top of it. The popup is display-only and never takes focus —
// routing it through Pages would move the focus on every add/remove (Pages
// refocuses its topmost visible page whenever it holds the focus).
type completionHost struct {
	tview.Primitive
	a *App
}

func (c completionHost) Draw(screen tcell.Screen) {
	c.Primitive.Draw(screen)
	if c.a.completionOpen {
		c.a.positionCompletion() // follow the cursor as it moves
		c.a.compList.Draw(screen)
	}
}

const (
	// compAutoMinRunes is the word length that auto-opens the popup while
	// typing; shorter words are noise. Ctrl-Space bypasses the threshold.
	compAutoMinRunes = 2
	// compMaxRows caps the popup height.
	compMaxRows = 8
)

// updateEditorCompletion recomputes the editor's completion popup for the
// current text and cursor. auto applies the minimum-word-length threshold;
// manual invocation (Ctrl-Space) shows whatever is available. Candidates come
// from repl.Completer (keywords + tables + columns); per plan R4 the schema
// source never hits the database while a query is running.
func (a *App) updateEditorCompletion(auto bool) {
	if a.compInserting {
		return
	}
	if a.completer == nil || a.app.GetFocus() != a.editor {
		a.hideCompletion()
		return
	}
	text := a.editor.GetText()
	_, cur, _ := a.editor.GetSelection() // no selection → start == end == cursor (bytes)
	prefix := identifierPrefix(text[:cur])
	if prefix == "" || (auto && utf8.RuneCountInString(prefix) < compAutoMinRunes) {
		a.hideCompletion()
		return
	}
	// The completer speaks rune positions; the editor speaks bytes (R3).
	suffixes, _ := a.completer.Complete(text, utf8.RuneCountInString(text[:cur]))
	cands := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		if s == "" {
			continue // the word is already complete
		}
		cands = append(cands, prefix+s)
	}
	if len(cands) == 0 {
		a.hideCompletion()
		return
	}
	a.showCompletion(cands, len(prefix), cur)
}

// showCompletion opens (or refreshes) the popup. prefixBytes is the byte
// length of the word being completed, which ends at byte offset cur.
func (a *App) showCompletion(cands []string, prefixBytes, cur int) {
	a.compCands = cands
	a.compPrefixBytes = prefixBytes
	a.compCursor = cur
	if a.compList == nil {
		a.compList = tview.NewList().ShowSecondaryText(false)
		a.compList.SetSelectedStyle(a.theme.cellCursorStyle())
		a.compList.SetMainTextColor(a.theme.FieldText)
		a.compList.SetBackgroundColor(a.theme.Surface)
		a.compList.SetBorder(true).SetBorderColor(a.theme.Focus)
	}
	a.compList.Clear()
	for _, c := range cands {
		a.compList.AddItem(c, "", 0, nil)
	}
	a.compList.SetCurrentItem(0)
	a.completionOpen = true // drawn by completionHost on the next frame
}

// positionCompletion places the popup near the cursor inside the editor panel,
// falling back to the panel bottom when there is no room below or above.
func (a *App) positionCompletion() {
	x, y, w, h := a.editor.GetInnerRect()
	width := 0
	for _, c := range a.compCands {
		if cw := runewidth.StringWidth(c); cw > width {
			width = cw
		}
	}
	width = min(width+4, max(w, 1))
	rows := min(len(a.compCands), compMaxRows)
	height := rows + 2 // border

	_, _, curRow, curCol := a.editor.GetCursor()
	offRow, offCol := a.editor.GetOffset()
	cx := min(max(x+curCol-offCol, x), max(x+w-width, x))
	cy := y + (curRow - offRow) + 1 // just under the cursor line
	if cy+height > y+h {
		if above := y + (curRow - offRow) - height; above >= y {
			cy = above
		} else {
			cy = max(y+h-height, y) // dock at the panel bottom
		}
	}
	a.compList.SetRect(cx, cy, width, height)
}

// moveCompletion moves the popup selection by delta, clamped to the list.
func (a *App) moveCompletion(delta int) {
	i := min(max(a.compList.GetCurrentItem()+delta, 0), len(a.compCands)-1)
	a.compList.SetCurrentItem(i)
}

// acceptCompletion replaces the word being typed with the selected candidate
// (byte offsets; Replace keeps the undo stack intact) and closes the popup.
func (a *App) acceptCompletion() {
	i := a.compList.GetCurrentItem()
	if i < 0 || i >= len(a.compCands) {
		a.hideCompletion()
		return
	}
	word := a.compCands[i]
	// The insertion itself fires the editor's changed hook; don't let it
	// reopen the popup mid-replace.
	a.compInserting = true
	a.editor.Replace(a.compCursor-a.compPrefixBytes, a.compCursor, word)
	a.compInserting = false
	a.hideCompletion()
}

// hideCompletion closes the popup. It only flips the draw flag — the popup is
// not a page and holds no focus, so closing it has no side effects.
func (a *App) hideCompletion() {
	a.completionOpen = false
}
