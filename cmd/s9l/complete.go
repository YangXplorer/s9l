package main

import (
	"context"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/repl"
	"github.com/YangXplorer/s9l/internal/schemacache"

	"github.com/chzyer/readline"
)

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

// newCompleter builds a readline.AutoCompleter for conn. store/connID enable
// the persistent schema cache (pass nil/"" to disable).
func newCompleter(ctx context.Context, conn driver.Conn, store *schemacache.Store, connID string) readline.AutoCompleter {
	schema := repl.NewSchemaCache(ctx, conn, store, connID)
	return completerAdapter{c: repl.NewCompleter(schema)}
}
