package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
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

	// TLS options (finer than the SSL bool). SSLMode overrides SSL when set:
	//   postgres   disable|require|verify-ca|verify-full (libpq sslmode)
	//   mysql      disable|require|skip-verify|preferred  (go-sql-driver tls)
	//   sqlserver  disable|require|verify-full            (encrypt/verify)
	// TLSCA/TLSCert/TLSKey are file paths: CA (postgres, sqlserver) and client
	// cert/key (postgres only) — others need a raw DSN.
	SSLMode string `yaml:"ssl_mode,omitempty"`
	TLSCA   string `yaml:"tls_ca,omitempty"`
	TLSCert string `yaml:"tls_cert,omitempty"`
	TLSKey  string `yaml:"tls_key,omitempty"`
}

// DSN builds the driver-specific data source name. Phase 1 supports SQLite,
// whose DSN is just the database file path; other drivers build their DSN when
// their adapter lands (PostgreSQL in P1-B1, MySQL in P2-1). password is the
// resolved secret (may be empty for drivers that need none, e.g. SQLite).
func (c ConnectionConfig) DSN(password string) (string, error) {
	if err := c.validateTLS(); err != nil {
		return "", err
	}
	switch c.Driver {
	case "sqlite":
		if c.Database == "" {
			return "", fmt.Errorf("connection %q: sqlite requires a database path", c.ID)
		}
		return c.Database, nil
	case "postgres":
		return c.postgresDSN(password), nil
	case "mysql":
		return c.mysqlDSN(password), nil
	case "sqlserver":
		return c.sqlserverDSN(password), nil
	case "clickhouse":
		return c.clickhouseDSN(password), nil
	case "":
		return "", fmt.Errorf("connection %q: missing driver", c.ID)
	default:
		return "", fmt.Errorf("connection %q: DSN building for driver %q not implemented yet", c.ID, c.Driver)
	}
}

// validateTLS rejects TLS file options on drivers that can't honor them via the
// config DSN, so the user gets a clear error instead of silently-ignored certs.
func (c ConnectionConfig) validateTLS() error {
	if c.TLSCA == "" && c.TLSCert == "" && c.TLSKey == "" {
		return nil
	}
	switch c.Driver {
	case "postgres":
		return nil // CA + client cert all supported
	case "sqlserver":
		if c.TLSCert != "" || c.TLSKey != "" {
			return fmt.Errorf("connection %q: sqlserver supports tls_ca but not client certs via config; use a raw DSN", c.ID)
		}
		return nil
	case "mysql":
		return fmt.Errorf("connection %q: mysql TLS certs require a raw DSN (RegisterTLSConfig); use ssl_mode for built-in modes", c.ID)
	case "clickhouse":
		return fmt.Errorf("connection %q: clickhouse TLS certs require a raw DSN; use ssl/ssl_mode for secure mode", c.ID)
	default:
		return fmt.Errorf("connection %q: driver %q has no TLS file options", c.ID, c.Driver)
	}
}

// sslMode returns the effective SSL mode: SSLMode if set, otherwise derived from
// the SSL bool (whenOn when true, "disable" when false).
func (c ConnectionConfig) sslMode(whenOn string) string {
	if c.SSLMode != "" {
		return c.SSLMode
	}
	if c.SSL {
		return whenOn
	}
	return "disable"
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
	q.Set("sslmode", c.sslMode("require"))
	if c.TLSCA != "" {
		q.Set("sslrootcert", c.TLSCA)
	}
	if c.TLSCert != "" {
		q.Set("sslcert", c.TLSCert)
	}
	if c.TLSKey != "" {
		q.Set("sslkey", c.TLSKey)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// mysqlDSN builds a go-sql-driver/mysql DSN:
//
//	user:password@tcp(host:port)/database?parseTime=true[&tls=true][&charset=...]
//
// parseTime makes DATE/DATETIME scan into time.Time. Note: passwords containing
// '@' or '/' are not escaped here — use an env/keychain ref without those, or a
// raw DSN, for such passwords.
func (c ConnectionConfig) mysqlDSN(password string) string {
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	port := c.Port
	if port == 0 {
		port = 3306
	}
	auth := c.User
	if password != "" {
		auth = c.User + ":" + password
	}
	q := url.Values{}
	q.Set("parseTime", "true")
	if c.Charset != "" {
		q.Set("charset", c.Charset)
	}
	if tls := mysqlTLS(c.sslMode("true")); tls != "" {
		q.Set("tls", tls)
	}
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?%s", auth, host, port, c.Database, q.Encode())
}

// sqlserverDSN builds a microsoft/go-mssqldb URL:
//
//	sqlserver://user:password@host:port?database=db&encrypt=disable|true
//
// SSL toggles encrypt (require vs disable); the database is a query parameter so
// the connection has a default database for INFORMATION_SCHEMA listings.
func (c ConnectionConfig) sqlserverDSN(password string) string {
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	if c.Port > 0 {
		host = host + ":" + strconv.Itoa(c.Port)
	}
	u := url.URL{Scheme: "sqlserver", Host: host}
	if c.User != "" {
		if password != "" {
			u.User = url.UserPassword(c.User, password)
		} else {
			u.User = url.User(c.User)
		}
	}
	q := url.Values{}
	if c.Database != "" {
		q.Set("database", c.Database)
	}
	// Default ssl:true verifies the certificate (encrypt=true); ssl_mode=require
	// encrypts without verification.
	switch strings.ToLower(c.sslMode("verify-full")) {
	case "disable", "false":
		q.Set("encrypt", "disable")
	case "require": // encrypt but don't verify the server certificate
		q.Set("encrypt", "true")
		q.Set("trustservercertificate", "true")
	default: // verify-ca / verify-full / true: encrypt and verify
		q.Set("encrypt", "true")
	}
	if c.TLSCA != "" {
		q.Set("certificate", c.TLSCA)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// clickhouseDSN builds a clickhouse-go URL:
//
//	clickhouse://user:password@host:9000/db?secure=true[&skip_verify=true]
//
// SSL/ssl_mode toggles the secure transport; ssl_mode=require encrypts without
// verifying the server certificate (skip_verify).
func (c ConnectionConfig) clickhouseDSN(password string) string {
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	port := c.Port
	if port == 0 {
		port = 9000
	}
	u := url.URL{Scheme: "clickhouse", Host: fmt.Sprintf("%s:%d", host, port), Path: "/" + c.Database}
	if c.User != "" {
		if password != "" {
			u.User = url.UserPassword(c.User, password)
		} else {
			u.User = url.User(c.User)
		}
	}
	q := url.Values{}
	switch strings.ToLower(c.sslMode("verify-full")) {
	case "disable", "false":
		// plaintext (default)
	case "require":
		q.Set("secure", "true")
		q.Set("skip_verify", "true")
	default: // verify-ca / verify-full / true
		q.Set("secure", "true")
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// mysqlTLS maps an ssl mode to a go-sql-driver/mysql "tls" parameter value.
func mysqlTLS(mode string) string {
	switch strings.ToLower(mode) {
	case "", "disable", "false":
		return ""
	case "require", "true":
		return "true"
	case "skip-verify", "preferred":
		return strings.ToLower(mode)
	default:
		return mode // a custom config name registered by the caller
	}
}
