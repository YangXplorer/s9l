package tunnel_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/tunnel"

	"golang.org/x/crypto/ssh"
)

// TestForwardsThroughSSH stands up an in-process SSH server and a target TCP
// echo server, opens a tunnel through the SSH server to the target, and checks
// that bytes flow end to end — exercising the real x/crypto/ssh forwarding
// without Docker.
func TestForwardsThroughSSH(t *testing.T) {
	// Target service the tunnel should reach: an echo server.
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = target.Close() }()
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	_, targetPortStr, _ := net.SplitHostPort(target.Addr().String())

	// A client key the in-process SSH server will accept.
	clientKey, clientSigner := genKey(t)
	keyFile := writeKey(t, clientKey)
	srvAddr, hostPub := startSSHServer(t, clientSigner.PublicKey())

	// known_hosts trusting the server's host key.
	srvHost, srvPort, _ := net.SplitHostPort(srvAddr)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	line := knownHostsLine(srvHost, srvPort, hostPub)
	if err := os.WriteFile(kh, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	portNum := atoiPort(srvPort)
	local, tun, err := tunnel.Open(context.Background(), tunnel.Config{
		Host: srvHost, Port: portNum, User: "tester",
		KeyFile: keyFile, KnownHostsFile: kh,
	}, "127.0.0.1", atoiPort(targetPortStr))
	if err != nil {
		t.Fatalf("tunnel.Open: %v", err)
	}
	defer func() { _ = tun.Close() }()

	// Talk to the local end; bytes must echo back from the target via SSH.
	conn, err := net.DialTimeout("tcp", local, 5*time.Second)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer func() { _ = conn.Close() }()
	want := []byte("hello-through-ssh")
	if _, err := conn.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("echo = %q, want %q", got, want)
	}
}

func TestInsecureHostKeySkipsVerification(t *testing.T) {
	target, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = target.Close() }()
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	_, tp, _ := net.SplitHostPort(target.Addr().String())

	clientKey, clientSigner := genKey(t)
	keyFile := writeKey(t, clientKey)
	srvAddr, _ := startSSHServer(t, clientSigner.PublicKey())
	srvHost, srvPort, _ := net.SplitHostPort(srvAddr)

	// No known_hosts file; InsecureHostKey must let it connect anyway.
	_, tun, err := tunnel.Open(context.Background(), tunnel.Config{
		Host: srvHost, Port: atoiPort(srvPort), User: "tester",
		KeyFile: keyFile, InsecureHostKey: true,
	}, "127.0.0.1", atoiPort(tp))
	if err != nil {
		t.Fatalf("tunnel.Open insecure: %v", err)
	}
	_ = tun.Close()
}

// --- helpers ---

func genKey(t *testing.T) (*rsa.PrivateKey, ssh.Signer) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, signer
}

func writeKey(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	pem, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(path, pemBytes(t, pem), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// startSSHServer runs an SSH server accepting clientPub via public-key auth and
// supporting direct-tcpip (port forwarding). Returns its address and host key.
func startSSHServer(t *testing.T, clientPub ssh.PublicKey) (addr string, hostKey ssh.PublicKey) {
	t.Helper()
	_, hostSigner := genKey(t)
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) == string(clientPub.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, errUnauthorized
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSSHConn(c, cfg)
		}
	}()
	return ln.Addr().String(), hostSigner.PublicKey()
}

func serveSSHConn(c net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		_ = c.Close()
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "direct-tcpip" {
			_ = nc.Reject(ssh.UnknownChannelType, "only direct-tcpip")
			continue
		}
		go handleDirectTCPIP(nc)
	}
}

// directTCPIP is the payload of a direct-tcpip channel open request.
type directTCPIP struct {
	DestHost string
	DestPort uint32
	SrcHost  string
	SrcPort  uint32
}

func handleDirectTCPIP(nc ssh.NewChannel) {
	var d directTCPIP
	if err := ssh.Unmarshal(nc.ExtraData(), &d); err != nil {
		_ = nc.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	remote, err := net.Dial("tcp", net.JoinHostPort(d.DestHost, itoa(d.DestPort)))
	if err != nil {
		_ = nc.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, err := nc.Accept()
	if err != nil {
		_ = remote.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	go func() { _, _ = io.Copy(ch, remote); _ = ch.Close() }()
	go func() { _, _ = io.Copy(remote, ch); _ = remote.Close() }()
}
