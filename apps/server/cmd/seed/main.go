// Dev-time tool: backfills a year of fake history for the demo-api endpoint
// so every chart range has something to show. Run by scripts/dev-db-up.sh.
//
//	go run ./cmd/seed [-config ../../seer.example.yaml] [-force]
//
// SEER_DB_DSN defaults to the dev database from scripts/dev-db-up.sh.
//
// It skips if the endpoint already has history, unless -force, which deletes
// and regenerates it.
package main

import (
	"context"
	"flag"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"time"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/ent/endpoint"
	"github.com/seer-status/seer/apps/server/ent/result"
	"github.com/seer-status/seer/apps/server/ent/rollup"
	"github.com/seer-status/seer/apps/server/internal/cluster"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
	"github.com/seer-status/seer/apps/server/internal/jobs"
)

const (
	key     = "demo-api"
	rawDays = 14
	days    = 365
)

// incident is a stretch where checks fail, or only get slow when slowMs is set.
type incident struct {
	ago    time.Duration
	length time.Duration
	slowMs int64
}

var incidents = []incident{
	{ago: 2 * 24 * time.Hour, length: 25 * time.Minute},
	{ago: 9*24*time.Hour + 5*time.Hour, length: 3 * time.Hour, slowMs: 900},
	{ago: 40 * 24 * time.Hour, length: 2 * time.Hour},
	{ago: 75 * 24 * time.Hour, length: 9 * time.Hour, slowMs: 600},
	{ago: 160 * 24 * time.Hour, length: 5 * time.Hour},
	{ago: 300 * 24 * time.Hour, length: 50 * time.Minute},
}

func main() {
	configPath := flag.String("config", "../../seer.example.yaml", "config file the demo endpoint is defined in")
	force := flag.Bool("force", false, "delete and regenerate existing demo history")
	flag.Parse()
	ctx := context.Background()

	// Dev-only tool, so default to the scripts/dev-db-up.sh database.
	if os.Getenv("SEER_DB_DSN") == "" {
		os.Setenv("SEER_DB_DSN", "postgres://seer:seer@localhost:5433/seer?sslmode=disable")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	d, err := db.Open(ctx, cfg.Storage.DSN)
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "seed"); err != nil {
		log.Fatal(err)
	}

	ep, err := d.Ent.Endpoint.Query().Where(endpoint.Key(key)).Only(ctx)
	if ent.IsNotFound(err) {
		log.Printf("no %q endpoint in %s; nothing to seed", key, *configPath)
		return
	}
	if err != nil {
		log.Fatal(err)
	}

	start := time.Now().Add(-days * 24 * time.Hour)
	seeded, err := d.Ent.Rollup.Query().Where(rollup.EndpointID(ep.ID), rollup.BucketLT(start.Add(24*time.Hour))).Exist(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if seeded && !*force {
		log.Printf("%s already has history; skipping (use -force to regenerate)", key)
		return
	}
	if _, err := d.Ent.Result.Delete().Where(result.EndpointID(ep.ID)).Exec(ctx); err != nil {
		log.Fatal(err)
	}
	if _, err := d.Ent.Rollup.Delete().Where(rollup.EndpointID(ep.ID)).Exec(ctx); err != nil {
		log.Fatal(err)
	}

	rng := rand.New(rand.NewPCG(42, 7))
	now := time.Now()
	rawFrom := now.Add(-rawDays * 24 * time.Hour)

	// Raw results, one per interval, in the same slots a worker would use.
	interval := time.Minute
	offset := cluster.Offset(key, interval.Milliseconds())
	var batch []*ent.ResultCreate
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := d.Ent.Result.CreateBulk(batch...).Exec(ctx); err != nil {
			log.Fatal(err)
		}
		batch = batch[:0]
	}
	n := 0
	for slot := (rawFrom.UnixMilli() - offset) / interval.Milliseconds(); ; slot++ {
		at := time.UnixMilli(slot*interval.Milliseconds() + offset)
		// Leave the current slot for the live worker.
		if at.After(now.Add(-interval)) {
			break
		}
		ok, rt := sample(rng, at, now)
		c := d.Ent.Result.Create().
			SetEndpointID(ep.ID).SetSlot(slot).SetCheckedAt(at).
			SetSuccess(ok).SetResponseTimeMs(rt)
		if !ok {
			c.SetError("Get \"http://demo-api/healthz\": dial tcp 10.0.0.12:443: connect: connection refused")
		}
		batch = append(batch, c)
		n++
		if len(batch) == 1000 {
			flush()
		}
	}
	flush()

	// Older history goes straight into hourly rollups, as if the raw rows had
	// already expired.
	var hours []*ent.RollupCreate
	for h := start.Truncate(time.Hour); h.Before(rawFrom.Truncate(time.Hour)); h = h.Add(time.Hour) {
		var checks, successes, sum, lo, hi int64
		lo = math.MaxInt64
		for m := range 60 {
			ok, rt := sample(rng, h.Add(time.Duration(m)*time.Minute), now)
			checks++
			// Like jobs.Rollup, response times count successful checks only.
			if !ok {
				continue
			}
			successes++
			sum += rt
			lo, hi = min(lo, rt), max(hi, rt)
		}
		if successes == 0 {
			lo = 0
		}
		hours = append(hours, d.Ent.Rollup.Create().
			SetEndpointID(ep.ID).SetResolution(rollup.Resolution1h).SetBucket(h).
			SetChecks(checks).SetSuccesses(successes).
			SetRtSumMs(sum).SetRtMinMs(lo).SetRtMaxMs(hi))
		if len(hours) == 1000 {
			if err := d.Ent.Rollup.CreateBulk(hours...).Exec(ctx); err != nil {
				log.Fatal(err)
			}
			hours = hours[:0]
		}
	}
	if len(hours) > 0 {
		if err := d.Ent.Rollup.CreateBulk(hours...).Exec(ctx); err != nil {
			log.Fatal(err)
		}
	}

	// Build 5m and 1h from the raw rows, and 1d over the whole year.
	window := (rawDays + 1) * 24 * time.Hour
	if err := jobs.RollupWindow(ctx, d, map[string]string{
		"5m": window.String(),
		"1h": window.String(),
		"1d": ((days + 2) * 24 * time.Hour).String(),
	}); err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded %s: %d raw results, %d days of history", key, n, days)
}

// sample makes up one check: a daily latency cycle with noise, plus the
// incidents above.
func sample(rng *rand.Rand, at, now time.Time) (bool, int64) {
	hour := float64(at.UTC().Hour()) + float64(at.UTC().Minute())/60
	rt := 120 + 45*math.Sin((hour-9)/24*2*math.Pi) + rng.NormFloat64()*15
	if rng.Float64() < 0.01 {
		rt += 200 + rng.Float64()*300 // occasional spike
	}
	for _, in := range incidents {
		from := now.Add(-in.ago)
		if at.Before(from) || !at.Before(from.Add(in.length)) {
			continue
		}
		if in.slowMs > 0 {
			rt += float64(in.slowMs) * (0.7 + 0.6*rng.Float64())
			continue
		}
		return false, int64(5000 + rng.IntN(1000))
	}
	// A rare one-off failure, roughly one a month, so the incidents above are
	// what stands out.
	if rng.Float64() < 0.00002 {
		return false, 5000
	}
	return true, int64(max(20, rt))
}
