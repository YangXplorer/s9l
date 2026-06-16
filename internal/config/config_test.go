package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/google/go-cmp/cmp"
)

func TestLoadFromMissingFile(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(cfg.Connections) != 0 {
		t.Fatalf("expected empty config, got %d connections", len(cfg.Connections))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := &config.Config{Connections: []config.ConnectionConfig{
		{ID: "local", Driver: "sqlite", Database: "./app.db"},
		{ID: "dev", Name: "Dev PG", Driver: "postgres", Host: "127.0.0.1", Port: 5432, User: "dev", Database: "app", SSL: true, PasswordRef: "env:PGPASSWORD"},
	}}
	if err := want.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 0600 permissions because it may reference credentials.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}

	got, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if diff := cmp.Diff(want.Connections, got.Connections); diff != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", diff)
	}
}

func TestParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("connections: [this is: not valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFrom(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAddGetRemove(t *testing.T) {
	var cfg config.Config
	if err := cfg.Add(config.ConnectionConfig{ID: "a", Driver: "sqlite", Database: "a.db"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := cfg.Add(config.ConnectionConfig{ID: "a", Driver: "sqlite"}); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if err := cfg.Add(config.ConnectionConfig{Driver: "sqlite"}); err == nil {
		t.Fatal("expected empty id error")
	}
	if _, ok := cfg.Get("a"); !ok {
		t.Fatal("Get(a) should exist")
	}
	if !cfg.Remove("a") {
		t.Fatal("Remove(a) should report true")
	}
	if cfg.Remove("a") {
		t.Fatal("Remove(a) again should report false")
	}
}

func TestDSN(t *testing.T) {
	dsn, err := config.ConnectionConfig{ID: "x", Driver: "sqlite", Database: "./x.db"}.DSN("")
	if err != nil || dsn != "./x.db" {
		t.Fatalf("sqlite DSN = %q, %v; want ./x.db, nil", dsn, err)
	}
	if _, err := (config.ConnectionConfig{ID: "x", Driver: "sqlite"}).DSN(""); err == nil {
		t.Fatal("sqlite without database should error")
	}

	// postgres builds a postgres:// URL with escaped credentials and sslmode.
	pg := config.ConnectionConfig{ID: "x", Driver: "postgres", Host: "db", Port: 5432, User: "dev", Database: "app", SSL: true}
	got, err := pg.DSN("p@ss/word")
	if err != nil {
		t.Fatalf("postgres DSN: %v", err)
	}
	want := "postgres://dev:p%40ss%2Fword@db:5432/app?sslmode=require"
	if got != want {
		t.Fatalf("postgres DSN = %q, want %q", got, want)
	}

	if _, err := (config.ConnectionConfig{ID: "x", Driver: "mysql"}).DSN(""); err == nil {
		t.Fatal("unimplemented driver should error")
	}
}
