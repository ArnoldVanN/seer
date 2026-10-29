package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
)

// Buckets are aligned to this origin, so days are UTC days.
const origin = `TIMESTAMPTZ '2000-01-01 00:00:00+00'`

// Response times (rt_*) cover successful checks only: a failure's time is how
// long it took to fail, which says nothing about how fast the endpoint is.
// They're 0 in buckets without successes.
//
// Each tier is rebuilt from the tier below over a lookback window. Rebuilding
// whole buckets with an upsert is idempotent, so a new leader can redo work
// safely. The lookback also covers a leader gap of that length.
var rollupSteps = []struct {
	name     string
	query    string
	lookback string
}{
	{
		name:     "5m",
		lookback: "2 hours",
		query: `
INSERT INTO rollups (endpoint_id, resolution, bucket, checks, successes, rt_sum_ms, rt_min_ms, rt_max_ms)
SELECT endpoint_id, '5m', date_bin('5 minutes', checked_at, ` + origin + `) AS b,
       count(*), count(*) FILTER (WHERE success),
       coalesce(sum(response_time_ms) FILTER (WHERE success), 0),
       coalesce(min(response_time_ms) FILTER (WHERE success), 0),
       coalesce(max(response_time_ms) FILTER (WHERE success), 0)
FROM results
WHERE checked_at >= date_bin('5 minutes', now() - $1::interval, ` + origin + `)
GROUP BY endpoint_id, b
ON CONFLICT (endpoint_id, resolution, bucket) DO UPDATE SET
  checks = EXCLUDED.checks, successes = EXCLUDED.successes, rt_sum_ms = EXCLUDED.rt_sum_ms,
  rt_min_ms = EXCLUDED.rt_min_ms, rt_max_ms = EXCLUDED.rt_max_ms`,
	},
	{
		name:     "1h",
		lookback: "6 hours",
		query:    rollupFrom("1h", "5m", "1 hour"),
	},
	{
		name:     "1d",
		lookback: "3 days",
		query:    rollupFrom("1d", "1h", "1 day"),
	},
}

func rollupFrom(to, from, stride string) string {
	return `
INSERT INTO rollups (endpoint_id, resolution, bucket, checks, successes, rt_sum_ms, rt_min_ms, rt_max_ms)
SELECT endpoint_id, '` + to + `', date_bin('` + stride + `', bucket, ` + origin + `) AS b,
       sum(checks), sum(successes), sum(rt_sum_ms),
       coalesce(min(rt_min_ms) FILTER (WHERE successes > 0), 0), max(rt_max_ms)
FROM rollups
WHERE resolution = '` + from + `' AND bucket >= date_bin('` + stride + `', now() - $1::interval, ` + origin + `)
GROUP BY endpoint_id, b
ON CONFLICT (endpoint_id, resolution, bucket) DO UPDATE SET
  checks = EXCLUDED.checks, successes = EXCLUDED.successes, rt_sum_ms = EXCLUDED.rt_sum_ms,
  rt_min_ms = EXCLUDED.rt_min_ms, rt_max_ms = EXCLUDED.rt_max_ms`
}

func Rollup(ctx context.Context, d *db.DB) error {
	return RollupWindow(ctx, d, nil)
}

// RollupWindow is Rollup with the lookback of some tiers overridden (by name:
// "5m", "1h", "1d"), for rebuilding older history such as seeded data.
func RollupWindow(ctx context.Context, d *db.DB, lookback map[string]string) error {
	for _, s := range rollupSteps {
		lb := s.lookback
		if v, ok := lookback[s.name]; ok {
			lb = v
		}
		if _, err := d.SQL.ExecContext(ctx, s.query, lb); err != nil {
			return fmt.Errorf("rollup %s: %w", s.name, err)
		}
	}
	return nil
}

const deleteBatch = 10000

// Retention deletes expired rows in batches so it never holds long locks.
func Retention(ctx context.Context, d *db.DB, r config.Retention, liveTimeout time.Duration) error {
	now := time.Now()
	if r.Raw > 0 {
		if err := deleteBatched(ctx, d,
			`DELETE FROM results WHERE id IN (SELECT id FROM results WHERE checked_at < $1 LIMIT `+fmt.Sprint(deleteBatch)+`)`,
			now.Add(-r.Raw.D())); err != nil {
			return fmt.Errorf("results: %w", err)
		}
	}
	for res, keep := range map[string]config.Duration{"5m": r.Rollup5m, "1h": r.Rollup1h, "1d": r.Rollup1d} {
		if keep <= 0 {
			continue
		}
		if err := deleteBatched(ctx, d,
			`DELETE FROM rollups WHERE id IN (SELECT id FROM rollups WHERE resolution = '`+res+`' AND bucket < $1 LIMIT `+fmt.Sprint(deleteBatch)+`)`,
			now.Add(-keep.D())); err != nil {
			return fmt.Errorf("rollups %s: %w", res, err)
		}
	}
	// Rows of workers that died without deregistering.
	_, err := d.SQL.ExecContext(ctx, `DELETE FROM workers WHERE last_heartbeat < $1`, now.Add(-10*liveTimeout))
	return err
}

func deleteBatched(ctx context.Context, d *db.DB, q string, before time.Time) error {
	total := int64(0)
	for {
		res, err := d.SQL.ExecContext(ctx, q, before)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < deleteBatch {
			break
		}
	}
	if total > 0 {
		log.Info().Int64("rows", total).Msg("retention deleted rows")
	}
	return nil
}

// RunPeriodic runs the rollup and retention jobs on whichever replica gets
// each lock first.
func RunPeriodic(ctx context.Context, d *db.DB, r config.Retention, liveTimeout time.Duration) {
	rollupTick := time.NewTicker(time.Minute)
	retentionTick := time.NewTicker(10 * time.Minute)
	defer rollupTick.Stop()
	defer retentionTick.Stop()

	run := func(name string, key int64, fn func(context.Context) error) {
		jctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		ran, err := d.TryRun(jctx, key, fn)
		if err != nil && ctx.Err() == nil {
			log.Error().Err(err).Str("job", name).Msg("job failed")
		} else if ran {
			log.Debug().Str("job", name).Msg("job ran")
		}
	}
	rollup := func(ctx context.Context) error { return Rollup(ctx, d) }
	retention := func(ctx context.Context) error { return Retention(ctx, d, r, liveTimeout) }

	run("rollup", db.LockRollups, rollup)
	for {
		select {
		case <-ctx.Done():
			return
		case <-rollupTick.C:
			run("rollup", db.LockRollups, rollup)
		case <-retentionTick.C:
			run("retention", db.LockRetention, retention)
		}
	}
}
