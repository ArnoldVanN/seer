// Package push receives pings for push endpoints: cron jobs and other things
// that report in rather than being checked. A ping is stored like any check
// result; the worker owning the endpoint records a failure when they stop
// (see scheduler.watchPush).
package push

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ArnoldVanN/seer/apps/server/ent"
	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/internal/checker"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/db"
	"github.com/ArnoldVanN/seer/apps/server/internal/scheduler"
)

// Path is where the handler is mounted; the endpoint key follows it.
const Path = "/push/"

// Pings per endpoint: a burst, then one a second. Enough for a job that
// reports each step, not for one stuck in a loop.
const (
	burst  = 5
	refill = time.Second
)

type Handler struct {
	DB       *db.DB
	Recorder *scheduler.Recorder

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

func New(d *db.DB, rec *scheduler.Recorder) *Handler {
	return &Handler{DB: d, Recorder: rec, buckets: map[string]*bucket{}}
}

// ServeHTTP handles GET or POST /push/{key}, with the token as a bearer
// token or ?token=. Optional query parameters:
//
//	status    up (default) or down
//	msg       shown as the result's error, e.g. why the job failed
//	duration  how long the job took: milliseconds or a Go duration (1.5s)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, Path)
	q := r.URL.Query()

	out := checker.Outcome{CheckedAt: time.Now(), Success: true}
	switch q.Get("status") {
	case "", "up":
	case "down":
		out.Success = false
	default:
		http.Error(w, "status must be up or down", http.StatusBadRequest)
		return
	}
	out.Error = q.Get("msg")
	if s := q.Get("duration"); s != "" {
		d, err := parseDuration(s)
		if err != nil {
			http.Error(w, "duration must be milliseconds or a duration like 1.5s", http.StatusBadRequest)
			return
		}
		out.ResponseTime = d
	}

	// Unknown, disabled and wrong-token all answer the same, so keys can't
	// be probed for.
	e, spec, ok := h.lookup(r, key)
	if !ok || !validToken(r, spec.Token) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.allow(key, out.CheckedAt) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many pings", http.StatusTooManyRequests)
		return
	}

	// The slot is the ping's millisecond: each ping is its own result.
	if _, err := h.Recorder.Record(r.Context(), e.ID, spec, out.CheckedAt.UnixMilli(), out); err != nil {
		log.Error().Err(err).Str("endpoint", key).Msg("record ping")
		http.Error(w, "could not record ping", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("ok\n"))
}

// lookup reads the endpoint from the DB, like workers do, so the token
// matches the synced config even while replicas roll over to a new one.
func (h *Handler) lookup(r *http.Request, key string) (*ent.Endpoint, *config.Endpoint, bool) {
	if key == "" || strings.Contains(key, "/") {
		return nil, nil, false
	}
	e, err := h.DB.Ent.Endpoint.Query().
		Where(endpoint.Key(key), endpoint.Enabled(true), endpoint.Type("push")).
		Only(r.Context())
	if err != nil {
		if !ent.IsNotFound(err) {
			log.Error().Err(err).Str("endpoint", key).Msg("look up push endpoint")
		}
		return nil, nil, false
	}
	var spec config.Endpoint
	if err := json.Unmarshal(e.Spec, &spec); err != nil {
		log.Error().Err(err).Str("endpoint", key).Msg("bad endpoint spec")
		return nil, nil, false
	}
	return e, &spec, true
}

func validToken(r *http.Request, want string) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		got = r.URL.Query().Get("token")
	}
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// allow is a token bucket per endpoint. It only runs after the token
// checks out, so the map holds at most one entry per push endpoint.
func (h *Handler) allow(key string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.buckets[key]
	if !ok {
		b = &bucket{tokens: burst, at: now}
		h.buckets[key] = b
	}
	b.tokens = min(burst, b.tokens+float64(now.Sub(b.at))/float64(refill))
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func parseDuration(s string) (time.Duration, error) {
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil && ms >= 0 {
		return time.Duration(ms) * time.Millisecond, nil
	}
	d, err := time.ParseDuration(s)
	if err == nil && d < 0 {
		err = strconv.ErrRange
	}
	return d, err
}
