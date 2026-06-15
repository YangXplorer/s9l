package render_test

import (
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/render"
	"github.com/google/go-cmp/cmp"
)

// chanSource is a Source whose rows are not held in a slice up front, used to
// confirm WriteSource pulls row-by-row.
type chanSource struct {
	cols []string
	rows [][]any
	i    int
}

func (s *chanSource) Columns() []string { return s.cols }
func (s *chanSource) Next() ([]any, bool, error) {
	if s.i >= len(s.rows) {
		return nil, false, nil
	}
	r := s.rows[s.i]
	s.i++
	return r, true, nil
}

func TestWriteSourceCountAndJSON(t *testing.T) {
	src := &chanSource{cols: []string{"id"}, rows: [][]any{{int64(1)}, {int64(2)}, {int64(3)}}}
	var sb strings.Builder
	n, err := render.WriteSource(&sb, render.Options{Format: render.FormatJSON}, src)
	if err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	if n != 3 {
		t.Errorf("count = %d, want 3", n)
	}
	want := `[{"id":1},{"id":2},{"id":3}]` + "\n"
	if diff := cmp.Diff(want, sb.String()); diff != "" {
		t.Errorf("json mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteSourceTableTruncation(t *testing.T) {
	src := &chanSource{cols: []string{"v"}, rows: [][]any{{"abcdefghij"}}}
	var sb strings.Builder
	if _, err := render.WriteSource(&sb, render.Options{Format: render.FormatTable, MaxCellWidth: 5}, src); err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	// 10-rune value truncated to 5 runes: 4 chars + ellipsis.
	if !strings.Contains(sb.String(), "abcd…") {
		t.Errorf("expected truncated cell 'abcd…', got:\n%s", sb.String())
	}
	if strings.Contains(sb.String(), "abcdefghij") {
		t.Errorf("full value should not appear when truncated:\n%s", sb.String())
	}
}

func TestWriteSourceCSVNoTruncation(t *testing.T) {
	// Truncation must NOT apply to machine formats (data integrity).
	src := &chanSource{cols: []string{"v"}, rows: [][]any{{"abcdefghij"}}}
	var sb strings.Builder
	if _, err := render.WriteSource(&sb, render.Options{Format: render.FormatCSV, MaxCellWidth: 5}, src); err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	if !strings.Contains(sb.String(), "abcdefghij") {
		t.Errorf("csv must keep full value, got:\n%s", sb.String())
	}
}
