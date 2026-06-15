package config

import "fmt"

// ConnectionConfig is one named connection. It never carries a plaintext
// password — PasswordRef points at a secret resolved via the secret package
// (e.g. an env var or, later, the system keychain). See docs/PLAN.md.
type ConnectionConfig struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name,omitempty"`
	Driver      string `yaml:"driver"`
	Host        string `yaml:"host,omitempty"`
	Port        int    `yaml:"port,omitempty"`
	User        string `yaml:"user,omitempty"`
	Database    string `yaml:"database,omitempty"`
	SSL         bool   `yaml:"ssl,omitempty"`
	Charset     string `yaml:"charset,omitempty"`
	PasswordRef string `yaml:"password_ref,omitempty"`
}

// DSN builds the driver-specific data source name. Phase 1 supports SQLite,
// whose DSN is just the database file path; other drivers build their DSN when
// their adapter lands (PostgreSQL in P1-B1, MySQL in P2-1). password is the
// resolved secret (may be empty for drivers that need none, e.g. SQLite).
func (c ConnectionConfig) DSN(password string) (string, error) {
	switch c.Driver {
	case "sqlite":
		if c.Database == "" {
			return "", fmt.Errorf("connection %q: sqlite requires a database path", c.ID)
		}
		return c.Database, nil
	case "":
		return "", fmt.Errorf("connection %q: missing driver", c.ID)
	default:
		return "", fmt.Errorf("connection %q: DSN building for driver %q not implemented yet", c.ID, c.Driver)
	}
}
