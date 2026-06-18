package sqlserver_test

import (
	"context"
	"testing"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/driver/drivertest"

	_ "github.com/YangXplorer/s9l/internal/driver/sqlserver"

	tcmssql "github.com/testcontainers/testcontainers-go/modules/mssql"
)

// startSQLServer spins up a throwaway SQL Server container and returns its DSN.
// These are integration tests: they need Docker and are skipped under -short.
// The image is large (>1GB) and slow to start.
func startSQLServer(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skip integration test (needs Docker); run without -short")
	}
	ctx := context.Background()
	ctr, err := tcmssql.Run(ctx, "mcr.microsoft.com/mssql/server:2022-latest",
		tcmssql.WithAcceptEULA(),
		tcmssql.WithPassword("yourStrong(!)Password"),
	)
	if err != nil {
		t.Fatalf("start sqlserver container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

func TestConformance(t *testing.T) {
	dsn := startSQLServer(t)
	drivertest.RunConformance(t, func(ctx context.Context) (driver.Conn, error) {
		return driver.Open(ctx, "sqlserver", dsn)
	})
}

func TestMetadata(t *testing.T) {
	dsn := startSQLServer(t)
	ctx := context.Background()
	conn, err := driver.Open(ctx, "sqlserver", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id INT PRIMARY KEY, label NVARCHAR(64) NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	md, ok := conn.(driver.Metadata)
	if !ok {
		t.Fatal("sqlserver conn should implement driver.Metadata")
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
	if !containsFirstCol(t, dbs, "master") {
		t.Error("Databases should include master")
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
