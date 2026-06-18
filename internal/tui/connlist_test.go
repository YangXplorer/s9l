package tui

import (
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
)

func TestConnDisplayName(t *testing.T) {
	if got := connDisplayName(config.ConnectionConfig{ID: "pg"}); got != "pg" {
		t.Errorf("no name → id: got %q, want pg", got)
	}
	if got := connDisplayName(config.ConnectionConfig{ID: "pg", Name: "Dev"}); got != "Dev" {
		t.Errorf("with name: got %q, want Dev", got)
	}
}

func TestConnIconAscii(t *testing.T) {
	t.Setenv("S9L_TUI_ICONS", "") // default = ascii
	cases := map[string]string{
		"postgres": "[pg] ", "mysql": "[my] ", "sqlite": "[sq] ",
		"sqlserver": "[ms] ", "unknown": "[db] ",
	}
	for driver, want := range cases {
		if got := connIcon(driver); got != want {
			t.Errorf("connIcon(%q) = %q, want %q", driver, got, want)
		}
	}
}

func TestConnIconOff(t *testing.T) {
	t.Setenv("S9L_TUI_ICONS", "off")
	if got := connIcon("postgres"); got != "" {
		t.Errorf("icons off: got %q, want empty", got)
	}
}

func TestConnIconNerd(t *testing.T) {
	t.Setenv("S9L_TUI_ICONS", "nerd")
	if got := connIcon("postgres"); got == "" || got == "[pg] " {
		t.Errorf("nerd mode should yield a glyph, got %q", got)
	}
	// Unknown driver falls back to the generic glyph.
	if got := connIcon("unknown"); got != genericDBGlyph {
		t.Errorf("nerd unknown = %q, want generic glyph", got)
	}
}

func TestConnListShowsIconAndName(t *testing.T) {
	t.Setenv("S9L_TUI_ICONS", "") // ascii
	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{ID: "pg", Name: "Dev Postgres", Driver: "postgres"},
	}}
	a := New(Options{Config: cfg})
	main, _ := a.connList.GetItemText(0)
	if !strings.HasPrefix(main, "[pg] ") || !strings.Contains(main, "Dev Postgres") {
		t.Errorf("conn row = %q, want icon + display name", main)
	}
}
