package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

	// mysql builds a go-sql-driver DSN.
	my := config.ConnectionConfig{ID: "x", Driver: "mysql", Host: "db", Port: 3306, User: "dev", Database: "app"}
	myDSN, err := my.DSN("pw")
	if err != nil {
		t.Fatalf("mysql DSN: %v", err)
	}
	if myDSN != "dev:pw@tcp(db:3306)/app?parseTime=true" {
		t.Fatalf("mysql DSN = %q", myDSN)
	}

	// sqlserver builds a sqlserver:// URL with escaped credentials and encrypt.
	ms := config.ConnectionConfig{ID: "x", Driver: "sqlserver", Host: "db", Port: 1433, User: "sa", Database: "app", SSL: true}
	msDSN, err := ms.DSN("p@ss")
	if err != nil {
		t.Fatalf("sqlserver DSN: %v", err)
	}
	if msDSN != "sqlserver://sa:p%40ss@db:1433?database=app&encrypt=true" {
		t.Fatalf("sqlserver DSN = %q", msDSN)
	}

	// clickhouse builds a clickhouse:// URL (default port 9000).
	ch := config.ConnectionConfig{ID: "x", Driver: "clickhouse", Host: "db", User: "dev", Database: "app"}
	chDSN, err := ch.DSN("pw")
	if err != nil {
		t.Fatalf("clickhouse DSN: %v", err)
	}
	if chDSN != "clickhouse://dev:pw@db:9000/app" {
		t.Fatalf("clickhouse DSN = %q", chDSN)
	}
	// SSL → secure transport.
	chSecure := config.ConnectionConfig{ID: "x", Driver: "clickhouse", Host: "db", Port: 9440, Database: "app", SSL: true}
	d, _ := chSecure.DSN("")
	if !strings.Contains(d, "secure=true") {
		t.Errorf("clickhouse ssl should set secure=true: %q", d)
	}

	if _, err := (config.ConnectionConfig{ID: "x", Driver: "mongodb"}).DSN(""); err == nil {
		t.Fatal("unimplemented driver should error")
	}
}

func TestSSHHelpers(t *testing.T) {
	if (config.ConnectionConfig{}).HasSSH() {
		t.Error("no ssh_host → HasSSH false")
	}
	if !(config.ConnectionConfig{SSHHost: "bastion"}).HasSSH() {
		t.Error("ssh_host set → HasSSH true")
	}

	// DialHostPort uses the configured port, else the driver default.
	h, p := config.ConnectionConfig{Driver: "postgres", Host: "db"}.DialHostPort()
	if h != "db" || p != 5432 {
		t.Errorf("postgres default = %s:%d, want db:5432", h, p)
	}
	_, p = config.ConnectionConfig{Driver: "mysql", Port: 3307}.DialHostPort()
	if p != 3307 {
		t.Errorf("explicit port = %d, want 3307", p)
	}
	_, p = config.ConnectionConfig{Driver: "clickhouse"}.DialHostPort()
	if p != 9000 {
		t.Errorf("clickhouse default = %d, want 9000", p)
	}
}

func TestDSNTLS(t *testing.T) {
	// postgres: ssl_mode + CA/client cert map to libpq params.
	pg := config.ConnectionConfig{ID: "x", Driver: "postgres", Host: "db", Port: 5432, User: "dev", Database: "app",
		SSLMode: "verify-full", TLSCA: "/ca.pem", TLSCert: "/c.pem", TLSKey: "/k.pem"}
	got, err := pg.DSN("")
	if err != nil {
		t.Fatalf("pg tls: %v", err)
	}
	want := "postgres://dev@db:5432/app?sslcert=%2Fc.pem&sslkey=%2Fk.pem&sslmode=verify-full&sslrootcert=%2Fca.pem"
	if got != want {
		t.Fatalf("pg tls DSN:\n got %s\nwant %s", got, want)
	}

	// mysql: ssl_mode maps to the tls parameter.
	for mode, tls := range map[string]string{"require": "true", "skip-verify": "skip-verify", "preferred": "preferred", "disable": ""} {
		my := config.ConnectionConfig{ID: "x", Driver: "mysql", Host: "db", Port: 3306, User: "u", Database: "app", SSLMode: mode}
		got, err := my.DSN("p")
		if err != nil {
			t.Fatalf("mysql %s: %v", mode, err)
		}
		wantHas := tls != ""
		if has := strings.Contains(got, "tls="+tls); wantHas && !has {
			t.Errorf("mysql ssl_mode=%s → %q, want tls=%s", mode, got, tls)
		}
		if mode == "disable" && strings.Contains(got, "tls=") {
			t.Errorf("mysql disable should omit tls: %q", got)
		}
	}

	// mysql with cert files is rejected (needs a raw DSN).
	if _, err := (config.ConnectionConfig{ID: "x", Driver: "mysql", TLSCA: "/ca.pem"}).DSN(""); err == nil {
		t.Error("mysql with tls_ca should error")
	}

	// sqlserver: ssl:true verifies (encrypt=true, no trust); ssl_mode=require skips verify.
	msVerify := config.ConnectionConfig{ID: "x", Driver: "sqlserver", Host: "db", Port: 1433, User: "sa", Database: "app", SSL: true}
	d, _ := msVerify.DSN("")
	if !strings.Contains(d, "encrypt=true") || strings.Contains(d, "trustservercertificate") {
		t.Errorf("sqlserver ssl:true should verify (encrypt=true, no trust): %q", d)
	}
	msReq := config.ConnectionConfig{ID: "x", Driver: "sqlserver", Host: "db", User: "sa", Database: "app", SSLMode: "require", TLSCA: "/ca.pem"}
	d, _ = msReq.DSN("")
	if !strings.Contains(d, "trustservercertificate=true") || !strings.Contains(d, "certificate=%2Fca.pem") {
		t.Errorf("sqlserver require+ca: %q", d)
	}
	// sqlserver client cert rejected.
	if _, err := (config.ConnectionConfig{ID: "x", Driver: "sqlserver", TLSCert: "/c.pem"}).DSN(""); err == nil {
		t.Error("sqlserver with client cert should error")
	}
}
