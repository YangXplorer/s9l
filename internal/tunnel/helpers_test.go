package tunnel_test

import (
	"encoding/pem"
	"errors"
	"net"
	"strconv"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var errUnauthorized = errors.New("unauthorized")

func pemBytes(t *testing.T, block *pem.Block) []byte {
	t.Helper()
	return pem.EncodeToMemory(block)
}

func knownHostsLine(host, port string, key ssh.PublicKey) string {
	addr := knownhosts.Normalize(net.JoinHostPort(host, port))
	return knownhosts.Line([]string{addr}, key) + "\n"
}

func atoiPort(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func itoa(u uint32) string { return strconv.Itoa(int(u)) }
