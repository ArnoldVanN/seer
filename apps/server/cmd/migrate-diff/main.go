// Dev-time tool: generate a new versioned migration by diffing the ent schema
// against an empty dev database. Use scripts/migrate-diff.sh, which manages a
// throwaway Postgres for it.
//
//	ATLAS_DEV_URL=postgres://user:pass@localhost:15432/dev?sslmode=disable \
//	    go run ./cmd/migrate-diff <migration_name>
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	atlas "ariga.io/atlas/sql/migrate"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"

	"github.com/seer-status/seer/apps/server/ent/migrate"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: migrate-diff <migration_name>")
	}
	devURL := os.Getenv("ATLAS_DEV_URL")
	if devURL == "" {
		log.Fatal("ATLAS_DEV_URL is required (e.g. postgres://user:pass@localhost:15432/dev?sslmode=disable)")
	}

	dir, err := atlas.NewLocalDir("migrations/postgres")
	if err != nil {
		log.Fatalf("open migrations dir: %v", err)
	}

	opts := []schema.MigrateOption{
		schema.WithDir(dir),
		schema.WithMigrationMode(schema.ModeReplay),
		schema.WithDialect(dialect.Postgres),
		schema.WithFormatter(atlas.DefaultFormatter),
		// Off by default, which leaves a removed field's column behind and
		// still NOT NULL, so every insert after it fails.
		schema.WithDropColumn(true),
		schema.WithDropIndex(true),
	}

	if err := migrate.NamedDiff(context.Background(), devURL, os.Args[1], opts...); err != nil {
		log.Fatalf("named diff: %v", err)
	}
	fmt.Println("migration generated")
}
