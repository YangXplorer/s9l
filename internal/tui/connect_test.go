package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"
)

func sqliteCfg(id, path string) *config.Config {
	return &config.Config{Connections: []config.ConnectionConfig{
		{ID: id, Driver: "sqlite", Database: path},
	}}
}

func TestConnectSQLite(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := sqliteCfg("demo", db)
	a := New(Options{Config: cfg, Store: secret.NewMemory()})
	defer a.closeConn()

	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if a.conn == nil {
		t.Fatal("conn should be set after a successful connect")
	}
	if a.connID != "demo" {
		t.Errorf("connID = %q, want demo", a.connID)
	}
}

func TestConnectErrorDoesNotCrash(t *testing.T) {
	cfg := sqliteCfg("bad", "/nonexistent_dir/x.db")
	a := New(Options{Config: cfg, Store: secret.NewMemory()})

	cc, _ := cfg.Get("bad")
	if err := a.connect(cc); err == nil {
		t.Fatal("expected connect error for an unopenable path")
	}
	if a.conn != nil {
		t.Fatal("conn should stay nil after a failed connect")
	}
}

func TestAutoConnect(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	a := New(Options{Conn: "demo", Config: sqliteCfg("demo", db), Store: secret.NewMemory()})
	defer a.closeConn()

	if a.conn == nil {
		t.Fatal("auto-connect should have opened the connection")
	}
	if a.connID != "demo" {
		t.Errorf("connID = %q, want demo", a.connID)
	}
}

func TestConnectionsPopulated(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	if got := a.connList.GetItemCount(); got != 1 {
		t.Fatalf("connList items = %d, want 1", got)
	}
}

func TestLoadSchemaShowsTables(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := sqliteCfg("demo", db)
	a := New(Options{Config: cfg, Store: secret.NewMemory()})
	defer a.closeConn()

	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ctx := context.Background()
	if _, err := a.conn.Exec(ctx, "create table users(id int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.conn.Exec(ctx, "create table orders(id int)"); err != nil {
		t.Fatal(err)
	}
	a.loadSchema()

	got := map[string]bool{}
	for _, n := range a.schema.GetRoot().GetChildren() {
		got[n.GetText()] = true
		if ref, _ := n.GetReference().(string); ref != n.GetText() {
			t.Errorf("table node %q should carry its name as reference, got %q", n.GetText(), ref)
		}
	}
	if !got["users"] || !got["orders"] {
		t.Fatalf("schema tree missing tables, got %v", got)
	}
}
