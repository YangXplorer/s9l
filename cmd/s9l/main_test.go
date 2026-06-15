package main

import (
	"io"
	"strings"
	"testing"
)

func TestRunExecSelect(t *testing.T) {
	var out strings.Builder
	err := run([]string{":memory:", "-e", "SELECT 1 AS n"}, &out, io.Discard)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "n") || !strings.Contains(out.String(), "1") {
		t.Fatalf("unexpected output:\n%s", out.String())
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
