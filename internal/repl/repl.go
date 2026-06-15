// Package repl provides the interactive read-eval-print loop: it reads lines
// from a LineReader, splits them into `;`-terminated statements, and dispatches
// each to an exec callback. The core loop is terminal-independent so it can be
// unit-tested with a slice-backed reader; the cmd layer wires a readline-backed
// reader for real terminals. See docs/TASKS.md C3.
package repl

import (
	"errors"
	"io"
	"strings"
)

// ErrInterrupt signals that the current input line was interrupted (Ctrl-C).
// The loop discards the pending buffer and continues.
var ErrInterrupt = errors.New("repl: interrupted")

// LineReader yields input lines. ReadLine returns io.EOF at end of input and
// ErrInterrupt when the user interrupts the current line.
type LineReader interface {
	ReadLine() (string, error)
}

// ExecFunc runs one complete SQL statement. A returned error is reported but
// does not stop the loop.
type ExecFunc func(sql string) error

// quitCommands end the loop when entered on their own line (no trailing `;`).
var quitCommands = map[string]bool{`\q`: true, "quit": true, "exit": true}

// Loop reads statements until EOF (or a quit command) and runs each via exec.
// Statements are split on `;`. A trailing statement without `;` at EOF is also
// executed. exec errors are written to errOut and the loop continues.
func Loop(lr LineReader, errOut io.Writer, exec ExecFunc) error {
	var buf strings.Builder
	for {
		line, err := lr.ReadLine()
		switch {
		case errors.Is(err, io.EOF):
			if s := strings.TrimSpace(buf.String()); s != "" {
				dispatch(s, errOut, exec)
			}
			return nil
		case errors.Is(err, ErrInterrupt):
			buf.Reset()
			continue
		case err != nil:
			return err
		}

		// A bare quit command (no pending statement buffered) ends the loop.
		if quitCommands[strings.TrimSpace(line)] && strings.TrimSpace(buf.String()) == "" {
			return nil
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
		flushStatements(&buf, errOut, exec)
	}
}

// flushStatements extracts and runs every complete (`;`-terminated) statement
// from buf, leaving any trailing partial statement in place.
func flushStatements(buf *strings.Builder, errOut io.Writer, exec ExecFunc) {
	for {
		before, after, found := strings.Cut(buf.String(), ";")
		if !found {
			return
		}
		stmt := strings.TrimSpace(before)
		buf.Reset()
		buf.WriteString(after)
		if stmt != "" {
			dispatch(stmt, errOut, exec)
		}
	}
}

func dispatch(sql string, errOut io.Writer, exec ExecFunc) {
	if err := exec(sql); err != nil {
		_, _ = io.WriteString(errOut, "s9l: "+err.Error()+"\n")
	}
}
