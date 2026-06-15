package secret_test

import (
	"testing"

	"github.com/YangXplorer/s9l/internal/secret"
)

func TestMemoryStore(t *testing.T) {
	s := secret.NewMemory()
	if _, err := s.Get(secret.Service, "missing"); err != secret.ErrNotFound {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
	if err := s.Set(secret.Service, "k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(secret.Service, "k")
	if err != nil || got != "v" {
		t.Fatalf("Get = %q, %v; want v, nil", got, err)
	}
	if err := s.Delete(secret.Service, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(secret.Service, "k"); err != secret.ErrNotFound {
		t.Fatalf("after delete = %v, want ErrNotFound", err)
	}
}

func TestResolveEmpty(t *testing.T) {
	v, err := secret.Resolve(secret.NewMemory(), "")
	if err != nil || v != "" {
		t.Fatalf("empty ref = %q, %v; want empty, nil", v, err)
	}
}

func TestResolveEnv(t *testing.T) {
	t.Setenv("S9L_TEST_PW", "hunter2")
	v, err := secret.Resolve(secret.NewMemory(), "env:S9L_TEST_PW")
	if err != nil || v != "hunter2" {
		t.Fatalf("env ref = %q, %v; want hunter2, nil", v, err)
	}
	if _, err := secret.Resolve(secret.NewMemory(), "env:S9L_TEST_MISSING"); err == nil {
		t.Fatal("missing env var should error")
	}
}

func TestResolveKeychain(t *testing.T) {
	s := secret.NewMemory()
	if err := s.Set(secret.Service, "connection.local.password", "pw"); err != nil {
		t.Fatal(err)
	}
	v, err := secret.Resolve(s, "keychain://s9l/connection.local.password")
	if err != nil || v != "pw" {
		t.Fatalf("keychain ref = %q, %v; want pw, nil", v, err)
	}
	if _, err := secret.Resolve(s, "keychain://bad"); err == nil {
		t.Fatal("malformed keychain ref should error")
	}
}

func TestResolveUnsupported(t *testing.T) {
	if _, err := secret.Resolve(secret.NewMemory(), "vault:/foo"); err == nil {
		t.Fatal("unsupported ref scheme should error")
	}
}
