package tui

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Theme holds the TUI color scheme. When NO_COLOR is set the colors collapse to
// the terminal default so the UI stays readable on monochrome terminals.
type Theme struct {
	Focus     tcell.Color // focused panel border
	Border    tcell.Color // unfocused panel border
	Title     tcell.Color // panel title text
	Accent    tcell.Color // keybar key glyphs / highlights
	Dim       tcell.Color // secondary text
	Error     tcell.Color // error messages
	Selection tcell.Color // selected list/table row
}

// newTheme returns the active theme, honoring NO_COLOR.
func newTheme() Theme {
	if noColor() {
		c := tcell.ColorDefault
		return Theme{Focus: c, Border: c, Title: c, Accent: c, Dim: c, Error: c, Selection: c}
	}
	return Theme{
		Focus:     tcell.ColorGreen,
		Border:    tcell.ColorGray,
		Title:     tcell.ColorWhite,
		Accent:    tcell.ColorGreen,
		Dim:       tcell.ColorGray,
		Error:     tcell.ColorRed,
		Selection: tcell.ColorTeal,
	}
}

// noColor reports whether color output should be suppressed (NO_COLOR, see
// https://no-color.org).
func noColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

// border returns the focused or unfocused border color for the theme.
func (t Theme) border(focused bool) tcell.Color {
	if focused {
		return t.Focus
	}
	return t.Border
}

// tag returns a tview color tag (e.g. "[#00ff00]") for c, or "" for the
// terminal default so NO_COLOR output carries no color tags.
func (t Theme) tag(c tcell.Color) string {
	if c == tcell.ColorDefault {
		return ""
	}
	return fmt.Sprintf("[#%06x]", c.Hex())
}

// reset returns the tview reset tag, or "" under NO_COLOR.
func (t Theme) reset() string {
	if noColor() {
		return ""
	}
	return "[-]"
}

// useRoundedBorders switches tview's global box-drawing runes to rounded
// corners (both normal and focus variants) for a softer, lazygit-like frame.
// Focus is conveyed by border color, not by switching to double lines.
func useRoundedBorders() {
	tview.Borders.TopLeft = '╭'
	tview.Borders.TopRight = '╮'
	tview.Borders.BottomLeft = '╰'
	tview.Borders.BottomRight = '╯'
	tview.Borders.TopLeftFocus = '╭'
	tview.Borders.TopRightFocus = '╮'
	tview.Borders.BottomLeftFocus = '╰'
	tview.Borders.BottomRightFocus = '╯'
	// Use single lines for the focused frame too (color marks focus).
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
}
