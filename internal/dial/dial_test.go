package dial_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/dial"
	"github.com/YangXplorer/s9l/internal/secret"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"
)

// Open without SSH opens the driver directly and the close func releases it.
func TestOpenNoSSH(t *testing.T) {
	db := filepath.Join(t.TempDir(), "d.db")
	cc := config.ConnectionConfig{ID: "x", Driver: "sqlite", Database: db}

	conn, closer, err := dial.Open(context.Background(), cc, secret.NewMemory())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := conn.Exec(context.Background(), "create table t(a int)"); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if err := closer(); err != nil {
		t.Errorf("close: %v", err)
	}
}

// A missing password reference surfaces as an error (not a panic).
func TestOpenPasswordRefError(t *testing.T) {
	cc := config.ConnectionConfig{ID: "x", Driver: "postgres", Host: "h", User: "u", Database: "d",
		PasswordRef: "env:S9L_DEFINITELY_UNSET_VAR"}
	if _, _, err := dial.Open(context.Background(), cc, secret.NewMemory()); err == nil {
		t.Error("expected error for unset password ref")
	}
}
