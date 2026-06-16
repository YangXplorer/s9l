package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/render"
	"github.com/YangXplorer/s9l/internal/repl"

	"github.com/chzyer/readline"
	"github.com/mattn/go-isatty"
)

const replPrompt = "s9l> "

// runREPL opens the connection once and runs the interactive loop, reusing the
// connection for every statement and recording each in history.
func runREPL(ctx context.Context, in io.Reader, out, errOut io.Writer, target, driverFlag string, opts render.Options, timeout time.Duration) error {
	drv, dsn, err := resolveTarget(target, driverFlag)
	if err != nil {
		return err
	}
	conn, err := driver.Open(ctx, drv, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	lr, closeLR, err := newLineReader(in)
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
		rowCount, qerr := runStatement(qctx, out, conn, sql, opts)
		qerr = classifyErr(qerr, timeout)
		recordHistory(errOut, target, sql, time.Since(start), rowCount, qerr)
		return qerr
	}
	return repl.Loop(lr, errOut, exec)
}

// newLineReader returns a readline-backed reader for an interactive terminal,
// otherwise a scanner over in (pipes, tests).
func newLineReader(in io.Reader) (repl.LineReader, func(), error) {
	if f, ok := in.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		rl, err := readline.NewEx(&readline.Config{
			Prompt:          replPrompt,
			InterruptPrompt: "^C",
			EOFPrompt:       `\q`,
			Stdin:           f,
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
