// Package db opens the database and holds the few Postgres-specific bits that
// ent doesn't cover: advisory locks and LISTEN/NOTIFY.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"entgo.io/ent/dialect"

	"github.com/seer-status/seer/apps/server/ent"
)

type DB struct {
	Ent *ent.Client
	SQL *sql.DB
	DSN string
}

func Open(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("no database DSN: set storage.dsn or SEER_DB_DSN")
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	return &DB{Ent: ent.NewClient(ent.Driver(drv)), SQL: sqlDB, DSN: dsn}, nil
}

func (d *DB) Close() error { return d.Ent.Close() }

// Lock keys. Advisory locks are cluster-wide, so each singleton job gets one.
const (
	LockMigrate int64 = 0x5ee4_0001 + iota
	LockConfigSync
	LockRollups
	LockRetention
)

// TryRun runs fn only if this replica gets the advisory lock for key, and
// reports whether it did. The lock lives on one pinned connection and is
// released when fn returns (or when the connection dies with the replica).
func (d *DB) TryRun(ctx context.Context, key int64, fn func(ctx context.Context) error) (bool, error) {
	conn, err := d.SQL.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	var ok bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", key) //nolint:errcheck
	return true, fn(ctx)
}

// WithLock blocks until it holds the lock for key, then runs fn.
func (d *DB) WithLock(ctx context.Context, key int64, fn func(ctx context.Context) error) error {
	conn, err := d.SQL.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		return err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", key) //nolint:errcheck
	return fn(ctx)
}
