//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PAYGATE_TEST_DB_URL")
	if dsn == "" {
		dsn = "postgres://paygate:paygate@localhost:5432/paygate?sslmode=disable"
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration: db unavailable: %v", err)
	}
	if err := db.Ping(ctx); err != nil {
		t.Skipf("skip integration: db ping failed: %v", err)
	}
	applyMigrations(t, db)
	return db
}

func applyMigrations(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := db.Exec(context.Background(), string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", f, err)
		}
	}
}
