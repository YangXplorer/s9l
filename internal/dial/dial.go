// Package dial opens a database connection for a configured connection,
// transparently setting up an SSH tunnel first when the connection requests one.
// It is shared by the CLI and the TUI so both get identical connect behavior.
package dial

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/YangXplorer/s9l/internal/config"
	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/secret"
	"github.com/YangXplorer/s9l/internal/tunnel"
)

// splitHostPort parses "host:port" into its parts (port 0 if unparsable).
func splitHostPort(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

// Open resolves the connection's password, establishes an SSH tunnel if
// configured, and opens the driver against the (possibly tunneled) DSN. The
// returned close func shuts the connection and the tunnel; it is always safe to
// call and replaces a direct conn.Close().
func Open(ctx context.Context, cc config.ConnectionConfig, store secret.SecretStore) (driver.Conn, func() error, error) {
	password, err := secret.Resolve(store, cc.PasswordRef)
	if err != nil {
		return nil, nil, fmt.Errorf("connection %q: %w", cc.ID, err)
	}

	closeTunnel := func() error { return nil }
	if cc.HasSSH() {
		tcfg, terr := sshConfig(cc, store)
		if terr != nil {
			return nil, nil, terr
		}
		host, port := cc.DialHostPort()
		localAddr, tun, terr := tunnel.Open(ctx, tcfg, host, port)
		if terr != nil {
			return nil, nil, terr
		}
		closeTunnel = tun.Close
		// Point the DSN at the local end of the tunnel.
		lh, lp := splitHostPort(localAddr)
		cc.Host, cc.Port = lh, lp
	}

	dsn, err := cc.DSN(password)
	if err != nil {
		_ = closeTunnel()
		return nil, nil, err
	}
	conn, err := driver.Open(ctx, cc.Driver, dsn)
	if err != nil {
		_ = closeTunnel()
		return nil, nil, err
	}
	closer := func() error {
		cerr := conn.Close()
		if terr := closeTunnel(); cerr == nil {
			cerr = terr
		}
		return cerr
	}
	return conn, closer, nil
}

// sshConfig builds the tunnel config, resolving the key passphrase (if any) via
// the secret store.
func sshConfig(cc config.ConnectionConfig, store secret.SecretStore) (tunnel.Config, error) {
	var passphrase string
	if cc.SSHKeyPassRef != "" {
		p, err := secret.Resolve(store, cc.SSHKeyPassRef)
		if err != nil {
			return tunnel.Config{}, fmt.Errorf("connection %q: ssh key passphrase: %w", cc.ID, err)
		}
		passphrase = p
	}
	return tunnel.Config{
		Host:            cc.SSHHost,
		Port:            cc.SSHPort,
		User:            cc.SSHUser,
		KeyFile:         cc.SSHKey,
		KeyPassphrase:   passphrase,
		KnownHostsFile:  cc.SSHKnownHosts,
		InsecureHostKey: cc.SSHInsecureHostKey,
	}, nil
}
