package render_test

import (
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/render"
	"github.com/google/go-cmp/cmp"
)

var (
	testCols = []string{"id", "name"}
	testRows = [][]any{
		{int64(1), "alice"},
		{int64(2), nil},
	}
)

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"table", "json", "csv", "tsv"} {
		if _, err := render.ParseFormat(s); err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", s, err)
		}
	}
	if _, err := render.ParseFormat("xml"); err == nil {
		t.Error("ParseFormat(xml) should error")
	}
}

func TestWriteJSON(t *testing.T) {
	var sb strings.Builder
	if err := render.Write(&sb, render.FormatJSON, testCols, testRows); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := `[{"id":1,"name":"alice"},{"id":2,"name":null}]` + "\n"
	if diff := cmp.Diff(want, sb.String()); diff != "" {
		t.Errorf("json mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteJSONEmpty(t *testing.T) {
	var sb strings.Builder
	if err := render.Write(&sb, render.FormatJSON, testCols, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := sb.String(); got != "[]\n" {
		t.Errorf("empty json = %q, want []", got)
	}
}

func TestWriteCSV(t *testing.T) {
	var sb strings.Builder
	if err := render.Write(&sb, render.FormatCSV, testCols, testRows); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := "id,name\n1,alice\n2,\n"
	if diff := cmp.Diff(want, sb.String()); diff != "" {
		t.Errorf("csv mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteTSV(t *testing.T) {
	var sb strings.Builder
	if err := render.Write(&sb, render.FormatTSV, testCols, testRows); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := "id\tname\n1\talice\n2\t\n"
	if diff := cmp.Diff(want, sb.String()); diff != "" {
		t.Errorf("tsv mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteTable(t *testing.T) {
	var sb strings.Builder
	if err := render.Write(&sb, render.FormatTable, testCols, testRows); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(sb.String(), "NULL") {
		t.Errorf("table should show NULL for nil:\n%s", sb.String())
	}
}
