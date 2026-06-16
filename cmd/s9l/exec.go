package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"
)

// queryContext derives a context that is cancelled on SIGINT (Ctrl-C) and,
// if timeout > 0, after the timeout. This lets a long-running query be
// interrupted without killing the whole process/REPL. The returned cancel
// must always be called (releases the signal handler).
func queryContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt)
	if timeout <= 0 {
		return ctx, stop
	}
	tctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	return tctx, func() {
		cancelTimeout()
		stop()
	}
}

// classifyErr turns low-level context errors into clear, user-facing messages,
// distinguishing cancellation and timeout from ordinary query errors. Other
// errors (connection/SQL) are returned as-is — they already carry context
// (driver name from driver.Open, or the database's own SQL error text).
func classifyErr(err error, timeout time.Duration) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return errors.New("query cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		if timeout > 0 {
			return fmt.Errorf("query timed out after %s", timeout)
		}
		return errors.New("query timed out")
	default:
		return err
	}
}
