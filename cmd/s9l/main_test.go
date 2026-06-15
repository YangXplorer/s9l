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
