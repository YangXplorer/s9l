package tui

import (
	"os"
	"strings"

	"github.com/YangXplorer/s9l/internal/config"
)

// connDisplayName is the label shown for a connection: its display name when
// set, otherwise its id.
func connDisplayName(cc config.ConnectionConfig) string {
	if cc.Name != "" {
		return cc.Name
	}
	return cc.ID
}

// iconMode resolves the connection-icon style from $S9L_TUI_ICONS:
//   - "off"/"none"/"0" -> no icon
//   - "nerd"           -> Nerd Font glyphs (needs a patched font)
//   - anything else    -> short ASCII tags (default; always renders & aligns)
func iconMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("S9L_TUI_ICONS"))) {
	case "off", "none", "0":
		return "off"
	case "nerd":
		return "nerd"
	default:
		return "ascii"
	}
}

// asciiIcons are short, always-rendering type tags (the safe default).
var asciiIcons = map[string]string{
	"postgres":  "[pg] ",
	"mysql":     "[my] ",
	"sqlite":    "[sq] ",
	"sqlserver": "[ms] ",
}

// genericDBGlyph is the Nerd Font fallback (nf-fa-database) for drivers without
// a dedicated glyph. Codepoints are written as escapes so the source stays
// ASCII; they only render with a patched Nerd Font.
const genericDBGlyph = "\uf1c0 "

// nerdIcons are Nerd Font devicon glyphs; only used when S9L_TUI_ICONS=nerd.
var nerdIcons = map[string]string{
	"postgres":  "\ue76e ", // nf-dev-postgresql
	"mysql":     "\ue704 ", // nf-dev-mysql
	"sqlite":    "\ue7c4 ", // nf-dev-sqllite
	"sqlserver": genericDBGlyph,
}

// connIcon returns the icon prefix for a driver under the active icon mode.
func connIcon(driver string) string {
	switch iconMode() {
	case "off":
		return ""
	case "nerd":
		if g, ok := nerdIcons[driver]; ok {
			return g
		}
		return genericDBGlyph
	default:
		if t, ok := asciiIcons[driver]; ok {
			return t
		}
		return "[db] "
	}
}
