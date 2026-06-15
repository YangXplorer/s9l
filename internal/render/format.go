package render

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
)

// Format is an output format for a result set.
type Format string

const (
	// FormatTable is a human-readable aligned text table (TTY default).
	FormatTable Format = "table"
	// FormatJSON is a JSON array of objects, one per row, preserving column order.
	FormatJSON Format = "json"
	// FormatCSV is comma-separated values with a header row.
	FormatCSV Format = "csv"
	// FormatTSV is tab-separated values with a header row (pipe default).
	FormatTSV Format = "tsv"
)

// ParseFormat validates and returns a Format.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatTable, FormatJSON, FormatCSV, FormatTSV:
		return Format(s), nil
	default:
		return "", fmt.Errorf("render: unknown format %q (want table|json|csv|tsv)", s)
	}
}

// Write renders the result set in the given format.
func Write(w io.Writer, f Format, cols []string, rows [][]any) error {
	switch f {
	case FormatTable:
		return Table(w, cols, rows)
	case FormatJSON:
		return writeJSON(w, cols, rows)
	case FormatCSV:
		return writeDelimited(w, ',', cols, rows)
	case FormatTSV:
		return writeDelimited(w, '\t', cols, rows)
	default:
		return fmt.Errorf("render: unknown format %q", f)
	}
}

// writeJSON emits a JSON array of objects. Keys follow column order (which a
// plain map would lose), and a SQL NULL becomes JSON null.
func writeJSON(w io.Writer, cols []string, rows [][]any) error {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, row := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('{')
		for j, col := range cols {
			if j > 0 {
				buf.WriteByte(',')
			}
			k, err := json.Marshal(col)
			if err != nil {
				return err
			}
			var v []byte
			if j < len(row) {
				v, err = json.Marshal(row[j])
			} else {
				v, err = json.Marshal(nil)
			}
			if err != nil {
				return err
			}
			buf.Write(k)
			buf.WriteByte(':')
			buf.Write(v)
		}
		buf.WriteByte('}')
	}
	buf.WriteByte(']')
	buf.WriteByte('\n')
	_, err := w.Write(buf.Bytes())
	return err
}

// writeDelimited emits CSV/TSV with a header row. A SQL NULL becomes an empty
// field (machine formats favor parseability over a distinct NULL token).
func writeDelimited(w io.Writer, comma rune, cols []string, rows [][]any) error {
	cw := csv.NewWriter(w)
	cw.Comma = comma
	if err := cw.Write(cols); err != nil {
		return err
	}
	rec := make([]string, len(cols))
	for _, row := range rows {
		for i := range cols {
			if i < len(row) && row[i] != nil {
				rec[i] = fmt.Sprintf("%v", row[i])
			} else {
				rec[i] = ""
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
