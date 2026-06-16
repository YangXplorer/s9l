package sqlite_test

import (
	"context"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/driver/drivertest"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlite"
)

// TestConformance runs the shared driver conformance suite against an
// in-memory SQLite database — fast, no CGO, no Docker.
func TestConformance(t *testing.T) {
	drivertest.RunConformance(t, func(ctx context.Context) (driver.Conn, error) {
		return driver.Open(ctx, "sqlite", ":memory:")
	})
}

func TestUnknownDriver(t *testing.T) {
	if _, err := driver.Get("nope"); err == nil {
		t.Fatal("expected error for unknown driver")
	}
}
