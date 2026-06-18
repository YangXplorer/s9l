package secret_test

import (
	"testing"

	"github.com/YangXplorer/s9l/internal/secret"

	"github.com/zalando/go-keyring"
)

// These tests use go-keyring's in-memory mock — no real OS keychain I/O (which
// is left to manual verification per docs/TESTING.md).
func TestKeychainGetSetDelete(t *testing.T) {
	keyring.MockInit()
	var kc secret.Keychain

	if _, err := kc.Get(secret.Service, "missing"); err != secret.ErrNotFound {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
	if err := kc.Set(secret.Service, "k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := kc.Get(secret.Service, "k")
	if err != nil || got != "v" {
		t.Fatalf("Get = %q, %v; want v, nil", got, err)
	}
	if err := kc.Delete(secret.Service, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := kc.Get(secret.Service, "k"); err != secret.ErrNotFound {
		t.Fatalf("after delete = %v, want ErrNotFound", err)
	}
	// Delete of a missing key is a no-op.
	if err := kc.Delete(secret.Service, "k"); err != nil {
		t.Fatalf("delete missing = %v, want nil", err)
	}
}

func TestResolveKeychainRef(t *testing.T) {
	keyring.MockInit()
	kc := secret.Keychain{}
	if err := kc.Set(secret.Service, secret.ConnPasswordKey("pg"), "hunter2"); err != nil {
		t.Fatal(err)
	}

	v, err := secret.Resolve(kc, secret.KeychainRef("pg"))
	if err != nil || v != "hunter2" {
		t.Fatalf("Resolve(%q) = %q, %v; want hunter2, nil", secret.KeychainRef("pg"), v, err)
	}
}

func TestKeychainRefFormat(t *testing.T) {
	if got := secret.KeychainRef("pg"); got != "keychain://s9l/connection.pg.password" {
		t.Errorf("KeychainRef = %q", got)
	}
}
