package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"
)

// pagerArgs resolves the pager command from the environment using look (an
// os.LookupEnv-shaped function), returning the argv and whether paging is
// enabled. S9L_PAGER overrides PAGER; setting S9L_PAGER to empty/whitespace
// disables paging. With neither set, the default is `less -FIRX` (-F quits if
// the output fits one screen, so small results print inline).
func pagerArgs(look func(string) (string, bool)) ([]string, bool) {
	if v, ok := look("S9L_PAGER"); ok {
		if strings.TrimSpace(v) == "" {
			return nil, false
		}
		return strings.Fields(v), true
	}
	if v, ok := look("PAGER"); ok && strings.TrimSpace(v) != "" {
		return strings.Fields(v), true
	}
	return []string{"less", "-FIRX"}, true
}

// maybePager returns out wrapped through the configured pager when enabled and
// out is a terminal, plus a finish func that closes the pipe and waits for the
// pager to exit. When paging does not apply (disabled, not a TTY, or the pager
// can't start) it returns out unchanged with a no-op finish.
func maybePager(out io.Writer, enabled bool) (io.Writer, func()) {
	noop := func() {}
	if !enabled {
		return out, noop
	}
	f, ok := out.(*os.File)
	if !ok || !isatty.IsTerminal(f.Fd()) {
		return out, noop
	}
	args, on := pagerArgs(os.LookupEnv)
	if !on || len(args) == 0 {
		return out, noop
	}
	cmd := exec.Command(args[0], args[1:]...)
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return out, noop
	}
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return out, noop
	}
	return pipe, func() {
		_ = pipe.Close()
		_ = cmd.Wait()
	}
}

// isBrokenPipe reports whether err is an EPIPE-style write failure, which
// happens when the user quits the pager before all rows are written. That is a
// normal interaction, not a query error.
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
