package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/render"
	"github.com/YangXplorer/s9l/internal/repl"
	"github.com/YangXplorer/s9l/internal/schemacache"

	"github.com/chzyer/readline"
	"github.com/mattn/go-isatty"
)

const replPrompt = "s9l> "

// runREPL opens the connection once and runs the interactive loop, reusing the
// connection for every statement and recording each in history.
func runREPL(ctx context.Context, in io.Reader, out, errOut io.Writer, target, driverFlag string, opts render.Options, timeout time.Duration, usePager bool) error {
	conn, _, closeConn, err := openTarget(ctx, target, driverFlag)
	if err != nil {
		return err
	}
	defer func() { _ = closeConn() }()

	// Persistent schema cache (best-effort): speeds up completion across
	// sessions and keeps it usable if a live metadata lookup fails. Only named
	// connections are cached; a bare DSN may embed a password.
	store, _ := schemacache.OpenDefault()
	if store != nil {
		defer func() { _ = store.Close() }()
	}

	lr, closeLR, err := newLineReader(in, newCompleter(ctx, conn, store, namedConnID(target)))
	if err != nil {
		return err
	}
	defer closeLR()

	exec := func(sql string) error {
		// Per-statement context: Ctrl-C cancels the running query without
		// quitting the REPL; an optional --timeout bounds it.
		qctx, cancel := queryContext(ctx, timeout)
		defer cancel()
		start := time.Now()
		rowCount, qerr := runStatementPaged(qctx, out, conn, sql, opts, usePager)
		qerr = classifyErr(qerr, timeout)
		recordHistory(errOut, target, sql, time.Since(start), rowCount, qerr)
		return qerr
	}
	return repl.Loop(lr, errOut, exec)
}

// namedConnID returns target when it names a configured connection (a stable
// id safe to use as a cache key), otherwise "" (bare DSN — not persisted).
func namedConnID(target string) string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	if _, ok := cfg.Get(target); ok {
		return target
	}
	return ""
}

// newLineReader returns a readline-backed reader for an interactive terminal,
// otherwise a scanner over in (pipes, tests). completer wires Tab completion on
// the terminal path; it is ignored for the scanner path.
func newLineReader(in io.Reader, completer readline.AutoCompleter) (repl.LineReader, func(), error) {
	if f, ok := in.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		rl, err := readline.NewEx(&readline.Config{
			Prompt:          replPrompt,
			InterruptPrompt: "^C",
			EOFPrompt:       `\q`,
			Stdin:           f,
			AutoComplete:    completer,
		})
		if err != nil {
			return nil, func() {}, err
		}
		return &readlineReader{rl: rl}, func() { _ = rl.Close() }, nil
	}
	return &scannerReader{sc: bufio.NewScanner(in)}, func() {}, nil
}

type scannerReader struct{ sc *bufio.Scanner }

func (s *scannerReader) ReadLine() (string, error) {
	if s.sc.Scan() {
		return s.sc.Text(), nil
	}
	if err := s.sc.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

type readlineReader struct{ rl *readline.Instance }

func (r *readlineReader) ReadLine() (string, error) {
	line, err := r.rl.Readline()
	switch {
	case err == readline.ErrInterrupt:
		return "", repl.ErrInterrupt
	case err == io.EOF:
		return "", io.EOF
	case err != nil:
		return "", err
	}
	return line, nil
}
