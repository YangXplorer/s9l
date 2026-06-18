package secret

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// Keychain is a SecretStore backed by the OS keychain — macOS Keychain, Windows
// Credential Manager, or the Linux Secret Service — via zalando/go-keyring.
// It is the production store; tests use Memory or go-keyring's MockInit.
type Keychain struct{}

// Get returns the stored secret, mapping the backend's not-found to ErrNotFound.
func (Keychain) Get(service, key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

// Set stores a secret in the OS keychain.
func (Keychain) Set(service, key, value string) error {
	return keyring.Set(service, key, value)
}

// Delete removes a secret (no error if it does not exist).
func (Keychain) Delete(service, key string) error {
	err := keyring.Delete(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Default returns the production SecretStore (the OS keychain). The keychain is
// only touched for keychain:// password refs, so env:/no-password connections
// work even on systems without a keyring backend.
func Default() SecretStore { return Keychain{} }

// ConnPasswordKey is the keychain key under Service for a connection's password.
func ConnPasswordKey(id string) string { return "connection." + id + ".password" }

// KeychainRef is the password_ref pointing at a connection's keychain entry.
func KeychainRef(id string) string { return "keychain://" + Service + "/" + ConnPasswordKey(id) }
