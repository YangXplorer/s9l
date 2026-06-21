package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"
)

func TestBuildUpdate(t *testing.T) {
	// postgres: $n placeholders, double-quoted idents, NULL → IS NULL.
	sql, args := buildUpdate("postgres", `"users"`, []string{"id", "name", "email"}, 2, "new@x", []any{int64(1), "alice", nil})
	wantSQL := `UPDATE "users" SET "email" = $1 WHERE "id" = $2 AND "name" = $3 AND "email" IS NULL`
	if sql != wantSQL {
		t.Errorf("pg sql =\n  %s\nwant\n  %s", sql, wantSQL)
	}
	if len(args) != 3 || args[0] != "new@x" || args[1] != int64(1) || args[2] != "alice" {
		t.Errorf("pg args = %v, want [new@x 1 alice]", args)
	}

	// mysql: ? placeholders, backtick idents; WHERE includes the edited column's
	// original value.
	sql, args = buildUpdate("mysql", "`t`", []string{"a", "b"}, 0, "x", []any{int64(5), nil})
	if sql != "UPDATE `t` SET `a` = ? WHERE `a` = ? AND `b` IS NULL" {
		t.Errorf("mysql sql = %s", sql)
	}
	if len(args) != 2 || args[0] != "x" || args[1] != int64(5) {
		t.Errorf("mysql args = %v, want [x 5]", args)
	}

	// sqlserver: @pN placeholders.
	sql, _ = buildUpdate("sqlserver", `"t"`, []string{"a"}, 0, "x", []any{int64(9)})
	if sql != `UPDATE "t" SET "a" = @p1 WHERE "a" = @p2` {
		t.Errorf("sqlserver sql = %s", sql)
	}
}

// End-to-end: the SQL buildUpdate produces is valid and updates the right row.
func TestBuildUpdateExecutesOnSQLite(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	cfg := config.Config{Connections: []config.ConnectionConfig{{ID: "demo", Driver: "sqlite", Database: db}}}
	a := New(Options{Config: &cfg, Store: secret.NewMemory()})
	defer a.closeConn()
	cc, _ := cfg.Get("demo")
	if err := a.connect(cc); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ctx := context.Background()
	if _, err := a.conn.Exec(ctx, "CREATE TABLE t(id INTEGER, name TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := a.conn.Exec(ctx, "INSERT INTO t VALUES (1,'alice'),(2,'bob')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	sql, args := buildUpdate("sqlite", quoteIdent("sqlite", "t"), []string{"id", "name"}, 1, "ALICE", []any{int64(1), "alice"})
	if _, err := a.conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("update exec: %v (%s)", err, sql)
	}

	got, err := namesFrom(a.conn.Query(ctx, "SELECT name FROM t WHERE id = 1"))
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(got) != 1 || got[0] != "ALICE" {
		t.Errorf("name after update = %v, want [ALICE]", got)
	}
	// The other row is untouched.
	if other, _ := namesFrom(a.conn.Query(ctx, "SELECT name FROM t WHERE id = 2")); len(other) != 1 || other[0] != "bob" {
		t.Errorf("row 2 name = %v, want [bob]", other)
	}
}

// showCellEdit is a no-op (no overlay) when the result is not a single-table
// preview.
func TestShowCellEditGuardsNonEditable(t *testing.T) {
	a := New(Options{Config: sqliteCfg("demo", "x.db"), Store: secret.NewMemory()})
	a.setResults([]string{"id"}, sampleRows())
	a.resultEditable = false
	a.showCellEdit()
	if a.cellEditOpen {
		t.Error("cell edit overlay should not open for a non-editable result")
	}
}
