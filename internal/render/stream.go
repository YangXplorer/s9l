package render

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
)

// Source is a pull-based result set for streaming rendering, so large results
// (csv/tsv/json) are written row-by-row without buffering the whole set in
// memory. driver.Rows is adapted to this in the cmd layer.
type Source interface {
	Columns() []string
	// Next returns the next row; ok is false at end-of-rows (then err carries
	// any iteration error).
	Next() (row []any, ok bool, err error)
}

// Options configures rendering.
type Options struct {
	Format Format
	// MaxCellWidth truncates table cells longer than this many runes (0 =
	// unlimited). Ignored by machine formats (csv/tsv/json).
	MaxCellWidth int
}

// WriteSource renders src in the chosen format, returning the row count.
// csv/tsv/json stream row-by-row; table buffers (it needs column widths) and is
// the interactive format where result sets are small.
func WriteSource(w io.Writer, opts Options, src Source) (int, error) {
	switch opts.Format {
	case FormatTable:
		return writeTableSource(w, src, opts.MaxCellWidth)
	case FormatJSON:
		return writeJSONSource(w, src)
	case FormatCSV:
		return writeDelimitedSource(w, ',', src)
	case FormatTSV:
		return writeDelimitedSource(w, '\t', src)
	default:
		return 0, fmt.Errorf("render: unknown format %q", opts.Format)
	}
}

func writeTableSource(w io.Writer, src Source, maxCell int) (int, error) {
	cols := src.Columns()
	var rows [][]any
	for {
		row, ok, err := src.Next()
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		rows = append(rows, row)
	}
	if len(cols) == 0 {
		return 0, nil
	}
	if err := tableWith(w, cols, rows, maxCell); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func writeJSONSource(w io.Writer, src Source) (int, error) {
	cols := src.Columns()
	var buf bytes.Buffer
	buf.WriteByte('[')
	n := 0
	for {
		row, ok, err := src.Next()
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		if n > 0 {
			buf.WriteByte(',')
		}
		if err := encodeJSONObject(&buf, cols, row); err != nil {
			return 0, err
		}
		n++
		// Flush periodically so memory stays bounded for large result sets.
		if buf.Len() > 32*1024 {
			if _, err := w.Write(buf.Bytes()); err != nil {
				return 0, err
			}
			buf.Reset()
		}
	}
	buf.WriteByte(']')
	buf.WriteByte('\n')
	if _, err := w.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return n, nil
}

func encodeJSONObject(buf *bytes.Buffer, cols []string, row []any) error {
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
	return nil
}

func writeDelimitedSource(w io.Writer, comma rune, src Source) (int, error) {
	cols := src.Columns()
	cw := csv.NewWriter(w)
	cw.Comma = comma
	if err := cw.Write(cols); err != nil {
		return 0, err
	}
	rec := make([]string, len(cols))
	n := 0
	for {
		row, ok, err := src.Next()
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		for i := range cols {
			if i < len(row) && row[i] != nil {
				rec[i] = fmt.Sprintf("%v", row[i])
			} else {
				rec[i] = ""
			}
		}
		if err := cw.Write(rec); err != nil {
			return 0, err
		}
		n++
	}
	cw.Flush()
	return n, cw.Error()
}

// truncate shortens s to maxWidth runes (0 = no limit), marking elision with ….
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	return string(r[:maxWidth-1]) + "…"
}
