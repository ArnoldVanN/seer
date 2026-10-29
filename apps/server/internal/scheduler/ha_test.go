package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seer-status/seer/apps/server/ent/endpoint"
	"github.com/seer-status/seer/apps/server/ent/result"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
	"github.com/seer-status/seer/apps/server/internal/jobs"
	"github.com/seer-status/seer/apps/server/internal/notify"
	"github.com/seer-status/seer/apps/server/internal/testdb"
)

type recordingNotifier struct {
	mu     sync.Mutex
	events map[string][]notify.Kind
}

func (r *recordingNotifier) Notify(_ context.Context, ev notify.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[ev.EndpointKey] = append(r.events[ev.EndpointKey], ev.Kind)
	return nil
}

func (r *recordingNotifier) snapshot() map[string][]notify.Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string][]notify.Kind{}
	for k, v := range r.events {
		out[k] = append([]notify.Kind(nil), v...)
	}
	return out
}

func startWorker(t *testing.T, d *db.DB, n *recordingNotifier) (*Worker, context.CancelFunc, chan struct{}) {
	disp := notify.NewDispatcher()
	disp.Register("test", n)
	w := NewWorker(d, &Recorder{DB: d, Notify: disp})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := w.Run(ctx); err != nil {
			t.Errorf("worker: %v", err)
		}
	}()
	return w, cancel, done
}

// TestHA runs three workers against the same endpoints and checks the
// guarantees the design rests on: one result per slot, failover when a worker
// dies, and exactly one notification per transition.
func TestHA(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	d := testdb.New(t)
	ctx := context.Background()

	var flapping atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/flap" && flapping.Load() {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	const n = 30
	cfg := &config.Config{}
	for i := range n {
		e := config.Endpoint{
			Key: fmt.Sprintf("ep-%02d", i), Name: fmt.Sprintf("ep %d", i), Group: "g", Type: "http",
			URL: srv.URL, Method: "GET", Interval: config.Duration(time.Second), Timeout: config.Duration(time.Second),
			Conditions: []string{"[STATUS] == 200"}, FailureThreshold: 1, SuccessThreshold: 1,
		}
		if i == 0 {
			e.Key, e.URL = "flap", srv.URL+"/flap"
			e.Alerts = []config.Alert{{Type: "test", FailureThreshold: 1, SuccessThreshold: 1}}
		}
		cfg.Endpoints = append(cfg.Endpoints, e)
	}
	cfg.Dashboards = []config.Dashboard{{Key: "default", Title: "x", Visibility: "public", Groups: []string{"g"}}}
	cfg.Hash = "test"
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}

	rec := &recordingNotifier{events: map[string][]notify.Kind{}}
	workers := make([]*Worker, 3)
	cancels := make([]context.CancelFunc, 3)
	dones := make([]chan struct{}, 3)
	for i := range workers {
		workers[i], cancels[i], dones[i] = startWorker(t, d, rec)
	}
	defer func() {
		for i := range cancels {
			cancels[i]()
			<-dones[i]
		}
	}()

	// Let membership settle, then flap the alerting endpoint a few times.
	time.Sleep(HeartbeatInterval + 2*time.Second)
	for range 4 {
		flapping.Store(!flapping.Load())
		time.Sleep(2500 * time.Millisecond)
	}

	owned := func(w *Worker) int {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.running)
	}
	total := 0
	for i, w := range workers {
		c := owned(w)
		t.Logf("worker %d owns %d endpoints", i, c)
		if c == 0 {
			t.Errorf("worker %d owns nothing; sharding isn't spreading load", i)
		}
		total += c
	}
	if total != n {
		t.Errorf("workers own %d endpoints in total, want %d (each exactly once)", total, n)
	}

	// Crash worker 0: it stops without deregistering, so the others only
	// notice through its missing heartbeats.
	workers[0].crash = true
	victim := map[string]bool{}
	workers[0].mu.Lock()
	for k := range workers[0].running {
		victim[k] = true
	}
	workers[0].mu.Unlock()
	cancels[0]()
	<-dones[0]
	crashedAt := time.Now()

	deadline := crashedAt.Add(LiveTimeout + 2*HeartbeatInterval)
	for {
		if owned(workers[1])+owned(workers[2]) == n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("endpoints not reassigned %s after crash", time.Since(crashedAt))
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Logf("reassigned in %s", time.Since(crashedAt).Round(100*time.Millisecond))
	time.Sleep(2 * time.Second)

	// Every endpoint the crashed worker owned has results again.
	for k := range victim {
		got, err := d.Ent.Result.Query().
			Where(result.HasEndpointWith(endpoint.Key(k)), result.CheckedAtGT(crashedAt)).
			Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got == 0 {
			t.Errorf("%s has no results after failover", k)
		}
	}

	// Alerts alternate triggered/resolved: a duplicate would show up as two
	// of the same kind in a row.
	evs := rec.snapshot()["flap"]
	t.Logf("flap events: %v", evs)
	if len(evs) < 3 {
		t.Fatalf("got %d alert events for 4 flips, want at least 3", len(evs))
	}
	for i := 1; i < len(evs); i++ {
		if evs[i] == evs[i-1] {
			t.Fatalf("duplicate %s notification at %d: %v", evs[i], i, evs)
		}
	}

	// Rollups agree with the raw results. Stop the workers first, or a check
	// can land between the rollup and the count.
	for i := range cancels {
		cancels[i]()
		<-dones[i]
	}
	if err := jobs.Rollup(ctx, d); err != nil {
		t.Fatal(err)
	}
	var raw, rolled int64
	if err := d.SQL.QueryRowContext(ctx, `SELECT count(*) FROM results`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := d.SQL.QueryRowContext(ctx, `SELECT coalesce(sum(checks), 0) FROM rollups WHERE resolution = '1d'`).Scan(&rolled); err != nil {
		t.Fatal(err)
	}
	if raw == 0 || raw != rolled {
		t.Errorf("1d rollups count %d checks, results table has %d", rolled, raw)
	}
}
