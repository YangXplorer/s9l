package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaceholderAndQuote(t *testing.T) {
	if placeholder("postgres", 3) != "$3" {
		t.Error("postgres placeholder")
	}
	if placeholder("sqlserver", 2) != "@p2" {
		t.Error("sqlserver placeholder")
	}
	if placeholder("mysql", 1) != "?" || placeholder("sqlite", 9) != "?" {
		t.Error("? placeholder")
	}
	if quoteIdentifier("mysql", "a`b") != "`a``b`" {
		t.Errorf("mysql quote: %s", quoteIdentifier("mysql", "a`b"))
	}
	if quoteIdentifier("sqlserver", "a]b") != "[a]]b]" {
		t.Errorf("sqlserver quote: %s", quoteIdentifier("sqlserver", "a]b"))
	}
	if quoteIdentifier("postgres", `a"b`) != `"a""b"` {
		t.Errorf("postgres quote: %s", quoteIdentifier("postgres", `a"b`))
	}
}

func TestInsertSQL(t *testing.T) {
	got := insertSQL("postgres", "users", []string{"id", "name"}, 2)
	want := `INSERT INTO "users" ("id", "name") VALUES ($1, $2), ($3, $4)`
	if got != want {
		t.Errorf("postgres:\n got %s\nwant %s", got, want)
	}
	got = insertSQL("sqlite", "t", []string{"a"}, 3)
	want = `INSERT INTO "t" ("a") VALUES (?), (?), (?)`
	if got != want {
		t.Errorf("sqlite:\n got %s\nwant %s", got, want)
	}
}

func TestReadCSV(t *testing.T) {
	cols, rows, err := readCSV(strings.NewReader("id,name\n1,alice\n2,bob\n"))
	if err != nil {
		t.Fatalf("readCSV: %v", err)
	}
	if strings.Join(cols, ",") != "id,name" {
		t.Errorf("cols = %v", cols)
	}
	if len(rows) != 2 || rows[0][0] != "1" || rows[1][1] != "bob" {
		t.Errorf("rows = %v", rows)
	}
}

func TestReadJSON(t *testing.T) {
	cols, rows, err := readJSON(strings.NewReader(`[{"id":1,"name":"alice"},{"id":2}]`))
	if err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	// keys sorted: id, name
	if strings.Join(cols, ",") != "id,name" {
		t.Errorf("cols = %v", cols)
	}
	// second object is missing "name" → nil (NULL)
	if rows[1][1] != nil {
		t.Errorf("missing key should be nil, got %#v", rows[1][1])
	}
}

func TestImportFormat(t *testing.T) {
	for file, want := range map[string]string{"a.csv": "csv", "b.json": "json", "c.tsv": "csv", "d": "csv"} {
		got, err := importFormat("", file)
		if err != nil || got != want {
			t.Errorf("importFormat(%q) = %q,%v want %q", file, got, err, want)
		}
	}
	if _, err := importFormat("xml", "x"); err == nil {
		t.Error("unknown format should error")
	}
}

// TestRunImportCSVIntoSQLite is an end-to-end import through run().
func TestRunImportCSVIntoSQLite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	db := filepath.Join(tmp, "app.db")

	// Seed a table.
	if err := run([]string{db, "-e", "create table users(id integer, name text)"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("seed: %v", err)
	}
	csv := filepath.Join(tmp, "users.csv")
	if err := os.WriteFile(csv, []byte("id,name\n1,alice\n2,bob\n3,carol\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := run([]string{"import", db, "--table", "users", "--file", csv, "--batch", "2"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(out.String(), "imported 3 rows") {
		t.Errorf("unexpected output: %s", out.String())
	}

	// Verify the rows landed.
	var q strings.Builder
	if err := run([]string{db, "-e", "select count(*) from users", "--format", "csv"}, noInput(), &q, io.Discard); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(q.String(), "3") {
		t.Errorf("expected 3 rows, got: %s", q.String())
	}
}
