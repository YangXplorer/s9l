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
	main, _ := a.connList.GetItemText(0)
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
