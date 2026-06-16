package secret

// Memory is an in-memory SecretStore. Secrets live only for the process
// lifetime and are never persisted — the Phase 1 default (passwords supplied at
// runtime). Phase 2 adds a keychain-backed store implementing the same
// interface.
type Memory struct {
	m map[string]string
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{m: make(map[string]string)}
}

func key(service, k string) string { return service + "\x00" + k }

// Get returns the stored secret or ErrNotFound.
func (s *Memory) Get(service, k string) (string, error) {
	v, ok := s.m[key(service, k)]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set stores a secret.
func (s *Memory) Set(service, k, value string) error {
	s.m[key(service, k)] = value
	return nil
}

// Delete removes a secret (no error if absent).
func (s *Memory) Delete(service, k string) error {
	delete(s.m, key(service, k))
	return nil
}
