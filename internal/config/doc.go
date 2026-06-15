// Package config loads s9l's YAML connection configuration from
// $XDG_CONFIG_HOME/s9l/config.yaml (falling back to ~/.config/s9l/config.yaml).
// It never stores plaintext passwords — only a password_ref resolved via the
// secret package. Implemented in Phase 1 (see docs/TASKS.md A1–A3).
package config
