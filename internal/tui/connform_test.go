package tui

import (
	"testing"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/zalando/go-keyring"
)

// newFormApp returns an App with an empty config and the memory secret store,
// backed by a temp XDG dir so saveConnection's config.Save writes to a temp file.
func newFormApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return New(Options{Config: &config.Config{}, Store: secret.NewMemory()})
}

func TestSaveConnectionValidation(t *testing.T) {
	a := newFormApp(t)
	if err := a.saveConnection(config.ConnectionConfig{Driver: "postgres"}, ""); err == nil {
		t.Error("missing id should error")
	}
	if err := a.saveConnection(config.ConnectionConfig{ID: "x"}, ""); err == nil {
		t.Error("missing driver should error")
	}
	if err := a.saveConnection(config.ConnectionConfig{ID: "s", Driver: "sqlite"}, ""); err == nil {
		t.Error("sqlite without database should error")
	}
}

func TestSaveConnectionPersistsAndRefreshes(t *testing.T) {
	a := newFormApp(t)
	cc := config.ConnectionConfig{ID: "local", Driver: "sqlite", Database: "./app.db"}
	if err := a.saveConnection(cc, ""); err != nil {
		t.Fatalf("save: %v", err)
	}
	// In-memory config updated and list refreshed.
	if _, ok := a.cfg.Get("local"); !ok {
		t.Error("connection not added to config")
	}
	main := a.connTree.GetRoot().GetChildren()[0].GetText()
	if main == "" || main == "(no connections — press n to add)" {
		t.Errorf("connections list not refreshed: %q", main)
	}
	// Persisted to disk: a fresh Load sees it.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := loaded.Get("local"); !ok {
		t.Error("connection not written to config.yaml")
	}

	// Duplicate id is rejected.
	if err := a.saveConnection(cc, ""); err == nil {
		t.Error("duplicate id should error")
	}
}

func TestSaveConnectionPasswordToKeychain(t *testing.T) {
	keyring.MockInit() // in-memory keychain
	a := newFormApp(t)
	// Use the keychain store so the password is set/resolved via go-keyring.
	a.store = secret.Default()

	cc := config.ConnectionConfig{ID: "pg", Driver: "postgres", Host: "h", User: "u", Database: "d"}
	if err := a.saveConnection(cc, "sekret"); err != nil {
		t.Fatalf("save: %v", err)
	}

	// config.yaml holds only the keychain ref, never the plaintext.
	saved, _ := a.cfg.Get("pg")
	if saved.PasswordRef != secret.KeychainRef("pg") {
		t.Errorf("password_ref = %q, want %q", saved.PasswordRef, secret.KeychainRef("pg"))
	}
	// The password resolves back from the keychain.
	got, err := secret.Resolve(a.store, saved.PasswordRef)
	if err != nil || got != "sekret" {
		t.Fatalf("resolve = %q, %v; want sekret, nil", got, err)
	}
}

func TestEditConnection(t *testing.T) {
	a := newFormApp(t)
	if err := a.saveConnection(config.ConnectionConfig{ID: "db", Driver: "sqlite", Database: "./a.db"}, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Rename + change database; keep it persisted and reloadable.
	edited := config.ConnectionConfig{ID: "db2", Driver: "sqlite", Database: "./b.db"}
	if err := a.editConnection("db", edited, ""); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if _, ok := a.cfg.Get("db"); ok {
		t.Error("old id should be gone after rename")
	}
	got, ok := a.cfg.Get("db2")
	if !ok || got.Database != "./b.db" {
		t.Fatalf("edited conn = %+v, ok=%v", got, ok)
	}
	loaded, _ := config.Load()
	if _, ok := loaded.Get("db2"); !ok {
		t.Error("edit not persisted to config.yaml")
	}

	// Editing a missing connection errors.
	if err := a.editConnection("nope", edited, ""); err == nil {
		t.Error("editing missing connection should error")
	}

	// Renaming onto an existing id is rejected (add a second, try to collide).
	if err := a.saveConnection(config.ConnectionConfig{ID: "other", Driver: "sqlite", Database: "./c.db"}, ""); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	clash := config.ConnectionConfig{ID: "other", Driver: "sqlite", Database: "./b.db"}
	if err := a.editConnection("db2", clash, ""); err == nil {
		t.Error("renaming onto an existing id should error")
	}
	// The original must survive a rejected edit.
	if _, ok := a.cfg.Get("db2"); !ok {
		t.Error("db2 should be restored after a failed rename")
	}
}

func TestEditConnectionPreservesCharset(t *testing.T) {
	a := newFormApp(t)
	// Seed a connection with a charset (no form field for it).
	a.cfg.Connections = []config.ConnectionConfig{
		{ID: "my", Driver: "mysql", Host: "h", User: "u", Database: "d", Charset: "utf8mb4"},
	}
	// Edit (form-rebuilt cc has no charset); it must be preserved.
	edited := config.ConnectionConfig{ID: "my", Driver: "mysql", Host: "h2", User: "u", Database: "d"}
	if err := a.editConnection("my", edited, ""); err != nil {
		t.Fatalf("edit: %v", err)
	}
	got, _ := a.cfg.Get("my")
	if got.Charset != "utf8mb4" {
		t.Errorf("charset = %q after edit, want utf8mb4 (preserved)", got.Charset)
	}
	if got.Host != "h2" {
		t.Errorf("host = %q, want h2 (edit applied)", got.Host)
	}
}

func TestEditConnectionUpdatesPassword(t *testing.T) {
	keyring.MockInit()
	a := newFormApp(t)
	a.store = secret.Default()
	if err := a.saveConnection(config.ConnectionConfig{ID: "pg", Driver: "postgres", User: "u", Database: "d"}, "old"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Edit with a new password updates the keychain entry.
	cc, _ := a.cfg.Get("pg")
	if err := a.editConnection("pg", cc, "new"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	got, err := secret.Resolve(a.store, secret.KeychainRef("pg"))
	if err != nil || got != "new" {
		t.Fatalf("password after edit = %q, %v; want new", got, err)
	}
}

func TestDeleteConnection(t *testing.T) {
	keyring.MockInit()
	a := newFormApp(t)
	a.store = secret.Default()
	if err := a.saveConnection(config.ConnectionConfig{ID: "pg", Driver: "postgres", User: "u", Database: "d"}, "pw"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := a.deleteConnection("pg"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := a.cfg.Get("pg"); ok {
		t.Error("connection should be gone")
	}
	loaded, _ := config.Load()
	if _, ok := loaded.Get("pg"); ok {
		t.Error("delete not persisted to config.yaml")
	}
	// Keychain password removed too.
	if _, err := a.store.Get(secret.Service, secret.ConnPasswordKey("pg")); err != secret.ErrNotFound {
		t.Errorf("keychain secret should be gone, got err=%v", err)
	}
	// Deleting again reports an error.
	if err := a.deleteConnection("pg"); err == nil {
		t.Error("deleting missing connection should error")
	}
}

func TestSelectedConn(t *testing.T) {
	a := newFormApp(t)
	if _, ok := a.selectedConn(); ok {
		t.Error("no selection when there are no connections")
	}
	for _, id := range []string{"a", "b", "c"} {
		if err := a.saveConnection(config.ConnectionConfig{ID: id, Driver: "sqlite", Database: "./" + id + ".db"}, ""); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	a.connTree.SetCurrentNode(a.connTree.GetRoot().GetChildren()[1]) // "b"
	cc, ok := a.selectedConn()
	if !ok || cc.ID != "b" {
		t.Fatalf("selectedConn = %+v ok=%v, want b", cc, ok)
	}
}
