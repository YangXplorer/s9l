package driver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
)

type failDriver struct{}

func (failDriver) Name() string { return "failtest" }
func (failDriver) Open(context.Context, string) (driver.Conn, error) {
	return nil, errors.New("boom")
}

func TestOpenWrapsConnectionError(t *testing.T) {
	driver.Register(failDriver{})
	_, err := driver.Open(context.Background(), "failtest", "dsn")
	if err == nil {
		t.Fatal("expected error")
	}
	// The error must carry the driver name so users can tell which connection failed.
	if !strings.Contains(err.Error(), "failtest") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q should contain driver name and cause", err)
	}
}

func TestGetUnknownDriver(t *testing.T) {
	if _, err := driver.Get("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown driver")
	}
}
