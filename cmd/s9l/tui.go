package main

import "github.com/YangXplorer/s9l/internal/tui"

// runTUI launches the full-screen TUI (Phase T). An optional positional arg is
// the connection to auto-open (wired up in T-1a).
func runTUI(args []string) error {
	var conn string
	if len(args) > 0 {
		conn = args[0]
	}
	return tui.New(tui.Options{Conn: conn}).Run()
}
