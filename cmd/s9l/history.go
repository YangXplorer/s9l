package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/YangXplorer/s9l/internal/history"
)

// runHistory implements `s9l history [--limit N]`, listing recent queries.
func runHistory(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l history", flag.ContinueOnError)
	fs.SetOutput(errOut)
	limit := fs.Int("limit", 20, "max entries to show (0 = all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	entries, err := store.ListHistory(context.Background(), *limit)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(out, "no query history")
		return err
	}
	for _, e := range entries {
		status := "ok"
		if !e.Success {
			status = "ERR"
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%dms\t%s\t%s\n",
			e.ExecutedAt.Local().Format("2006-01-02 15:04:05"),
			status, e.Duration.Milliseconds(), e.ConnectionID, singleLine(e.SQL)); err != nil {
			return err
		}
	}
	return nil
}

// recordHistory writes a best-effort history entry. Any failure is reported as
// a warning and never affects the query outcome.
func recordHistory(errOut io.Writer, connID, sql string, dur time.Duration, rowCount int, qerr error) {
	store, err := history.OpenDefault()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "s9l: warning: cannot open history:", err)
		return
	}
	defer func() { _ = store.Close() }()

	entry := history.HistoryEntry{
		ConnectionID: connID,
		SQL:          sql,
		ExecutedAt:   time.Now(),
		Duration:     dur,
		RowsAffected: int64(rowCount),
		Success:      qerr == nil,
	}
	if qerr != nil {
		entry.ErrorMessage = qerr.Error()
	}
	if _, err := store.AddHistory(context.Background(), entry); err != nil {
		_, _ = fmt.Fprintln(errOut, "s9l: warning: cannot record history:", err)
	}
}

// singleLine collapses whitespace/newlines so a multi-line SQL prints on one
// history row.
func singleLine(s string) string {
	out := make([]rune, 0, len(s))
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' || r == ' ' {
			if !prevSpace {
				out = append(out, ' ')
				prevSpace = true
			}
			continue
		}
		out = append(out, r)
		prevSpace = false
	}
	return string(out)
}
