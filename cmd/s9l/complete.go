package main

import (
	"context"
	"fmt"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/repl"

	"github.com/chzyer/readline"
)

// schemaCache implements repl.Schema over a driver's Metadata capability,
// caching the table list and each table's columns so completion only hits the
// database once per name. It is not safe for concurrent use; readline calls it
// from a single input goroutine.
type schemaCache struct {
	ctx context.Context
	md  driver.Metadata

	tables       []string
	tablesLoaded bool
	columns      map[string][]string
}

// newSchemaCache returns a repl.Schema for conn, or nil if conn lacks the
// Metadata capability (completion then falls back to keywords only).
func newSchemaCache(ctx context.Context, conn driver.Conn) repl.Schema {
	md, ok := conn.(driver.Metadata)
	if !ok {
		return nil
	}
	return &schemaCache{ctx: ctx, md: md, columns: map[string][]string{}}
}

func (s *schemaCache) Tables() []string {
	if !s.tablesLoaded {
		s.tablesLoaded = true // cache even on error to avoid refetch storms
		rows, err := s.md.Tables(s.ctx)
		if err == nil {
			s.tables, _ = collectFirstCol(rows)
		}
	}
	return s.tables
}

func (s *schemaCache) Columns(table string) []string {
	if cols, ok := s.columns[table]; ok {
		return cols
	}
	var cols []string
	rows, err := s.md.Columns(s.ctx, table)
	if err == nil {
		cols, _ = collectFirstCol(rows)
	}
	s.columns[table] = cols // cache nil on error too
	return cols
}

// collectFirstCol reads the first column of every row as a string, closing the
// rows. Metadata listings put the name (table/column) in the first column.
func collectFirstCol(rows driver.Rows) ([]string, error) {
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return out, err
		}
		if len(vals) > 0 {
			out = append(out, fmt.Sprintf("%v", vals[0]))
		}
	}
	return out, rows.Err()
}

// completerAdapter bridges repl.Completer to readline.AutoCompleter.
type completerAdapter struct{ c *repl.Completer }

func (a completerAdapter) Do(line []rune, pos int) ([][]rune, int) {
	suffixes, prefixLen := a.c.Complete(string(line[:pos]), pos)
	out := make([][]rune, len(suffixes))
	for i, s := range suffixes {
		out[i] = []rune(s)
	}
	return out, prefixLen
}

// newCompleter builds a readline.AutoCompleter for conn, or nil when no
// completion source is available (so readline keeps its default behavior).
func newCompleter(ctx context.Context, conn driver.Conn) readline.AutoCompleter {
	schema := newSchemaCache(ctx, conn)
	return completerAdapter{c: repl.NewCompleter(schema)}
}
