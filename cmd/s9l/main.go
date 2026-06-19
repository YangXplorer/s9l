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
	"time"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/render"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/mattn/go-isatty"

	// Register the built-in drivers.
	_ "github.com/YangXplorer/s9l/internal/driver/mysql"
	_ "github.com/YangXplorer/s9l/internal/driver/postgres"
	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"
	_ "github.com/YangXplorer/s9l/internal/driver/sqlserver"
)

// Injected at build time via -ldflags (see docs/RELEASE.md).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "s9l:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) > 0 && args[0] == "conn" {
		return runConn(args[1:], out, errOut)
	}
	if len(args) > 0 && args[0] == "history" {
		return runHistory(args[1:], out, errOut)
	}
	if len(args) > 0 && args[0] == "saved" {
		return runSaved(args[1:], out, errOut)
	}
	if len(args) > 0 && args[0] == "tui" {
		return runTUI(args[1:])
	}
	if len(args) > 0 && args[0] == "import" {
		return runImport(args[1:], out, errOut)
	}
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		return printHelp(out)
	}

	fs := flag.NewFlagSet("s9l", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var (
		execSQL     string
		driverName  string
		formatFlag  string
		maxColWidth int
		timeout     time.Duration
		showVer     bool
		noPager     bool
	)
	fs.StringVar(&execSQL, "e", "", `execute SQL and exit`)
	fs.StringVar(&driverName, "driver", "sqlite", "driver name for a bare DSN ("+fmt.Sprint(driver.Names())+")")
	fs.StringVar(&formatFlag, "format", "", "output format: table|json|csv|tsv (default: table on a TTY, tsv when piped)")
	fs.IntVar(&maxColWidth, "max-col-width", 0, "truncate table cells to N runes (0 = unlimited; table format only)")
	fs.DurationVar(&timeout, "timeout", 0, "abort a query after this duration (e.g. 30s; 0 = no limit)")
	fs.BoolVar(&noPager, "no-pager", false, "do not page large output through $PAGER on a terminal")
	fs.BoolVar(&showVer, "version", false, "print version and exit")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(errOut, `usage: s9l <connection-id|dsn> -e "SQL"   (omit -e for REPL)`)
		_, _ = fmt.Fprintln(errOut, `       s9l tui [connection]              (full-screen UI)`)
		_, _ = fmt.Fprintln(errOut, `       s9l conn|history|saved ...`)
		fs.PrintDefaults()
	}
	positionals, err := parseFlagsInterspersed(fs, args)
	if err != nil {
		return err
	}

	if showVer {
		_, err := fmt.Fprintf(out, "s9l %s (commit %s, built %s)\n", version, commit, date)
		return err
	}

	if len(positionals) < 1 {
		fs.Usage()
		return errors.New("missing <connection-id|dsn>")
	}

	format, err := outputFormat(formatFlag, out)
	if err != nil {
		return err
	}
	opts := render.Options{Format: format, MaxCellWidth: maxColWidth}

	ctx := context.Background()
	usePager := !noPager
	if execSQL == "" {
		// No -e: drop into the interactive REPL.
		return runREPL(ctx, in, out, errOut, positionals[0], driverName, opts, timeout, usePager)
	}
	return runQuery(ctx, out, errOut, positionals[0], driverName, execSQL, opts, timeout, usePager)
}

// runQuery resolves target to a connection, runs sql, renders the result, and
// records history (best-effort). It is shared by the `-e` path and `saved run`.
func runQuery(ctx context.Context, out, errOut io.Writer, target, driverFlag, sql string, opts render.Options, timeout time.Duration, usePager bool) error {
	drv, dsn, err := resolveTarget(target, driverFlag)
	if err != nil {
		return err
	}

	conn, err := driver.Open(ctx, drv, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	qctx, cancel := queryContext(ctx, timeout)
	defer cancel()

	start := time.Now()
	rowCount, qerr := runStatementPaged(qctx, out, conn, sql, opts, usePager)
	qerr = classifyErr(qerr, timeout)
	// History recording is best-effort and must not affect the query result.
	recordHistory(errOut, target, sql, time.Since(start), rowCount, qerr)
	return qerr
}

// parseFlagsInterspersed parses fs allowing flags to appear before or after
// positional arguments. The Go flag package stops at the first non-flag
// argument, so we loop: parse, peel off one positional, parse the rest.
func parseFlagsInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	return positionals, nil
}

// outputFormat picks the result format: the explicit --format flag if given,
// otherwise a TTY-aware default (table for a terminal, tsv when piped so the
// output stays machine-parseable).
func outputFormat(flagVal string, out io.Writer) (render.Format, error) {
	if flagVal != "" {
		return render.ParseFormat(flagVal)
	}
	if isTTY(out) {
		return render.FormatTable, nil
	}
	return render.FormatTSV, nil
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
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
	// Resolve via the OS keychain store (handles env: and keychain:// refs;
	// the keychain is only touched for keychain:// refs).
	password, err := secret.Resolve(secret.Default(), cc.PasswordRef)
	if err != nil {
		return "", "", fmt.Errorf("connection %q: %w", cc.ID, err)
	}
	d, err := cc.DSN(password)
	if err != nil {
		return "", "", err
	}
	return cc.Driver, d, nil
}

// execute runs the SQL and renders the result, returning the number of rows
// rendered (for history bookkeeping).
func execute(ctx context.Context, out io.Writer, conn driver.Conn, sql string, opts render.Options) (int, error) {
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return 0, err
	}
	return drainAndRender(out, opts, rows)
}

// drainAndRender streams rows to the renderer, returning the row count.
// Statements that return no columns (DDL/DML) render nothing.
func drainAndRender(out io.Writer, opts render.Options, rows driver.Rows) (int, error) {
	defer func() { _ = rows.Close() }()
	if len(rows.Columns()) == 0 {
		return 0, nil
	}
	return render.WriteSource(out, opts, &rowsSource{rows: rows})
}

// rowsSource adapts driver.Rows to render.Source for streaming output.
type rowsSource struct{ rows driver.Rows }

func (s *rowsSource) Columns() []string { return s.rows.Columns() }

func (s *rowsSource) Next() ([]any, bool, error) {
	if !s.rows.Next() {
		return nil, false, s.rows.Err()
	}
	v, err := s.rows.Values()
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}
