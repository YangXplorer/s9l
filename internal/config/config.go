// Package config loads and persists s9l's YAML connection configuration from
// $XDG_CONFIG_HOME/s9l/config.yaml (falling back to ~/.config/s9l/config.yaml).
// It never stores plaintext passwords — only a password_ref resolved via the
// secret package. See docs/PLAN.md "存储与凭据架构".
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// fileMode is the permission for the config file: owner read/write only, since
// it may reference credentials.
const fileMode = 0o600

// Config is the top-level configuration document.
type Config struct {
	Connections []ConnectionConfig `yaml:"connections"`
}

// Path returns the config file path, honoring $XDG_CONFIG_HOME and falling back
// to ~/.config/s9l/config.yaml.
func Path() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "s9l", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".config", "s9l", "config.yaml"), nil
}

// Load reads the config from its default path. A missing file is not an error;
// it yields an empty Config.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads the config from a specific path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Get returns the connection with the given id.
func (c *Config) Get(id string) (ConnectionConfig, bool) {
	for _, conn := range c.Connections {
		if conn.ID == id {
			return conn, true
		}
	}
	return ConnectionConfig{}, false
}

// Add appends a connection. It returns an error if the id is empty or already
// exists.
func (c *Config) Add(conn ConnectionConfig) error {
	if conn.ID == "" {
		return errors.New("config: connection id is required")
	}
	if _, ok := c.Get(conn.ID); ok {
		return fmt.Errorf("config: connection %q already exists", conn.ID)
	}
	c.Connections = append(c.Connections, conn)
	return nil
}

// Remove deletes the connection with the given id, reporting whether it existed.
func (c *Config) Remove(id string) bool {
	for i, conn := range c.Connections {
		if conn.ID == id {
			c.Connections = append(c.Connections[:i], c.Connections[i+1:]...)
			return true
		}
	}
	return false
}

// IDs returns the sorted connection ids.
func (c *Config) IDs() []string {
	ids := make([]string, 0, len(c.Connections))
	for _, conn := range c.Connections {
		ids = append(ids, conn.ID)
	}
	sort.Strings(ids)
	return ids
}

// Save writes the config to its default path, creating the directory as needed.
// The file is written with 0600 permissions (it may reference credentials).
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return c.SaveTo(path)
}

// SaveTo writes the config to a specific path.
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, fileMode); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
