package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/YangXplorer/s9l/internal/driver"
)

// runImport implements:
//
//	s9l import <connection|dsn> --table T --file data.csv [--format csv|json] [--batch N]
//
// It bulk-loads a CSV or JSON file into a table using batched multi-row INSERTs.
func runImport(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l import", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var (
		table      string
		file       string
		formatFlag string
		driverFlag string
		batch      int
	)
	fs.StringVar(&table, "table", "", "destination table (required)")
	fs.StringVar(&file, "file", "", "input file (required)")
	fs.StringVar(&formatFlag, "format", "", "input format: csv|json (default: from file extension)")
	fs.StringVar(&driverFlag, "driver", "sqlite", "driver name for a bare DSN")
	fs.IntVar(&batch, "batch", 500, "rows per INSERT statement")
	positionals, err := parseFlagsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) < 1 {
		return errors.New(`usage: s9l import <connection|dsn> --table T --file data.csv [--format csv|json]`)
	}
	if table == "" || file == "" {
		return errors.New("import: --table and --file are required")
	}
	if batch < 1 {
		batch = 1
	}
	format, err := importFormat(formatFlag, file)
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, drv, closeConn, err := openTarget(ctx, positionals[0], driverFlag)
	if err != nil {
		return err
	}
	defer func() { _ = closeConn() }()

	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var cols []string
	var rows [][]any
	switch format {
	case "json":
		cols, rows, err = readJSON(f)
	default:
		cols, rows, err = readCSV(f)
	}
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return errors.New("import: no columns found in input")
	}

	n, err := importRows(ctx, conn, drv, table, cols, rows, batch)
	if err != nil {
		return fmt.Errorf("imported %d rows, then: %w", n, err)
	}
	_, err = fmt.Fprintf(out, "imported %d rows into %s\n", n, table)
	return err
}

// importFormat resolves the input format from the flag or the file extension.
func importFormat(flagVal, file string) (string, error) {
	v := strings.ToLower(flagVal)
	if v == "" {
		switch strings.ToLower(filepath.Ext(file)) {
		case ".json":
			v = "json"
		case ".csv", ".tsv", "":
			v = "csv"
		default:
			v = "csv"
		}
	}
	if v != "csv" && v != "json" {
		return "", fmt.Errorf("import: unknown format %q (want csv|json)", flagVal)
	}
	return v, nil
}

// readCSV reads a CSV file: the first row is the header (column names), the rest
// are string-valued rows.
func readCSV(r io.Reader) ([]string, [][]any, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err == io.EOF {
		return nil, nil, errors.New("import: empty CSV file")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("import: read header: %w", err)
	}
	var rows [][]any
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("import: read row: %w", err)
		}
		row := make([]any, len(header))
		for i := range header {
			if i < len(rec) {
				row[i] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return header, rows, nil
}

// readJSON reads a JSON array of objects. Columns are the sorted keys of the
// first object; later objects map by those columns (missing keys → NULL).
func readJSON(r io.Reader) ([]string, [][]any, error) {
	var records []map[string]any
	dec := json.NewDecoder(r)
	if err := dec.Decode(&records); err != nil {
		return nil, nil, fmt.Errorf("import: parse JSON (expected an array of objects): %w", err)
	}
	if len(records) == 0 {
		return nil, nil, errors.New("import: empty JSON array")
	}
	cols := make([]string, 0, len(records[0]))
	for k := range records[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	rows := make([][]any, 0, len(records))
	for _, rec := range records {
		row := make([]any, len(cols))
		for i, c := range cols {
			row[i] = rec[c] // missing key → nil → NULL
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}

// importRows inserts rows in batches of up to batch rows per multi-row INSERT,
// returning the number of rows inserted.
func importRows(ctx context.Context, conn driver.Conn, drv, table string, cols []string, rows [][]any, batch int) (int, error) {
	done := 0
	for start := 0; start < len(rows); start += batch {
		end := min(start+batch, len(rows))
		chunk := rows[start:end]
		sql := insertSQL(drv, table, cols, len(chunk))
		args := make([]any, 0, len(chunk)*len(cols))
		for _, row := range chunk {
			args = append(args, row...)
		}
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			return done, err
		}
		done += len(chunk)
	}
	return done, nil
}

// insertSQL builds a multi-row INSERT with dialect-correct placeholders for
// nRows rows of len(cols) columns each.
func insertSQL(drv, table string, cols []string, nRows int) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdentifier(drv, c)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES ", quoteIdentifier(drv, table), strings.Join(quoted, ", "))
	n := 0
	for r := range nRows {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for c := range cols {
			if c > 0 {
				b.WriteString(", ")
			}
			n++
			b.WriteString(placeholder(drv, n))
		}
		b.WriteByte(')')
	}
	return b.String()
}

// placeholder returns the n-th bind placeholder for the driver's dialect.
func placeholder(drv string, n int) string {
	switch drv {
	case "postgres":
		return "$" + strconv.Itoa(n)
	case "sqlserver":
		return "@p" + strconv.Itoa(n)
	default: // sqlite, mysql
		return "?"
	}
}

// quoteIdentifier quotes a table/column name for the driver's dialect.
func quoteIdentifier(drv, name string) string {
	switch drv {
	case "mysql":
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	case "sqlserver":
		return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
	default: // postgres, sqlite
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}
