package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/driver/drivertest"

	_ "github.com/YangXplorer/s9l/internal/driver/postgres"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres spins up a throwaway PostgreSQL container and returns its DSN.
// These are integration tests: they need Docker and are skipped under -short.
func startPostgres(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skip integration test (needs Docker); run without -short")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("dev"),
		tcpostgres.WithPassword("secret"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

func TestConformance(t *testing.T) {
	dsn := startPostgres(t)
	drivertest.RunConformance(t, func(ctx context.Context) (driver.Conn, error) {
		return driver.Open(ctx, "postgres", dsn)
	})
}

func TestMetadata(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	conn, err := driver.Open(ctx, "postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id serial PRIMARY KEY, label text NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	md, ok := conn.(driver.Metadata)
	if !ok {
		t.Fatal("postgres conn should implement driver.Metadata")
	}

	tables, err := md.Tables(ctx)
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if !containsFirstCol(t, tables, "widgets") {
		t.Error("Tables should include widgets")
	}

	cols, err := md.Columns(ctx, "widgets")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if !containsFirstCol(t, cols, "id") {
		t.Error("Columns(widgets) should include id")
	}

	dbs, err := md.Databases(ctx)
	if err != nil {
		t.Fatalf("Databases: %v", err)
	}
	if !containsFirstCol(t, dbs, "app") {
		t.Error("Databases should include app")
	}
}

func containsFirstCol(t *testing.T, rows driver.Rows, want string) bool {
	t.Helper()
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatalf("values: %v", err)
		}
		if s, ok := vals[0].(string); ok && s == want {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return false
}
