// Command s9l is a fast terminal database client.
//
// Phase 0 supports a single-shot query against a DSN:
//
//	s9l <dsn> -e "SQL"
//
// Named connections, a REPL, multiple output formats and more drivers arrive
// in later phases (see docs/TASKS.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/render"

	// Register the built-in drivers.
	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"
)

// Injected at build time via -ldflags (see docs/RELEASE.md).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "s9l:", err)
		os.Exit(1)
	}
}

func run(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var (
		execSQL    string
		driverName string
		showVer    bool
	)
	fs.StringVar(&execSQL, "e", "", `execute SQL and exit`)
	fs.StringVar(&driverName, "driver", "sqlite", "driver name ("+fmt.Sprint(driver.Names())+")")
	fs.BoolVar(&showVer, "version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(errOut, `usage: s9l <dsn> -e "SQL"`)
		fs.PrintDefaults()
	}
	// Parse flags that may appear before or after the positional DSN. The Go
	// flag package stops at the first non-flag argument, so we loop: parse,
	// peel off one positional, parse the rest, repeat.
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	if showVer {
		fmt.Fprintf(out, "s9l %s (commit %s, built %s)\n", version, commit, date)
		return nil
	}

	if len(positionals) < 1 {
		fs.Usage()
		return fmt.Errorf("missing <dsn>")
	}
	if execSQL == "" {
		return fmt.Errorf(`Phase 0 requires -e "SQL"`)
	}

	ctx := context.Background()
	conn, err := driver.Open(ctx, driverName, positionals[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	return execute(ctx, out, conn, execSQL)
}

func execute(ctx context.Context, out io.Writer, conn driver.Conn, sql string) error {
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer rows.Close()

	cols := rows.Columns()
	var data [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return err
		}
		data = append(data, vals)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return render.Table(out, cols, data)
}
