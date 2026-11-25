package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/ent/endpointstate"
	"github.com/ArnoldVanN/seer/apps/server/internal/checker"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/jobs"
	"github.com/ArnoldVanN/seer/apps/server/internal/testdb"
)

// TestRecordAcrossPause checks that nothing is stored while paused, and that
// the first result after resuming starts from a fresh state: no resolve for
// the alert that fired before the pause.
func TestRecordAcrossPause(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()

	spec := config.Endpoint{
		Key: "cron", Name: "cron", Type: "push", Token: "0123456789abcdef",
		Interval: config.Duration(time.Minute), Timeout: config.Duration(time.Second),
		FailureThreshold: 2, SuccessThreshold: 1,
		Alerts: []config.Alert{{Type: "test", FailureThreshold: 2, SuccessThreshold: 1}},
	}
	gen := int64(0)
	sync := func(paused bool) {
		t.Helper()
		gen++
		e := spec
		e.Paused = paused
		if err := jobs.SyncConfig(ctx, d, &config.Config{Endpoints: []config.Endpoint{e}, Hash: fmt.Sprint(gen)}, gen, "test"); err != nil {
			t.Fatal(err)
		}
	}
	sync(false)
	ep := d.Ent.Endpoint.Query().Where(endpoint.Key("cron")).OnlyX(ctx)

	rec := &Recorder{DB: d}
	record := func(success bool) bool {
		t.Helper()
		now := time.Now()
		stored, err := rec.Record(ctx, ep.ID, &spec, now.UnixMilli(), checker.Outcome{CheckedAt: now, Success: success})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // one slot per millisecond
		return stored
	}
	notifications := func() int { return d.Ent.Notification.Query().CountX(ctx) }

	record(false)
	record(false)
	if n := notifications(); n != 1 {
		t.Fatalf("got %d notifications after going down, want 1", n)
	}

	sync(true)
	if record(false) {
		t.Error("a result was stored while paused")
	}

	sync(false)
	// One failure would fire again if the pre-pause counter carried over.
	record(false)
	if n := notifications(); n != 1 {
		t.Errorf("got %d notifications after one failure since resuming, want 1", n)
	}
	record(true)
	if n := notifications(); n != 1 {
		t.Errorf("got %d notifications after recovering, want 1 (no resolve for the pre-pause alert)", n)
	}
	st := d.Ent.EndpointState.Query().Where(endpointstate.EndpointID(ep.ID)).OnlyX(ctx)
	if st.Status != endpointstate.StatusUp || st.ConsecutiveFailures != 0 {
		t.Errorf("state = %s with %d failures, want up with 0", st.Status, st.ConsecutiveFailures)
	}
}
