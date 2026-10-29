// Package scheduler runs checks for the endpoints this worker owns.
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/ent/endpoint"
	"github.com/seer-status/seer/apps/server/ent/result"
	entworker "github.com/seer-status/seer/apps/server/ent/worker"
	"github.com/seer-status/seer/apps/server/internal/checker"
	"github.com/seer-status/seer/apps/server/internal/cluster"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
)

const (
	HeartbeatInterval = 5 * time.Second
	// A worker missing heartbeats for this long is treated as dead and its
	// endpoints move to the others.
	LiveTimeout = 15 * time.Second
)

type Worker struct {
	ID       string
	DB       *db.DB
	Recorder *Recorder

	mu      sync.Mutex
	running map[string]*runner

	// crash skips deregistering on shutdown; tests use it to simulate a
	// worker dying.
	crash bool
}

type runner struct {
	cancel context.CancelFunc
	done   chan struct{}
	hash   [32]byte
	group  string
}

func NewWorker(d *db.DB, rec *Recorder) *Worker {
	return &Worker{ID: uuid.NewString(), DB: d, Recorder: rec, running: map[string]*runner{}}
}

// Run heartbeats and reconciles owned endpoints until ctx is done, then
// deregisters so the others take over right away instead of after LiveTimeout.
func (w *Worker) Run(ctx context.Context) error {
	host, _ := os.Hostname()
	now := time.Now()
	if err := w.DB.Ent.Worker.Create().
		SetID(w.ID).SetHostname(host).SetStartedAt(now).SetLastHeartbeat(now).
		Exec(ctx); err != nil {
		return err
	}
	log.Info().Str("worker", w.ID).Msg("worker registered")

	t := time.NewTicker(HeartbeatInterval)
	defer t.Stop()
	for {
		if err := w.tick(ctx); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("worker tick")
		}
		select {
		case <-ctx.Done():
			w.stopAll()
			if w.crash {
				return nil
			}
			_, err := w.DB.Ent.Worker.Delete().Where(entworker.ID(w.ID)).Exec(context.WithoutCancel(ctx))
			log.Info().Err(err).Msg("worker deregistered")
			return nil
		case <-t.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	now := time.Now()
	// Upsert, so a worker whose row was cleaned up after a long DB outage
	// comes back.
	host, _ := os.Hostname()
	if err := w.DB.Ent.Worker.Create().
		SetID(w.ID).SetHostname(host).SetStartedAt(now).SetLastHeartbeat(now).
		OnConflictColumns(entworker.FieldID).
		UpdateLastHeartbeat().
		Exec(ctx); err != nil {
		return err
	}

	live, err := w.DB.Ent.Worker.Query().
		Where(entworker.LastHeartbeatGT(now.Add(-LiveTimeout))).
		IDs(ctx)
	if err != nil {
		return err
	}
	liveWorkers.Set(float64(len(live)))

	eps, err := w.DB.Ent.Endpoint.Query().Where(endpoint.Enabled(true)).All(ctx)
	if err != nil {
		return err
	}

	owned := map[string]*ent.Endpoint{}
	for _, e := range eps {
		if cluster.Owner(e.Key, live) == w.ID {
			owned[e.Key] = e
		}
	}
	w.reconcile(ctx, owned)
	return nil
}

func (w *Worker) reconcile(ctx context.Context, owned map[string]*ent.Endpoint) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for key, r := range w.running {
		e, ok := owned[key]
		if ok && sha256.Sum256(e.Spec) == r.hash {
			continue
		}
		r.cancel()
		<-r.done
		delete(w.running, key)
		if !ok {
			forgetMetrics(key, r.group)
			log.Debug().Str("endpoint", key).Msg("released endpoint")
		}
	}
	for key, e := range owned {
		if _, ok := w.running[key]; ok {
			continue
		}
		var spec config.Endpoint
		if err := json.Unmarshal(e.Spec, &spec); err != nil {
			log.Error().Err(err).Str("endpoint", key).Msg("bad endpoint spec")
			continue
		}
		rctx, cancel := context.WithCancel(ctx)
		r := &runner{cancel: cancel, done: make(chan struct{}), hash: sha256.Sum256(e.Spec), group: spec.Group}
		w.running[key] = r
		go func() {
			defer close(r.done)
			w.runEndpoint(rctx, e.ID, &spec)
		}()
		log.Debug().Str("endpoint", key).Msg("acquired endpoint")
	}
	ownedEndpoints.Set(float64(len(w.running)))
}

func (w *Worker) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for key, r := range w.running {
		r.cancel()
		<-r.done
		delete(w.running, key)
	}
}

// runEndpoint checks once per slot. Slots are aligned to wall-clock time plus
// a per-key offset, so whichever worker owns the endpoint checks at the same
// moments and the (endpoint, slot) unique key catches overlap during handoff.
func (w *Worker) runEndpoint(ctx context.Context, endpointID int, spec *config.Endpoint) {
	interval := spec.Interval.D().Milliseconds()
	offset := cluster.Offset(spec.Key, interval)
	slotOf := func(t time.Time) int64 { return (t.UnixMilli() - offset) / interval }
	slotStart := func(s int64) time.Time { return time.UnixMilli(s*interval + offset) }

	// Fill the current slot right away unless someone already did, so a new
	// owner (or a fresh endpoint with a long interval) doesn't wait a whole
	// interval.
	cur := slotOf(time.Now())
	exists, err := w.DB.Ent.Result.Query().
		Where(result.EndpointID(endpointID), result.Slot(cur)).Exist(ctx)
	if err == nil && !exists {
		w.check(ctx, endpointID, spec, cur)
	}

	for {
		nextSlot := slotOf(time.Now()) + 1
		timer := time.NewTimer(time.Until(slotStart(nextSlot)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		w.check(ctx, endpointID, spec, nextSlot)
	}
}

func (w *Worker) check(ctx context.Context, endpointID int, spec *config.Endpoint, slot int64) {
	out := checker.Run(ctx, spec)
	if ctx.Err() != nil {
		return // lost ownership mid-check; the new owner covers this slot
	}
	stored, err := w.Recorder.Record(ctx, endpointID, spec, slot, out)
	if err != nil {
		log.Error().Err(err).Str("endpoint", spec.Key).Msg("record result")
		return
	}
	if !stored {
		return
	}
	up := 0.0
	if out.Success {
		up = 1
	}
	endpointUp.WithLabelValues(spec.Key, spec.Group).Set(up)
	endpointResponse.WithLabelValues(spec.Key, spec.Group).Set(out.ResponseTime.Seconds())
	checksTotal.WithLabelValues(spec.Key, spec.Group, strconv.FormatBool(out.Success)).Inc()
}
