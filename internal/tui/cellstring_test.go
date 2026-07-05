package tui

import "testing"

// Binary values (drivers normalize []byte to string, so binary columns arrive
// as invalid-UTF-8 strings) must render as 0x… hex in the Results table, not
// as raw mojibake bytes.
func TestFillResultsRendersBinaryAsHex(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db")})
	bin := string([]byte{0x92, 0x5c, 0x00, 0xf4})
	a.setResults([]string{"uuid", "label"}, [][]any{{bin, "aupay"}})

	if got, want := a.results.GetCell(1, 0).Text, "0x925c00f4"; got != want {
		t.Errorf("binary cell = %q, want %q", got, want)
	}
	if got := a.results.GetCell(1, 1).Text; got != "aupay" {
		t.Errorf("text cell = %q, want aupay", got)
	}
}
