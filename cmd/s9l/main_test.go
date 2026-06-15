package main

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExecSelect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out strings.Builder
	err := run([]string{":memory:", "-e", "SELECT 1 AS n"}, &out, io.Discard)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "n") || !strings.Contains(out.String(), "1") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunFormatJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out strings.Builder
	if err := run([]string{":memory:", "-e", "select 1 as n", "--format", "json"}, &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != `[{"n":1}]` {
		t.Fatalf("json output = %q, want [{\"n\":1}]", got)
	}
}

func TestRunBadFormat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := run([]string{":memory:", "-e", "select 1", "--format", "xml"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestRunVersion(t *testing.T) {
	var out strings.Builder
	if err := run([]string{"-version"}, &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out.String(), "s9l ") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestRunMissingDSN(t *testing.T) {
	if err := run(nil, io.Discard, io.Discard); err == nil {
		t.Fatal("expected error when DSN is missing")
	}
}

func TestRunMissingExec(t *testing.T) {
	if err := run([]string{":memory:"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected error when -e is missing")
	}
}

func TestConnAddListRm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite", "--database", "./app.db"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}
	// duplicate id should fail
	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected duplicate id error")
	}

	var list strings.Builder
	if err := run([]string{"conn", "list"}, &list, io.Discard); err != nil {
		t.Fatalf("conn list: %v", err)
	}
	if !strings.Contains(list.String(), "app") || !strings.Contains(list.String(), "sqlite") {
		t.Fatalf("conn list missing entry:\n%s", list.String())
	}

	if err := run([]string{"conn", "rm", "app"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("conn rm: %v", err)
	}
	if err := run([]string{"conn", "rm", "app"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected error removing missing connection")
	}
}

func TestHistoryRecordedAndListed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// A successful query and a failing one are both recorded.
	if err := run([]string{":memory:", "-e", "select 1 as n"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("run ok: %v", err)
	}
	if err := run([]string{":memory:", "-e", "select * from nope"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected query error")
	}

	var out strings.Builder
	if err := run([]string{"history"}, &out, io.Discard); err != nil {
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
	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite", "--database", dbPath}, io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}
	// Seed a table so the saved SQL returns rows.
	if err := run([]string{"app", "-e", "create table t(a int); insert into t values(7)"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// add
	if err := run([]string{"saved", "add", "--title", "all t", "--conn", "app", "--sql", "select a from t", "--tags", "demo"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("saved add: %v", err)
	}
	// list
	var list strings.Builder
	if err := run([]string{"saved", "list"}, &list, io.Discard); err != nil {
		t.Fatalf("saved list: %v", err)
	}
	if !strings.Contains(list.String(), "all t") || !strings.Contains(list.String(), "demo") {
		t.Fatalf("saved list missing entry:\n%s", list.String())
	}
	// search by tag
	var search strings.Builder
	if err := run([]string{"saved", "search", "demo"}, &search, io.Discard); err != nil {
		t.Fatalf("saved search: %v", err)
	}
	if !strings.Contains(search.String(), "all t") {
		t.Fatalf("saved search missing entry:\n%s", search.String())
	}
	// run (#1)
	var runOut strings.Builder
	if err := run([]string{"saved", "run", "1", "--format", "json"}, &runOut, io.Discard); err != nil {
		t.Fatalf("saved run: %v", err)
	}
	if !strings.Contains(runOut.String(), `"a":7`) {
		t.Fatalf("saved run output unexpected:\n%s", runOut.String())
	}
	// rm
	if err := run([]string{"saved", "rm", "1"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("saved rm: %v", err)
	}
	if err := run([]string{"saved", "rm", "1"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected error removing missing saved query")
	}
}

func TestRunNamedConnection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dbPath := filepath.Join(tmp, "app.db")

	if err := run([]string{"conn", "add", "--id", "app", "--driver", "sqlite", "--database", dbPath}, io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}

	var out strings.Builder
	if err := run([]string{"app", "-e", "select 1 as n"}, &out, io.Discard); err != nil {
		t.Fatalf("query via named connection: %v", err)
	}
	if !strings.Contains(out.String(), "n") || !strings.Contains(out.String(), "1") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
