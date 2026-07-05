package tui

import "testing"

func TestSQLServerNLiterals(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"non-ascii gets N", "last_name = '楊'", "last_name = N'楊'"},
		{"ascii untouched", "name = 'yang' AND x = 1", "name = 'yang' AND x = 1"},
		{"existing N kept", "last_name = N'楊'", "last_name = N'楊'"},
		{"existing lowercase n kept", "last_name = n'楊'", "last_name = n'楊'"},
		{"multiple literals", "a = '楊' OR b = 'x' OR c = 'メモ'", "a = N'楊' OR b = 'x' OR c = N'メモ'"},
		{"escaped quote inside", "note = 'it''s 楊'", "note = N'it''s 楊'"},
		{"identifier ending in N is not a prefix", "colN'楊'", "colNN'楊'"},
		{"no literals", "id > 10", "id > 10"},
		{"unterminated literal", "name = '楊", "name = N'楊"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := sqlserverNLiterals(c.in); got != c.want {
			t.Errorf("%s: %q → %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
