package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/render"
)

func TestExportFormat(t *testing.T) {
	cases := map[string]render.Format{
		"results.csv": render.FormatCSV,
		"out.json":    render.FormatJSON,
		"data.tsv":    render.FormatTSV,
		"noext":       render.FormatCSV,
		"UPPER.JSON":  render.FormatJSON,
	}
	for path, want := range cases {
		if got := exportFormat(path); got != want {
			t.Errorf("exportFormat(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestExportResultsWritesFile(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.setResults([]string{"id", "name"}, [][]any{{1, "alice"}, {2, nil}})

	dir := t.TempDir()
	csv := filepath.Join(dir, "out.csv")
	if err := a.exportResults(csv); err != nil {
		t.Fatalf("export csv: %v", err)
	}
	b, err := os.ReadFile(csv)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "id,name") || !strings.Contains(got, "1,alice") {
		t.Errorf("csv missing header/row:\n%s", got)
	}

	js := filepath.Join(dir, "out.json")
	if err := a.exportResults(js); err != nil {
		t.Fatalf("export json: %v", err)
	}
	jb, _ := os.ReadFile(js)
	if !strings.Contains(string(jb), `"name":"alice"`) {
		t.Errorf("json missing object:\n%s", string(jb))
	}
}

func TestShowExportNoResults(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	a.showExport()
	if a.exportOpen {
		t.Error("export overlay should not open with no results")
	}
}
