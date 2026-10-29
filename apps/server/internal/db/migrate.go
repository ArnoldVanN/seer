package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/seer-status/seer/apps/server/migrations/postgres"
)

const revisionsTable = "seer_schema_migrations"

type migrationFile struct {
	version string
	name    string
	body    []byte
}

// Migrate applies pending embedded migrations, each in its own transaction.
// It holds an advisory lock so replicas starting together don't race.
func (d *DB) Migrate(ctx context.Context) error {
	files, err := listMigrations(postgres.FS)
	if err != nil {
		return err
	}
	return d.WithLock(ctx, LockMigrate, func(ctx context.Context) error {
		if _, err := d.SQL.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+revisionsTable+` (
			version    TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
			return err
		}
		applied, err := d.appliedVersions(ctx)
		if err != nil {
			return err
		}
		n := 0
		for _, f := range files {
			if applied[f.version] {
				continue
			}
			log.Info().Str("name", f.name).Msg("applying migration")
			if err := d.applyOne(ctx, f); err != nil {
				return fmt.Errorf("apply %s: %w", f.name, err)
			}
			n++
		}
		log.Info().Int("applied", n).Int("total", len(files)).Msg("migrations complete")
		return nil
	})
}

func (d *DB) appliedVersions(ctx context.Context) (map[string]bool, error) {
	rows, err := d.SQL.QueryContext(ctx, "SELECT version FROM "+revisionsTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func (d *DB) applyOne(ctx context.Context, f migrationFile) error {
	tx, err := d.SQL.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, string(f.body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO "+revisionsTable+" (version, name) VALUES ($1, $2)", f.version, f.name); err != nil {
		return err
	}
	return tx.Commit()
}

func listMigrations(efs fs.FS) ([]migrationFile, error) {
	entries, err := fs.ReadDir(efs, ".")
	if err != nil {
		return nil, err
	}
	var out []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(efs, e.Name())
		if err != nil {
			return nil, err
		}
		version, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("migration %q has no version prefix", e.Name())
		}
		out = append(out, migrationFile{version: version, name: e.Name(), body: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
