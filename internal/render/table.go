// Package render formats query results for terminal output. Phase 0 provides
// a minimal aligned text table; json/csv and pipe-aware output land in Phase 1
// (see docs/TASKS.md C1/C2).
package render

import (
	"fmt"
	"io"
	"strings"
)

// nullText is how a SQL NULL (nil value) is displayed.
const nullText = "NULL"

// Table writes the result set as a simple left-aligned text table.
func Table(w io.Writer, cols []string, rows [][]any) error {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	cells := make([][]string, len(rows))
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for i := range cols {
			var s string
			if i < len(row) {
				s = format(row[i])
			}
			cells[r][i] = s
			if len(s) > widths[i] {
				widths[i] = len(s)
			}
		}
	}

	if err := writeRow(w, cols, widths); err != nil {
		return err
	}
	if err := writeSep(w, widths); err != nil {
		return err
	}
	for _, row := range cells {
		if err := writeRow(w, row, widths); err != nil {
			return err
		}
	}
	return nil
}

func format(v any) string {
	if v == nil {
		return nullText
	}
	return fmt.Sprintf("%v", v)
}

func writeRow(w io.Writer, cells []string, widths []int) error {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = pad(c, widths[i])
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, " | "))
	return err
}

func writeSep(w io.Writer, widths []int) error {
	parts := make([]string, len(widths))
	for i, width := range widths {
		parts[i] = strings.Repeat("-", width)
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "-+-"))
	return err
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
