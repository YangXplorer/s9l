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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/render"
	"github.com/YangXplorer/s9l/internal/secret"

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
	if len(args) > 0 && args[0] == "conn" {
		return runConn(args[1:], out, errOut)
	}

	fs := flag.NewFlagSet("s9l", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var (
		execSQL    string
		driverName string
		showVer    bool
	)
	fs.StringVar(&execSQL, "e", "", `execute SQL and exit`)
	fs.StringVar(&driverName, "driver", "sqlite", "driver name for a bare DSN ("+fmt.Sprint(driver.Names())+")")
	fs.BoolVar(&showVer, "version", false, "print version and exit")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(errOut, `usage: s9l <connection-id|dsn> -e "SQL"`)
		_, _ = fmt.Fprintln(errOut, `       s9l conn <list|add|rm>`)
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
		_, err := fmt.Fprintf(out, "s9l %s (commit %s, built %s)\n", version, commit, date)
		return err
	}

	if len(positionals) < 1 {
		fs.Usage()
		return errors.New("missing <connection-id|dsn>")
	}
	if execSQL == "" {
		return errors.New(`-e "SQL" is required`)
	}

	drv, dsn, err := resolveTarget(positionals[0], driverName)
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := driver.Open(ctx, drv, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	return execute(ctx, out, conn, execSQL)
}

// resolveTarget maps the positional argument to a (driver, dsn) pair. If it
// names a configured connection, that connection's driver/DSN (with its
// password resolved) is used; otherwise it is treated as a bare DSN for the
// driver given by the --driver flag.
func resolveTarget(target, driverFlag string) (drv, dsn string, _ error) {
	cfg, err := config.Load()
	if err != nil {
		return "", "", err
	}
	cc, ok := cfg.Get(target)
	if !ok {
		return driverFlag, target, nil
	}
	// Phase 1 uses an in-memory secret store (env: refs work, keychain in P2).
	password, err := secret.Resolve(secret.NewMemory(), cc.PasswordRef)
	if err != nil {
		return "", "", fmt.Errorf("connection %q: %w", cc.ID, err)
	}
	d, err := cc.DSN(password)
	if err != nil {
		return "", "", err
	}
	return cc.Driver, d, nil
}

func execute(ctx context.Context, out io.Writer, conn driver.Conn, sql string) error {
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

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
