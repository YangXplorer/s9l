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
	Focus         tcell.Color // focused panel border
	Border        tcell.Color // unfocused panel border
	Title         tcell.Color // panel title text
	Accent        tcell.Color // keybar key glyphs / highlights
	Dim           tcell.Color // secondary text
	Error         tcell.Color // error messages
	Selection     tcell.Color // selected row background
	SelectionText tcell.Color // selected row text
	Field         tcell.Color // input field / modal background
	FieldText     tcell.Color // input field / modal text
	Contrast      tcell.Color // fallback contrast background (readable with light text)
}

// newTheme returns the active theme, honoring NO_COLOR.
func newTheme() Theme {
	if noColor() {
		c := tcell.ColorDefault
		return Theme{Focus: c, Border: c, Title: c, Accent: c, Dim: c, Error: c,
			Selection: c, SelectionText: c, Field: c, FieldText: c, Contrast: c}
	}
	return Theme{
		Focus:         tcell.ColorGreen,
		Border:        tcell.ColorGray,
		Title:         tcell.ColorWhite,
		Accent:        tcell.ColorGreen,
		Dim:           tcell.ColorGray,
		Error:         tcell.ColorRed,
		Selection:     tcell.NewRGBColor(0xc0, 0xc0, 0xc0), // light gray bar
		SelectionText: tcell.ColorBlack,
		Field:         tcell.NewRGBColor(0xcf, 0xcf, 0xcf), // light input/modal surface
		FieldText:     tcell.ColorBlack,
		Contrast:      tcell.NewRGBColor(0x30, 0x35, 0x40), // dim slate fallback
	}
}

// selectionStyle is the style for the selected row (light bar + dark text), or
// reverse video under NO_COLOR so the selection stays visible.
func (t Theme) selectionStyle() tcell.Style {
	if noColor() {
		return tcell.StyleDefault.Reverse(true)
	}
	return tcell.StyleDefault.Background(t.Selection).Foreground(t.SelectionText)
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

// applyStyles points tview's global Styles at the terminal's own background and
// foreground so the UI blends into the terminal like lazygit (tview otherwise
// paints a solid black background). Contrast colors (input fields, buttons, the
// delete modal) are left at tview's defaults so they stay visible on the
// now-transparent background.
func (t Theme) applyStyles() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = tcell.ColorDefault
	tview.Styles.BorderColor = t.Border
	tview.Styles.TitleColor = t.Title
	tview.Styles.GraphicsColor = t.Border
	tview.Styles.SecondaryTextColor = t.Dim
	tview.Styles.TertiaryTextColor = t.Accent
	// Fallback contrast surface (a dim slate that stays readable with the light
	// default text); interactive widgets override this with the lighter Field.
	tview.Styles.ContrastBackgroundColor = t.Contrast
	tview.Styles.MoreContrastBackgroundColor = t.Contrast
	if noColor() {
		tview.Styles.InverseTextColor = tcell.ColorDefault
	} else {
		tview.Styles.InverseTextColor = tcell.ColorWhite
	}
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
