package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/YangXplorer/s9l/internal/render"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// showExport opens an input to save the current result set to a file. The format
// is inferred from the extension (.json / .tsv, otherwise CSV).
func (a *App) showExport() {
	if len(a.lastData) == 0 {
		a.SetStatus("no results to export")
		return
	}
	in := tview.NewInputField().SetLabel(" save to: ").SetText("results.csv")
	in.SetBorder(true).
		SetTitle(" Export results — Enter: save · Esc: cancel ").
		SetBorderColor(a.theme.Focus)
	in.SetDoneFunc(func(key tcell.Key) {
		path := strings.TrimSpace(in.GetText())
		a.pages.RemovePage("export")
		a.exportOpen = false
		a.app.SetFocus(a.navPanels()[a.focusIdx])
		if key != tcell.KeyEnter || path == "" {
			return
		}
		if err := a.exportResults(path); err != nil {
			a.setError("export: " + err.Error())
			return
		}
		a.SetStatus(fmt.Sprintf("exported %d rows to %s", len(a.lastData), path))
	})
	a.pages.AddPage("export", centered(in, 60, 3), true, true)
	a.app.SetFocus(in)
	a.exportOpen = true
}

// exportResults writes the retained result set to path, choosing the format from
// the file extension.
func (a *App) exportResults(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := render.Write(f, exportFormat(path), a.lastCols, a.lastData); err != nil {
		return err
	}
	return f.Close()
}

// exportFormat maps a file extension to a render format (default CSV).
func exportFormat(path string) render.Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return render.FormatJSON
	case ".tsv":
		return render.FormatTSV
	default:
		return render.FormatCSV
	}
}
