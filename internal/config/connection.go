package config

import (
	"fmt"
	"net/url"
	"strconv"
)

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
	case "postgres":
		return c.postgresDSN(password), nil
	case "":
		return "", fmt.Errorf("connection %q: missing driver", c.ID)
	default:
		return "", fmt.Errorf("connection %q: DSN building for driver %q not implemented yet", c.ID, c.Driver)
	}
}

// postgresDSN builds a postgres:// URL. sslmode follows SSL (require/disable);
// the password (if any) is added as userinfo so url escaping handles it.
func (c ConnectionConfig) postgresDSN(password string) string {
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	if c.Port > 0 {
		host = host + ":" + strconv.Itoa(c.Port)
	}
	u := url.URL{
		Scheme: "postgres",
		Host:   host,
		Path:   "/" + c.Database,
	}
	if c.User != "" {
		if password != "" {
			u.User = url.UserPassword(c.User, password)
		} else {
			u.User = url.User(c.User)
		}
	}
	q := url.Values{}
	if c.SSL {
		q.Set("sslmode", "require")
	} else {
		q.Set("sslmode", "disable")
	}
	u.RawQuery = q.Encode()
	return u.String()
}
