// Package secret abstracts credential storage behind a SecretStore interface,
// so connection passwords never live in config.yaml. Phase 1 ships an in-memory
// store (with env-var resolution); Phase 2 adds a system keychain backend
// (zalando/go-keyring). See docs/PLAN.md "存储与凭据架构".
package secret

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Service is the namespace under which s9l stores secrets (e.g. in a keychain).
const Service = "s9l"

// ErrNotFound is returned when a secret cannot be resolved.
var ErrNotFound = errors.New("secret: not found")

// SecretStore stores and retrieves secrets by (service, key).
type SecretStore interface {
	Get(service, key string) (string, error)
	Set(service, key, value string) error
	Delete(service, key string) error
}

// Resolve returns the password for a connection given its password_ref.
//
// Supported ref forms (Phase 1):
//   - ""                      → no password (e.g. SQLite)
//   - "env:NAME"              → value of environment variable NAME
//   - "keychain://s9l/<key>"  → store.Get(Service, <key>)
//
// The keychain form works with any SecretStore (the in-memory one in Phase 1,
// a real keychain in Phase 2) without changing callers.
func Resolve(store SecretStore, ref string) (string, error) {
	switch {
	case ref == "":
		return "", nil
	case strings.HasPrefix(ref, "env:"):
		name := strings.TrimPrefix(ref, "env:")
		v, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("secret: env var %q not set", name)
		}
		return v, nil
	case strings.HasPrefix(ref, "keychain://"):
		rest := strings.TrimPrefix(ref, "keychain://")
		svc, key, ok := strings.Cut(rest, "/")
		if !ok || svc == "" || key == "" {
			return "", fmt.Errorf("secret: invalid keychain ref %q (want keychain://service/key)", ref)
		}
		return store.Get(svc, key)
	default:
		return "", fmt.Errorf("secret: unsupported password_ref %q", ref)
	}
}
