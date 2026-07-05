package render

import (
	"encoding/hex"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Cell formats a single value for human-facing display: NULL for nil, and a
// hex literal (0x…) for binary values. Drivers normalize []byte to string, so
// binary columns (e.g. MySQL binary(16) UUIDs) arrive as invalid-UTF-8 strings
// that would otherwise print as mojibake. Machine formats (csv/tsv/json) do
// not use this — they keep the raw data.
func Cell(v any) string {
	switch x := v.(type) {
	case nil:
		return nullText
	case []byte:
		if printableText(string(x)) {
			return string(x)
		}
		return "0x" + hex.EncodeToString(x)
	case string:
		if printableText(x) {
			return x
		}
		return "0x" + hex.EncodeToString([]byte(x))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// printableText reports whether s is valid UTF-8 made of printable runes
// (plus \n, \r, \t), i.e. safe to show as text in a terminal table.
func printableText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
