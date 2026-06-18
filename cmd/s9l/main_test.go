package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/zalando/go-keyring"
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

func TestRunMetaCommands(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	db := filepath.Join(tmp, "m.db")

	if err := run([]string{"conn", "add", "--id", "m", "--driver", "sqlite", "--database", db}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}
	if err := run([]string{"m", "-e", "create table widgets(id int, label text)"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("create: %v", err)
	}

	// \dt lists the table.
	var dt strings.Builder
	if err := run([]string{"m", "-e", `\dt`, "--format", "csv"}, noInput(), &dt, io.Discard); err != nil {
		t.Fatalf("\\dt: %v", err)
	}
	if !strings.Contains(dt.String(), "widgets") {
		t.Fatalf("\\dt missing table:\n%s", dt.String())
	}

	// \d <table> describes columns.
	var d strings.Builder
	if err := run([]string{"m", "-e", `\d widgets`, "--format", "csv"}, noInput(), &d, io.Discard); err != nil {
		t.Fatalf("\\d: %v", err)
	}
	if !strings.Contains(d.String(), "label") {
		t.Fatalf("\\d missing column:\n%s", d.String())
	}

	// Unknown backslash command errors.
	if err := run([]string{"m", "-e", `\nope`}, noInput(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected error for unknown meta command")
	}
}

func TestRunMaxColWidth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out strings.Builder
	if err := run([]string{":memory:", "-e", "select 'abcdefghij' as v", "--format", "table", "--max-col-width", "5"}, noInput(), &out, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "abcd…") {
		t.Fatalf("expected truncated cell, got:\n%s", out.String())
	}
}

func TestClassifyErr(t *testing.T) {
	if classifyErr(nil, 0) != nil {
		t.Error("nil should stay nil")
	}
	if got := classifyErr(context.Canceled, 0); got == nil || got.Error() != "query cancelled" {
		t.Errorf("canceled = %v, want 'query cancelled'", got)
	}
	if got := classifyErr(context.DeadlineExceeded, 5*time.Second); got == nil || !strings.Contains(got.Error(), "timed out after 5s") {
		t.Errorf("deadline = %v, want contains 'timed out after 5s'", got)
	}
	sentinel := errors.New("boom")
	if got := classifyErr(sentinel, 0); got != sentinel {
		t.Errorf("other error should pass through unchanged, got %v", got)
	}
}

func TestRunTimeout(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := run([]string{":memory:", "-e", "select 1", "--timeout", "1ns"}, noInput(), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
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

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		var out strings.Builder
		if err := run([]string{arg}, noInput(), &out, io.Discard); err != nil {
			t.Fatalf("run %q: %v", arg, err)
		}
		s := out.String()
		for _, want := range []string{"Usage:", "s9l tui", "s9l conn", "s9l saved", "--format", "--no-pager"} {
			if !strings.Contains(s, want) {
				t.Errorf("help (%s) missing %q", arg, want)
			}
		}
	}
}

func TestConnAddWithPasswordUsesKeychain(t *testing.T) {
	keyring.MockInit() // in-memory keychain; no real OS keychain I/O
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"conn", "add", "--id", "pg", "--driver", "postgres",
		"--host", "h", "--user", "u", "--database", "d", "--password", "sekret"},
		noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn add: %v", err)
	}

	// config.yaml stores only the keychain ref, never the plaintext password.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cc, ok := cfg.Get("pg")
	if !ok {
		t.Fatal("connection pg not saved")
	}
	if cc.PasswordRef != secret.KeychainRef("pg") {
		t.Errorf("password_ref = %q, want %q", cc.PasswordRef, secret.KeychainRef("pg"))
	}
	// The password lives in the keychain and resolves back.
	got, err := secret.Resolve(secret.Default(), cc.PasswordRef)
	if err != nil || got != "sekret" {
		t.Fatalf("resolve = %q, %v; want sekret, nil", got, err)
	}

	// conn rm also removes the keychain secret.
	if err := run([]string{"conn", "rm", "pg"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("conn rm: %v", err)
	}
	if _, err := secret.Default().Get(secret.Service, secret.ConnPasswordKey("pg")); err != secret.ErrNotFound {
		t.Errorf("keychain secret should be gone after rm, got err=%v", err)
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

func TestSavedFolders(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Create a folder; folders lists it.
	if err := run([]string{"saved", "folder", "add", "reports"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("folder add: %v", err)
	}
	var folders strings.Builder
	if err := run([]string{"saved", "folders"}, noInput(), &folders, io.Discard); err != nil {
		t.Fatalf("folders: %v", err)
	}
	if !strings.Contains(folders.String(), "reports") {
		t.Fatalf("folders missing reports:\n%s", folders.String())
	}

	// Add a query filed under folder 1, plus an unfiled one.
	if err := run([]string{"saved", "add", "--title", "filed", "--sql", "select 1", "--folder", "1"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("saved add filed: %v", err)
	}
	if err := run([]string{"saved", "add", "--title", "loose", "--sql", "select 2"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("saved add loose: %v", err)
	}

	// list --folder 1 shows only the filed one.
	var inFolder strings.Builder
	if err := run([]string{"saved", "list", "--folder", "1"}, noInput(), &inFolder, io.Discard); err != nil {
		t.Fatalf("list --folder: %v", err)
	}
	if !strings.Contains(inFolder.String(), "filed") || strings.Contains(inFolder.String(), "loose") {
		t.Fatalf("list --folder 1 unexpected:\n%s", inFolder.String())
	}

	// mv the loose query (#2) into folder 1.
	if err := run([]string{"saved", "mv", "2", "--folder", "1"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("saved mv: %v", err)
	}
	var unfiled strings.Builder
	if err := run([]string{"saved", "list", "--folder", "0"}, noInput(), &unfiled, io.Discard); err != nil {
		t.Fatalf("list --folder 0: %v", err)
	}
	if !strings.Contains(unfiled.String(), "no saved queries") {
		t.Fatalf("expected no unfiled queries, got:\n%s", unfiled.String())
	}

	// Deleting the folder unfiles its queries (still present).
	if err := run([]string{"saved", "folder", "rm", "1"}, noInput(), io.Discard, io.Discard); err != nil {
		t.Fatalf("folder rm: %v", err)
	}
	var all strings.Builder
	if err := run([]string{"saved", "list"}, noInput(), &all, io.Discard); err != nil {
		t.Fatalf("saved list: %v", err)
	}
	if !strings.Contains(all.String(), "filed") || !strings.Contains(all.String(), "loose") {
		t.Fatalf("queries lost after folder delete:\n%s", all.String())
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
