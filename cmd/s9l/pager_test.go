package main

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPagerArgs(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
		on   bool
	}{
		{"default", nil, []string{"less", "-FIRX"}, true},
		{"pager env", map[string]string{"PAGER": "more"}, []string{"more"}, true},
		{"pager with flags", map[string]string{"PAGER": "less -R"}, []string{"less", "-R"}, true},
		{"s9l overrides pager", map[string]string{"S9L_PAGER": "bat", "PAGER": "less"}, []string{"bat"}, true},
		{"s9l empty disables", map[string]string{"S9L_PAGER": "", "PAGER": "less"}, nil, false},
		{"s9l whitespace disables", map[string]string{"S9L_PAGER": "   "}, nil, false},
		{"empty pager falls back to default", map[string]string{"PAGER": ""}, []string{"less", "-FIRX"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			look := func(k string) (string, bool) {
				v, ok := tc.env[k]
				return v, ok
			}
			got, on := pagerArgs(look)
			if on != tc.on {
				t.Errorf("enabled = %v, want %v", on, tc.on)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMaybePagerBypassesNonTTY(t *testing.T) {
	var buf bytes.Buffer
	// A bytes.Buffer is not a *os.File terminal, so paging never applies even
	// when enabled: the writer is returned unchanged and finish is a no-op.
	w, finish := maybePager(&buf, true)
	if w != io.Writer(&buf) {
		t.Error("expected the original writer for a non-TTY destination")
	}
	finish() // must not panic
}

func TestMaybePagerDisabled(t *testing.T) {
	var buf bytes.Buffer
	w, finish := maybePager(&buf, false)
	if w != io.Writer(&buf) {
		t.Error("expected the original writer when paging is disabled")
	}
	finish()
}

// TestRunNoPagerFlag checks the --no-pager flag is accepted and the query
// renders normally to a (non-TTY) buffer.
func TestRunNoPagerFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "app.db")
	var out bytes.Buffer
	if err := run([]string{"--no-pager", dbPath, "-e", "select 42 as answer"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("run --no-pager: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("42")) {
		t.Errorf("expected query output, got:\n%s", out.String())
	}
}

func TestIsBrokenPipe(t *testing.T) {
	if !isBrokenPipe(fmt.Errorf("write: %w", syscall.EPIPE)) {
		t.Error("EPIPE-wrapped error should be a broken pipe")
	}
	if isBrokenPipe(fmt.Errorf("some other error")) {
		t.Error("unrelated error should not be a broken pipe")
	}
	if isBrokenPipe(nil) {
		t.Error("nil should not be a broken pipe")
	}
}
