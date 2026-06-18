package main

import (
	"fmt"
	"io"
)

// helpText is the top-level overview shown by `s9l help` / `-h` / `--help`.
const helpText = `s9l — a fast terminal database client (SQLite, PostgreSQL, MySQL)

Usage:
  s9l <connection|dsn> -e "SQL"     run a query and exit
  s9l <connection|dsn>              start the interactive REPL
  s9l tui [connection]              launch the full-screen TUI
  s9l conn    <list|add|rm>         manage named connections
  s9l history [--limit N]           show recent query history
  s9l saved   <add|list|search|rm|run>   manage and run saved queries
  s9l help | --version

Query flags:
  -e "SQL"            execute SQL and exit (omit to enter the REPL)
  --driver NAME       driver for a bare DSN (default: sqlite)
  --format FMT        output: table | json | csv | tsv (TTY→table, pipe→tsv)
  --max-col-width N   truncate table cells to N runes (0 = unlimited)
  --timeout DUR       abort a query after DUR (e.g. 30s); Ctrl-C also cancels

Connections live in ~/.config/s9l/config.yaml (never plaintext passwords —
use --password to store in the OS keychain, or password_ref env:NAME).

Docs: https://github.com/YangXplorer/s9l
`

// printHelp writes the top-level help.
func printHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, helpText)
	return err
}
