package mysql_test

import (
	"context"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/driver/drivertest"

	_ "github.com/YangXplorer/s9l/internal/driver/mysql"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// startMySQL spins up a throwaway MySQL container and returns its DSN. These are
// integration tests: they need Docker and are skipped under -short.
func startMySQL(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skip integration test (needs Docker); run without -short")
	}
	ctx := context.Background()
	// Rely on the module's built-in readiness wait (it waits until MySQL
	// actually accepts connections — the port opening alone is not enough).
	ctr, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("app"),
		tcmysql.WithUsername("dev"),
		tcmysql.WithPassword("secret"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

func TestConformance(t *testing.T) {
	dsn := startMySQL(t)
	drivertest.RunConformance(t, func(ctx context.Context) (driver.Conn, error) {
		return driver.Open(ctx, "mysql", dsn)
	})
}

func TestMetadata(t *testing.T) {
	dsn := startMySQL(t)
	ctx := context.Background()
	conn, err := driver.Open(ctx, "mysql", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id INT PRIMARY KEY, label VARCHAR(64) NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	md, ok := conn.(driver.Metadata)
	if !ok {
		t.Fatal("mysql conn should implement driver.Metadata")
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
