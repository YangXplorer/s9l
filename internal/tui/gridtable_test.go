package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// A bordered tview.Table paints a highlighted cell's background over the grid
// lines around it; gridTable must repaint every grid glyph with the plain
// border-on-background style so highlights stay inside their cell frame.
func TestGridTableKeepsGridLinesClean(t *testing.T) {
	s := tcell.NewSimulationScreen("")
	if err := s.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	s.SetSize(40, 12)

	th := newTheme() // colors on
	gt := &gridTable{
		Table: tview.NewTable().SetBorders(true),
		grid:  th.Border,
		bg:    th.Background,
	}
	// An opaque highlighted cell — without the repaint its background would
	// spill onto the surrounding grid lines (tview paints the cell box over
	// them). A second row guarantees inner grid joints exist.
	gt.SetCell(0, 0, tview.NewTableCell("aaaa").SetStyle(th.selectionStyle()).SetTransparency(false))
	gt.SetCell(0, 1, tview.NewTableCell("bbbb"))
	gt.SetCell(1, 0, tview.NewTableCell("cccc"))
	gt.SetCell(1, 1, tview.NewTableCell("dddd"))
	gt.SetRect(0, 0, 40, 12)
	gt.Draw(s)

	found := 0
	for ry := range 12 {
		for rx := range 40 {
			str, style, _ := s.Get(rx, ry)
			if !gridRunes[str] {
				continue
			}
			found++
			_, bg, _ := style.Decompose()
			if bg != th.Background {
				t.Fatalf("grid glyph %q at (%d,%d) has background %v, want %v (highlight bled onto the grid)",
					str, rx, ry, bg, th.Background)
			}
		}
	}
	if found == 0 {
		t.Fatal("no grid glyphs drawn — bordered table expected")
	}
}
