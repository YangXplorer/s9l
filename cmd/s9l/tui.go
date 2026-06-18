package main

import (
	"github.com/YangXplorer/s9l/internal/history"
	"github.com/YangXplorer/s9l/internal/secret"
	"github.com/YangXplorer/s9l/internal/tui"
)

// runTUI launches the full-screen TUI (Phase T). An optional positional arg is
// the connection to auto-open. The history store is opened best-effort; if it
// fails, the TUI still runs with history features disabled.
func runTUI(args []string) error {
	var conn string
	if len(args) > 0 {
		conn = args[0]
	}
	opts := tui.Options{Conn: conn, Store: secret.Default()}
	if h, err := history.OpenDefault(); err == nil {
		opts.History = h
		defer func() { _ = h.Close() }()
	}
	return tui.New(opts).Run()
}
