package main

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// noInput is an empty stdin for tests that do not exercise the REPL.
func noInput() io.Reader { return strings.NewReader("") }

func TestRunExecSelect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out strings.Builder
	if err := run([]string{":memory:", "-e", "SELECT 1 AS n"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "n") || !strings.Contains(out.String(), "1") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunFormatJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out strings.Builder
	if err := run([]string{":memory:", "-e", "select 1 as n", "--format", "json"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != `[{"n":1}]` {
		t.Fatalf("json output = %q, want [{\"n\":1}]", got)
	}
}

func TestRunBadFormat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := run([]string{":memory:", "-e", "select 1", "--format", "xml"}, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestRunReplFromPipe(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	in := strings.NewReader("select 1 as n;\nselect 2 as m;\n")
	var out strings.Builder
	if err := run([]string{":memory:"}, in, &out, io.Discard); err != nil {
		t.Fatalf("repl: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "n") || !strings.Contains(got, "1") ||
		!strings.Contains(got, "m") || !strings.Contains(got, "2") {
		t.Fatalf("repl output missing results:\n%s", got)
	}
}

func TestRunExecNoResultSet(t *testing.T) {
	// DDL/DML returns no columns and must produce no output (not an empty table).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out strings.Builder
	if err := run([]string{":memory:", "-e", "create table t(a int)"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("expected no output for DDL, got %q", out.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out strings.Builder
	if err := run([]string{"-version"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out.String(), "s9l ") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestRunMissingDSN(t *testing.T) {
	if err := run(nil, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected error when target is missing")
	}
}

func TestConnAddListRm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite", "--database", "./app.db"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}
	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite"}, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected duplicate id error")
	}

	var list strings.Builder
	if err := run([]string{"conn", "list"}, noInput(), &list, io.Discard); err != nil {
		t.Fatalf("conn list: %v", err)
	}
	if !strings.Contains(list.String(), "app") || !strings.Contains(list.String(), "sqlite") {
		t.Fatalf("conn list missing entry:\n%s", list.String())
	}

	if err := run([]string{"conn", "rm", "app"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn rm: %v", err)
	}
	if err := run([]string{"conn", "rm", "app"}, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected error removing missing connection")
	}
}

func TestHistoryRecordedAndListed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{":memory:", "-e", "select 1 as n"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("run ok: %v", err)
	}
	if err := run([]string{":memory:", "-e", "select * from nope"}, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected query error")
	}

	var out strings.Builder
	if err := run([]string{"history"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("history: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "select 1 as n") {
		t.Errorf("history missing successful query:\n%s", got)
	}
	if !strings.Contains(got, "ERR") {
		t.Errorf("history missing failed query marker:\n%s", got)
	}
}

func TestSavedAddListSearchRunRm(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dbPath := filepath.Join(tmp, "app.db")

	// A named connection the saved query can run against.
	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite", "--database", dbPath}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}
	// Seed a table so the saved SQL returns rows.
	if err := run([]string{"app", "-e", "create table t(a int); insert into t values(7)"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// add
	if err := run([]string{"saved", "add", "--title", "all t", "--conn", "app", "--sql", "select a from t", "--tags", "demo"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("saved add: %v", err)
	}
	// list
	var list strings.Builder
	if err := run([]string{"saved", "list"}, noInput(), &list, io.Discard); err != nil {
		t.Fatalf("saved list: %v", err)
	}
	if !strings.Contains(list.String(), "all t") || !strings.Contains(list.String(), "demo") {
		t.Fatalf("saved list missing entry:\n%s", list.String())
	}
	// search by tag
	var search strings.Builder
	if err := run([]string{"saved", "search", "demo"}, noInput(), &search, io.Discard); err != nil {
		t.Fatalf("saved search: %v", err)
	}
	if !strings.Contains(search.String(), "all t") {
		t.Fatalf("saved search missing entry:\n%s", search.String())
	}
	// run (#1)
	var runOut strings.Builder
	if err := run([]string{"saved", "run", "1", "--format", "json"}, noInput(), &runOut, io.Discard); err != nil {
		t.Fatalf("saved run: %v", err)
	}
	if !strings.Contains(runOut.String(), `"a":7`) {
		t.Fatalf("saved run output unexpected:\n%s", runOut.String())
	}
	// rm
	if err := run([]string{"saved", "rm", "1"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("saved rm: %v", err)
	}
	if err := run([]string{"saved", "rm", "1"}, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected error removing missing saved query")
	}
}

func TestRunNamedConnection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dbPath := filepath.Join(tmp, "app.db")

	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite", "--database", dbPath}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}

	var out strings.Builder
	if err := run([]string{"app", "-e", "select 1 as n"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("query via named connection: %v", err)
	}
	if !strings.Contains(out.String(), "n") || !strings.Contains(out.String(), "1") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
