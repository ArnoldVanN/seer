// Package testdb gives integration tests a fresh, migrated database. Tests
// using it skip unless SEER_TEST_DSN points at a Postgres they may create
// databases on (scripts/test-integration.sh starts one).
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/seer-status/seer/apps/server/internal/db"
)

func New(t *testing.T) *db.DB {
	t.Helper()
	admin := os.Getenv("SEER_TEST_DSN")
	if admin == "" {
		t.Skip("SEER_TEST_DSN not set")
	}
	ctx := context.Background()

	adminDB, err := sql.Open("pgx", admin)
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Close()
	name := fmt.Sprintf("seer_test_%d", rand.Uint32())
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	d, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		c, err := sql.Open("pgx", admin)
		if err == nil {
			c.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)") //nolint:errcheck
			c.Close()
		}
	})
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return d
}
