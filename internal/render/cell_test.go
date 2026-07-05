package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestCell(t *testing.T) {
	binUUID := string([]byte{0x92, 0x5c, 0x00, 0xf4, 0x47, 0x0d, 0x4b, 0x1c,
		0x8f, 0x3a, 0x01, 0x2d, 0x6e, 0x99, 0xab, 0xcd}) // MySQL binary(16), invalid UTF-8
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil is NULL", nil, "NULL"},
		{"plain text", "hello", "hello"},
		{"cjk text", "クレジットカード決済", "クレジットカード決済"},
		{"multiline text", "a\nb\tc", "a\nb\tc"},
		{"number", 42, "42"},
		{"binary string is hex", binUUID, "0x925c00f4470d4b1c8f3a012d6e99abcd"},
		{"control bytes are hex", "\x00\x01\x02", "0x000102"},
		{"binary bytes are hex", []byte{0xde, 0xad, 0xbe, 0xef}, "0xdeadbeef"},
		{"text bytes stay text", []byte("plain"), "plain"},
		{"empty string", "", ""},
	}
	for _, c := range cases {
		if got := Cell(c.in); got != c.want {
			t.Errorf("%s: Cell(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestTableRendersBinaryAsHex(t *testing.T) {
	var buf bytes.Buffer
	bin := string([]byte{0x92, 0x00, 0xff}) // invalid UTF-8, as drivers deliver binary
	if err := Table(&buf, []string{"uuid", "label"}, [][]any{{bin, "aupay"}}); err != nil {
		t.Fatalf("Table: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "0x9200ff") {
		t.Errorf("table should render binary as hex, got:\n%s", out)
	}
	if strings.Contains(out, "\x92") {
		t.Errorf("table must not emit raw binary bytes, got:\n%s", out)
	}
}
