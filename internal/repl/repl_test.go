package repl_test

import (
	"io"
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/repl"
	"github.com/google/go-cmp/cmp"
)

// sliceReader is a LineReader backed by a slice of (line, err) steps.
type sliceReader struct {
	lines []string
	errs  []error
	i     int
}

func (r *sliceReader) ReadLine() (string, error) {
	if r.i >= len(r.lines) {
		return "", io.EOF
	}
	line, err := r.lines[r.i], r.errs[r.i]
	r.i++
	return line, err
}

func newReader(lines ...string) *sliceReader {
	return &sliceReader{lines: lines, errs: make([]error, len(lines))}
}

func collectExec(got *[]string) repl.ExecFunc {
	return func(sql string) error {
		*got = append(*got, sql)
		return nil
	}
}

func TestLoopSplitsStatements(t *testing.T) {
	var got []string
	lr := newReader("select 1; select 2;", "select 3;")
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	want := []string{"select 1", "select 2", "select 3"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("statements mismatch (-want +got):\n%s", diff)
	}
}

func TestLoopMultiLineStatement(t *testing.T) {
	var got []string
	lr := newReader("select a,", "       b", "from t;")
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "select a,") || !strings.Contains(got[0], "from t") {
		t.Fatalf("expected one multi-line statement, got %#v", got)
	}
}

func TestLoopTrailingStatementNoSemicolon(t *testing.T) {
	var got []string
	lr := newReader("select 1") // EOF without ';'
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	if diff := cmp.Diff([]string{"select 1"}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestLoopQuitCommand(t *testing.T) {
	var got []string
	lr := newReader(`\q`, "select 1;") // should stop at \q, never run select 1
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no statements after quit, got %#v", got)
	}
}

func TestLoopQuitAfterStatement(t *testing.T) {
	// Regression: a flushed statement leaves a trailing newline in the buffer;
	// a following bare \q must still be recognized as quit.
	var got []string
	lr := newReader("select 1;", `\q`, "select 2;")
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	if diff := cmp.Diff([]string{"select 1"}, got); diff != "" {
		t.Errorf("expected only select 1 before quit (-want +got):\n%s", diff)
	}
}

func TestLoopBackslashLineDispatch(t *testing.T) {
	// Backslash commands are line-oriented: dispatched on Enter, no `;` needed.
	var got []string
	lr := newReader(`\dt`, "select 1;")
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	want := []string{`\dt`, "select 1"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestLoopInterruptDiscardsBuffer(t *testing.T) {
	var got []string
	lr := &sliceReader{
		lines: []string{"select bad", "", "select 1;"},
		errs:  []error{nil, repl.ErrInterrupt, nil},
	}
	if err := repl.Loop(lr, io.Discard, collectExec(&got)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	// The interrupted "select bad" buffer is discarded; only "select 1" runs.
	if diff := cmp.Diff([]string{"select 1"}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestLoopExecErrorContinues(t *testing.T) {
	var errOut strings.Builder
	var ran int
	exec := func(sql string) error {
		ran++
		if ran == 1 {
			return io.ErrUnexpectedEOF
		}
		return nil
	}
	lr := newReader("select bad; select ok;")
	if err := repl.Loop(lr, &errOut, exec); err != nil {
		t.Fatalf("loop: %v", err)
	}
	if ran != 2 {
		t.Fatalf("expected loop to continue after error, ran=%d", ran)
	}
	if !strings.Contains(errOut.String(), "s9l:") {
		t.Errorf("expected error reported to errOut, got %q", errOut.String())
	}
}
