package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestThemeBorderFocus(t *testing.T) {
	th := Theme{Focus: tcell.ColorGreen, Border: tcell.ColorGray}
	if th.border(true) != tcell.ColorGreen {
		t.Errorf("focused border = %v, want green", th.border(true))
	}
	if th.border(false) != tcell.ColorGray {
		t.Errorf("unfocused border = %v, want gray", th.border(false))
	}
}

func TestNewThemeNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	th := newTheme()
	// Every role collapses to the terminal default so no color is emitted.
	for name, c := range map[string]tcell.Color{
		"Focus": th.Focus, "Border": th.Border, "Accent": th.Accent, "Error": th.Error,
	} {
		if c != tcell.ColorDefault {
			t.Errorf("%s = %v under NO_COLOR, want ColorDefault", name, c)
		}
	}
	if th.tag(th.Accent) != "" || th.reset() != "" {
		t.Errorf("NO_COLOR should yield empty tags, got tag=%q reset=%q", th.tag(th.Accent), th.reset())
	}
}

func TestThemeTag(t *testing.T) {
	th := Theme{Accent: tcell.ColorGreen}
	if got := th.tag(th.Accent); got != "[#008000]" {
		t.Errorf("tag(green) = %q, want [#008000]", got)
	}
	if got := th.tag(tcell.ColorDefault); got != "" {
		t.Errorf("tag(default) = %q, want empty", got)
	}
}

func TestFocusPanelSetsBorderColors(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.focusPanel(2) // Results
	if a.results.GetBorderColor() != a.theme.Focus {
		t.Errorf("focused Results border = %v, want %v", a.results.GetBorderColor(), a.theme.Focus)
	}
	if a.connList.GetBorderColor() != a.theme.Border {
		t.Errorf("unfocused Connections border = %v, want %v", a.connList.GetBorderColor(), a.theme.Border)
	}
}

func TestApplyStylesUsesTerminalBackground(t *testing.T) {
	// New must point tview's global background/foreground at the terminal
	// default so the UI blends in like lazygit (not a solid black box).
	_ = New(Options{Config: sqliteCfg("demo", "x.db")})
	if tview.Styles.PrimitiveBackgroundColor != tcell.ColorDefault {
		t.Errorf("PrimitiveBackgroundColor = %v, want ColorDefault", tview.Styles.PrimitiveBackgroundColor)
	}
	if tview.Styles.PrimaryTextColor != tcell.ColorDefault {
		t.Errorf("PrimaryTextColor = %v, want ColorDefault", tview.Styles.PrimaryTextColor)
	}
}

func TestKeyBarListsShortcuts(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	bar := a.keyBar()
	for _, want := range []string{"Tab", "F5", "filter", "history", "saved", "help", "quit"} {
		if !strings.Contains(bar, want) {
			t.Errorf("keybar missing %q: %s", want, bar)
		}
	}
}
