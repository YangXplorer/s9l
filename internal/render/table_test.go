package render_test

import (
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/render"
	"github.com/google/go-cmp/cmp"
)

func TestTable(t *testing.T) {
	var sb strings.Builder
	cols := []string{"id", "name"}
	rows := [][]any{
		{int64(1), "alice"},
		{int64(2), nil},
	}
	if err := render.Table(&sb, cols, rows); err != nil {
		t.Fatalf("Table: %v", err)
	}
	want := strings.Join([]string{
		"id | name ",
		"---+------",
		"1  | alice",
		"2  | NULL ",
		"",
	}, "\n")
	if diff := cmp.Diff(want, sb.String()); diff != "" {
		t.Errorf("table output mismatch (-want +got):\n%s", diff)
	}
}

func TestTableEmpty(t *testing.T) {
	var sb strings.Builder
	if err := render.Table(&sb, []string{"id"}, nil); err != nil {
		t.Fatalf("Table: %v", err)
	}
	want := "id\n--\n"
	if diff := cmp.Diff(want, sb.String()); diff != "" {
		t.Errorf("empty table mismatch (-want +got):\n%s", diff)
	}
}
