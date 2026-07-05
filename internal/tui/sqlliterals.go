package tui

import (
	"strings"
	"unicode"
)

// sqlserverNLiterals prefixes single-quoted literals containing non-ASCII
// characters with N so SQL Server treats them as Unicode. Without N, a literal
// like '楊' is coerced to the database's default (often Latin1) collation and
// silently becomes '?', matching nothing. Escaped quotes (”) stay part of
// their literal; existing N'…' literals are left alone.
func sqlserverNLiterals(expr string) string {
	rs := []rune(expr)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\'' {
			b.WriteRune(rs[i])
			continue
		}
		// Find the literal's end, treating '' as an escaped quote. An
		// unterminated literal runs to the end of the expression.
		j := i + 1
		for j < len(rs) {
			if rs[j] == '\'' {
				if j+1 < len(rs) && rs[j+1] == '\'' {
					j += 2
					continue
				}
				break
			}
			j++
		}
		if j >= len(rs) {
			j = len(rs) - 1
		}
		lit := string(rs[i : j+1])
		if !hasNPrefix(rs, i) && hasNonASCII(lit) {
			b.WriteRune('N')
		}
		b.WriteString(lit)
		i = j
	}
	return b.String()
}

// hasNPrefix reports whether the quote at rs[i] is already N-prefixed (a
// standalone N, not the tail of an identifier).
func hasNPrefix(rs []rune, i int) bool {
	if i == 0 || (rs[i-1] != 'N' && rs[i-1] != 'n') {
		return false
	}
	if i >= 2 {
		r := rs[i-2]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	return true
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}
