package repl

import (
	"sort"
	"strings"
)

// Schema supplies names for SQL completion. Implementations are expected to
// cache results (metadata lookups are relatively expensive); all methods must
// be safe to call repeatedly and may return nil when unavailable.
type Schema interface {
	// Tables returns the table names in the current database.
	Tables() []string
	// Columns returns the column names of the given table.
	Columns(table string) []string
}

// Completer offers word completions for the REPL: backslash meta-commands, SQL
// keywords, table names, and column names (including qualified table.column and
// columns of tables referenced in the current line). It is terminal-independent
// so it can be unit-tested; cmd wires it to readline's AutoCompleter.
type Completer struct {
	schema Schema
}

// NewCompleter returns a Completer backed by schema (which may be nil, in which
// case only keywords and meta-commands are offered).
func NewCompleter(schema Schema) *Completer {
	return &Completer{schema: schema}
}

// metaCommands are the backslash commands offered when the word starts with \.
var metaCommands = []string{`\l`, `\dt`, `\d`, `\?`, `\q`}

// sqlKeywords is a small, dialect-neutral set of common keywords. It is not
// exhaustive — just enough to speed up typing the usual statements.
var sqlKeywords = []string{
	"SELECT", "FROM", "WHERE", "GROUP BY", "ORDER BY", "HAVING", "LIMIT",
	"OFFSET", "INSERT INTO", "VALUES", "UPDATE", "SET", "DELETE", "JOIN",
	"LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "ON", "AS", "AND", "OR", "NOT",
	"NULL", "IS NULL", "IS NOT NULL", "DISTINCT", "COUNT", "CREATE TABLE",
	"DROP TABLE", "ALTER TABLE", "INDEX", "BETWEEN", "LIKE", "IN", "ASC", "DESC",
}

// Complete returns the candidate completions (as the suffix to append after the
// already-typed prefix) for the word ending at pos in line, together with the
// rune length of that prefix. The contract matches readline.AutoCompleter.Do:
// the prefix length is used only for display alignment.
func (c *Completer) Complete(line string, pos int) ([]string, int) {
	runes := []rune(line)
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos < 0 {
		pos = 0
	}
	start := pos
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	word := string(runes[start:pos])

	// Backslash meta-commands.
	if strings.HasPrefix(word, `\`) {
		return matchSuffixes(word, metaCommands), len([]rune(word))
	}

	// Qualified name: table.<colPrefix> completes that table's columns.
	if dot := strings.LastIndex(word, "."); dot >= 0 {
		table := word[:dot]
		colPrefix := word[dot+1:]
		return matchSuffixes(colPrefix, c.columns(table)), len([]rune(colPrefix))
	}

	// Plain word: keywords + table names + columns of referenced tables.
	cands := make([]string, 0, len(sqlKeywords))
	cands = append(cands, sqlKeywords...)
	tables := c.tables()
	cands = append(cands, tables...)
	for _, t := range referencedTables(line, tables) {
		cands = append(cands, c.columns(t)...)
	}
	return matchSuffixes(word, cands), len([]rune(word))
}

func (c *Completer) tables() []string {
	if c.schema == nil {
		return nil
	}
	return c.schema.Tables()
}

func (c *Completer) columns(table string) []string {
	if c.schema == nil {
		return nil
	}
	return c.schema.Columns(table)
}

// isWordRune reports whether r can be part of a completion word: identifier
// characters plus '.' (qualified names) and '\' (meta-commands).
func isWordRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '_' || r == '.' || r == '\\':
		return true
	default:
		return false
	}
}

// referencedTables returns the known tables that appear as whole words in line,
// so their columns can be offered as completions. Matching is case-insensitive.
func referencedTables(line string, tables []string) []string {
	if len(tables) == 0 {
		return nil
	}
	lower := strings.ToLower(line)
	var out []string
	for _, t := range tables {
		if containsWord(lower, strings.ToLower(t)) {
			out = append(out, t)
		}
	}
	return out
}

// containsWord reports whether word appears in s delimited by non-word runes.
func containsWord(s, word string) bool {
	if word == "" {
		return false
	}
	from := 0
	for {
		i := strings.Index(s[from:], word)
		if i < 0 {
			return false
		}
		i += from
		beforeOK := i == 0 || !isWordRune(rune(s[i-1]))
		end := i + len(word)
		afterOK := end == len(s) || !isWordRune(rune(s[end]))
		if beforeOK && afterOK {
			return true
		}
		from = i + 1
	}
}

// matchSuffixes returns, for every candidate that has prefix (case-insensitive)
// and is longer than it, the part of the candidate after the prefix. Results
// are de-duplicated and sorted for stable display.
func matchSuffixes(prefix string, candidates []string) []string {
	pr := []rune(prefix)
	lowPrefix := strings.ToLower(prefix)
	seen := make(map[string]bool)
	var out []string
	for _, cand := range candidates {
		cr := []rune(cand)
		if len(cr) <= len(pr) {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(cand), lowPrefix) {
			continue
		}
		suffix := string(cr[len(pr):])
		if seen[suffix] {
			continue
		}
		seen[suffix] = true
		out = append(out, suffix)
	}
	sort.Strings(out)
	return out
}
