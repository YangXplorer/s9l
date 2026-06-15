// Package secret abstracts credential storage behind a SecretStore interface.
// Phase 1 ships an in-memory implementation (prompt at startup, never
// persisted); Phase 2 adds a system keychain backend (zalando/go-keyring).
// See docs/PLAN.md "存储与凭据架构".
package secret
