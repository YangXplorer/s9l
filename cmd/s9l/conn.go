package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/secret"
)

// runConn dispatches the `s9l conn <list|add|rm>` subcommands.
func runConn(args []string, out, errOut io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: s9l conn <list|add|rm>")
	}
	switch args[0] {
	case "list":
		return connList(out)
	case "add":
		return connAdd(args[1:], errOut)
	case "rm":
		return connRm(args[1:])
	default:
		return fmt.Errorf("unknown conn subcommand %q (want list|add|rm)", args[0])
	}
}

func connList(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Connections) == 0 {
		_, err := fmt.Fprintln(out, "no connections configured")
		return err
	}
	for _, c := range cfg.Connections {
		desc := c.Driver
		if c.Database != "" {
			desc += " " + c.Database
		}
		if c.Host != "" {
			desc += fmt.Sprintf(" %s@%s:%d", c.User, c.Host, c.Port)
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\n", c.ID, desc); err != nil {
			return err
		}
	}
	return nil
}

func connAdd(args []string, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l conn add", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var c config.ConnectionConfig
	fs.StringVar(&c.ID, "id", "", "connection id (required)")
	fs.StringVar(&c.Name, "name", "", "display name")
	fs.StringVar(&c.Driver, "driver", "", "driver (required), e.g. sqlite/postgres")
	fs.StringVar(&c.Host, "host", "", "host")
	fs.IntVar(&c.Port, "port", 0, "port")
	fs.StringVar(&c.User, "user", "", "user")
	fs.StringVar(&c.Database, "database", "", "database name or sqlite file path")
	fs.BoolVar(&c.SSL, "ssl", false, "use SSL/TLS")
	fs.StringVar(&c.SSLMode, "ssl-mode", "", "TLS mode (e.g. require|verify-full; mysql: skip-verify|preferred)")
	fs.StringVar(&c.TLSCA, "tls-ca", "", "CA certificate file (postgres, sqlserver)")
	fs.StringVar(&c.TLSCert, "tls-cert", "", "client certificate file (postgres)")
	fs.StringVar(&c.TLSKey, "tls-key", "", "client key file (postgres)")
	fs.StringVar(&c.SSHHost, "ssh-host", "", "SSH bastion host (enables tunneling)")
	fs.IntVar(&c.SSHPort, "ssh-port", 0, "SSH bastion port (default 22)")
	fs.StringVar(&c.SSHUser, "ssh-user", "", "SSH user")
	fs.StringVar(&c.SSHKey, "ssh-key", "", "SSH private key file (else the SSH agent)")
	fs.StringVar(&c.SSHKnownHosts, "ssh-known-hosts", "", "known_hosts file (default ~/.ssh/known_hosts)")
	fs.BoolVar(&c.SSHInsecureHostKey, "ssh-insecure-host-key", false, "skip SSH host-key verification (INSECURE)")
	fs.StringVar(&c.Charset, "charset", "", "charset")
	fs.StringVar(&c.PasswordRef, "password-ref", "", "password reference, e.g. env:PGPASSWORD or keychain://s9l/connection.<id>.password")
	password := fs.String("password", "", "store this password in the OS keychain (sets password_ref automatically)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if c.Driver == "" {
		return errors.New("conn add: --driver is required")
	}
	if c.ID == "" {
		return errors.New("conn add: --id is required")
	}

	// A given password is stored in the OS keychain (never in config.yaml); the
	// connection references it via password_ref.
	if *password != "" && c.PasswordRef == "" {
		c.PasswordRef = secret.KeychainRef(c.ID)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Add(c); err != nil { // validates the id is unique before we touch the keychain
		return err
	}
	if *password != "" {
		if err := secret.Default().Set(secret.Service, secret.ConnPasswordKey(c.ID), *password); err != nil {
			return fmt.Errorf("store password in keychain: %w", err)
		}
	}
	return cfg.Save()
}

func connRm(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: s9l conn rm <id>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Remove(args[0]) {
		return fmt.Errorf("connection %q not found", args[0])
	}
	// Best-effort: drop any keychain password for this connection.
	_ = secret.Default().Delete(secret.Service, secret.ConnPasswordKey(args[0]))
	return cfg.Save()
}
