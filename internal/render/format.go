package render

import (
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

// Write renders an in-memory result set. It is a convenience wrapper over
// WriteSource for callers that already hold all rows (e.g. tests). Streaming
// callers should use WriteSource directly.
func Write(w io.Writer, f Format, cols []string, rows [][]any) error {
	_, err := WriteSource(w, Options{Format: f}, &sliceSource{cols: cols, rows: rows})
	return err
}

// sliceSource adapts an in-memory [][]any to a streaming Source.
type sliceSource struct {
	cols []string
	rows [][]any
	i    int
}

func (s *sliceSource) Columns() []string { return s.cols }

func (s *sliceSource) Next() ([]any, bool, error) {
	if s.i >= len(s.rows) {
		return nil, false, nil
	}
	row := s.rows[s.i]
	s.i++
	return row, true, nil
}
