package push

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/ent/endpointstate"
	"github.com/ArnoldVanN/seer/apps/server/ent/result"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/jobs"
	"github.com/ArnoldVanN/seer/apps/server/internal/scheduler"
	"github.com/ArnoldVanN/seer/apps/server/internal/testdb"
)

const token = "0123456789abcdef"

func TestPush(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()

	cfg, err := config.Parse([]byte(`
storage: { dsn: "postgres://unused" }
endpoints:
  - { key: backup, interval: 1h, token: ` + token + ` }
  - { key: off, interval: 1h, token: ` + token + `, enabled: false }
  - { key: web, url: "https://example.com", conditions: ["[STATUS] == 200"] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	h := New(d, &scheduler.Recorder{DB: d})

	do := func(method, target, auth string) int {
		req := httptest.NewRequest(method, target, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, tc := range []struct {
		name, method, target, auth string
		want                       int
	}{
		{"bearer", "POST", "/push/backup?duration=1500", token, http.StatusOK},
		{"query token, failure", "GET", "/push/backup?token=" + token + "&status=down&msg=disk+full&duration=2s", "", http.StatusOK},
		{"wrong token", "POST", "/push/backup", "nope", http.StatusNotFound},
		{"no token", "POST", "/push/backup", "", http.StatusNotFound},
		{"unknown key", "POST", "/push/nope", token, http.StatusNotFound},
		{"disabled", "POST", "/push/off", token, http.StatusNotFound},
		{"not a push endpoint", "POST", "/push/web", token, http.StatusNotFound},
		{"bad status", "POST", "/push/backup?status=meh", token, http.StatusBadRequest},
		{"bad duration", "POST", "/push/backup?duration=soon", token, http.StatusBadRequest},
		{"method", "DELETE", "/push/backup", token, http.StatusMethodNotAllowed},
	} {
		if got := do(tc.method, tc.target, tc.auth); got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.want)
		}
		time.Sleep(2 * time.Millisecond) // one slot per millisecond
	}

	rs, err := d.Ent.Result.Query().
		Where(result.HasEndpointWith(endpoint.Key("backup"))).
		Order(result.ByCheckedAt()).
		All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("got %d results, want 2", len(rs))
	}
	if !rs[0].Success || rs[0].ResponseTimeMs != 1500 {
		t.Errorf("first ping: %+v", rs[0])
	}
	if rs[1].Success || rs[1].Error != "disk full" || rs[1].ResponseTimeMs != 2000 {
		t.Errorf("second ping: %+v", rs[1])
	}
	st, err := d.Ent.EndpointState.Query().
		Where(endpointstate.HasEndpointWith(endpoint.Key("backup"))).
		Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != endpointstate.StatusDown {
		t.Errorf("status %s after a down ping, want down", st.Status)
	}

	// The burst is spent quickly; the limiter then refuses.
	limited := false
	for range burst + 1 {
		if do("POST", "/push/backup", token) == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Error("expected 429 after a burst of pings")
	}
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"1500": 1500 * time.Millisecond,
		"0":    0,
		"1.5s": 1500 * time.Millisecond,
		"2m":   2 * time.Minute,
	} {
		if got, err := parseDuration(in); err != nil || got != want {
			t.Errorf("parseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"-5", "-1s", "soon"} {
		if _, err := parseDuration(in); err == nil {
			t.Errorf("parseDuration(%q): expected error", in)
		}
	}
}
