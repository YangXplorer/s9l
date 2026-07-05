package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// defaultCellStyle mirrors tview.NewTableCell's initial style, i.e. what a
// results cell must return to once its row loses the highlight bar.
func defaultCellStyle() tcell.Style {
	return tcell.StyleDefault.
		Foreground(tview.Styles.PrimaryTextColor).
		Background(tview.Styles.PrimitiveBackgroundColor)
}

func TestFillResultsHighlightsSelectedRow(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.setResults([]string{"id", "name"}, [][]any{{1, "alice"}, {2, "bob"}})

	if a.hlRow != 1 {
		t.Fatalf("hlRow = %d after fill, want 1", a.hlRow)
	}
	sel := a.theme.selectionStyle()
	for c := range 2 {
		if got := a.results.GetCell(1, c).Style; got != sel {
			t.Errorf("row 1 col %d style = %v, want selection bar %v", c, got, sel)
		}
	}
	// The header row never carries the bar.
	if got := a.results.GetCell(0, 0).Style; got == sel {
		t.Error("header cell must not get the row-highlight bar")
	}
}

func TestMoveSelectionRepaintsRowBar(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.setResults([]string{"id", "name"}, [][]any{{1, "alice"}, {2, "bob"}})

	a.results.Select(2, 1)
	if a.hlRow != 2 {
		t.Fatalf("hlRow = %d after move, want 2", a.hlRow)
	}
	sel, def := a.theme.selectionStyle(), defaultCellStyle()
	for c := range 2 {
		if got := a.results.GetCell(1, c).Style; got != def {
			t.Errorf("old row 1 col %d style = %v, want default %v", c, got, def)
		}
		if got := a.results.GetCell(2, c).Style; got != sel {
			t.Errorf("row 2 col %d style = %v, want selection bar %v", c, got, sel)
		}
	}
}

func TestFilterRerenderKeepsRowBar(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.setResults([]string{"id", "name"}, [][]any{{1, "alice"}, {2, "bob"}, {3, "alina"}})
	a.results.Select(3, 0)

	a.applyFilter("ali") // re-renders with alice + alina; selection clamps to row 2
	if a.hlRow != 2 {
		t.Fatalf("hlRow = %d after filter re-render, want 2", a.hlRow)
	}
	if got := a.results.GetCell(2, 0).Style; got != a.theme.selectionStyle() {
		t.Errorf("filtered row style = %v, want selection bar", got)
	}
}

func TestEmptyResultsClearsRowBar(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.setResults([]string{"id", "name"}, [][]any{{1, "alice"}})
	a.setResults([]string{"id"}, nil)
	if a.hlRow != 0 {
		t.Errorf("hlRow = %d with empty results, want 0", a.hlRow)
	}
}
