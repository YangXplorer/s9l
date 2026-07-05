package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// gridRunes are the box-drawing glyphs a bordered tview.Table draws between
// cells (including the rounded corners from useRoundedBorders).
var gridRunes = map[string]bool{
	"─": true, "│": true, "┼": true,
	"├": true, "┤": true, "┬": true, "┴": true,
	"┌": true, "┐": true, "└": true, "┘": true,
	"╭": true, "╮": true, "╰": true, "╯": true,
}

// gridTable is a bordered Table whose grid lines stay clean: tview paints a
// highlighted cell's background over the border lines around it (the cell box
// spans them), so after drawing, every grid glyph is repainted with the plain
// border-on-background style. Highlights then read as strictly inside their
// cell frame.
type gridTable struct {
	*tview.Table
	grid, bg tcell.Color
}

func (t *gridTable) Draw(screen tcell.Screen) {
	t.Table.Draw(screen)
	style := tcell.StyleDefault.Foreground(t.grid).Background(t.bg)
	x, y, w, h := t.GetInnerRect()
	for ry := y; ry < y+h; ry++ {
		for rx := x; rx < x+w; rx++ {
			if s, _, _ := screen.Get(rx, ry); gridRunes[s] {
				screen.Put(rx, ry, s, style)
			}
		}
	}
}
