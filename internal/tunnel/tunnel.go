// Package tunnel forwards a local TCP port to a remote address through an SSH
// bastion, so s9l can reach a database that's only accessible from behind a
// jump host. It is a thin layer over golang.org/x/crypto/ssh (pure Go) and is
// independent of any driver — callers point their DSN at the returned local
// address.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Config describes how to reach the SSH bastion and verify its host key.
type Config struct {
	Host            string // bastion host (required)
	Port            int    // bastion port (default 22)
	User            string
	KeyFile         string // private key path; empty → use the SSH agent
	KeyPassphrase   string // passphrase for an encrypted KeyFile
	KnownHostsFile  string // host-key DB; empty → ~/.ssh/known_hosts
	InsecureHostKey bool   // skip host-key verification (INSECURE)
}

// Tunnel is a running SSH port-forward. Close it to stop forwarding and drop the
// SSH connection.
type Tunnel struct {
	listener net.Listener
	client   *ssh.Client
}

// Open dials the bastion, starts a local listener, and forwards new local
// connections to remoteHost:remotePort over SSH. It returns the local address
// to connect to and the Tunnel (call Close when done).
func Open(ctx context.Context, cfg Config, remoteHost string, remotePort int) (localAddr string, t *Tunnel, err error) {
	auth, err := authMethods(cfg)
	if err != nil {
		return "", nil, err
	}
	hostKey, err := hostKeyCallback(cfg)
	if err != nil {
		return "", nil, err
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         15 * time.Second,
	}

	bastion := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	d := net.Dialer{Timeout: clientCfg.Timeout}
	netConn, err := d.DialContext(ctx, "tcp", bastion)
	if err != nil {
		return "", nil, fmt.Errorf("tunnel: dial bastion %s: %w", bastion, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, bastion, clientCfg)
	if err != nil {
		_ = netConn.Close()
		return "", nil, fmt.Errorf("tunnel: ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return "", nil, fmt.Errorf("tunnel: local listen: %w", err)
	}
	remote := net.JoinHostPort(remoteHost, strconv.Itoa(remotePort))
	tun := &Tunnel{listener: ln, client: client}
	go tun.serve(remote)
	return ln.Addr().String(), tun, nil
}

// serve accepts local connections and forwards each to the remote address over
// the SSH client until the listener is closed.
func (t *Tunnel) serve(remote string) {
	for {
		local, err := t.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go t.forward(local, remote)
	}
}

func (t *Tunnel) forward(local net.Conn, remote string) {
	defer func() { _ = local.Close() }()
	rc, err := t.client.Dial("tcp", remote)
	if err != nil {
		return
	}
	defer func() { _ = rc.Close() }()
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) { _, _ = io.Copy(dst, src); done <- struct{}{} }
	go cp(rc, local)
	go cp(local, rc)
	<-done // either side closed
}

// Close stops the listener and closes the SSH connection.
func (t *Tunnel) Close() error {
	err := t.listener.Close()
	if cerr := t.client.Close(); err == nil {
		err = cerr
	}
	return err
}

// authMethods builds the SSH auth methods: the configured private key, otherwise
// the running SSH agent.
func authMethods(cfg Config) ([]ssh.AuthMethod, error) {
	if cfg.KeyFile != "" {
		key, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("tunnel: read key %s: %w", cfg.KeyFile, err)
		}
		var signer ssh.Signer
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("tunnel: parse key %s: %w", cfg.KeyFile, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(conn).Signers)}, nil
		}
	}
	return nil, errors.New("tunnel: no SSH auth available (set ssh_key or run an SSH agent)")
}

// hostKeyCallback verifies the bastion's host key against known_hosts, unless
// InsecureHostKey is set.
func hostKeyCallback(cfg Config) (ssh.HostKeyCallback, error) {
	if cfg.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit opt-out
	}
	path := cfg.KnownHostsFile
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("tunnel: locate known_hosts: %w", err)
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("tunnel: known_hosts %s: %w (add the host with ssh, or set ssh_insecure_host_key)", path, err)
	}
	return cb, nil
}
