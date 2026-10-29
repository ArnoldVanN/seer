package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/ent/endpoint"
	"github.com/seer-status/seer/apps/server/ent/rollup"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/testdb"
)

func TestRollupResponseTimesCountSuccessesOnly(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	cfg, err := config.Parse([]byte(`
endpoints:
  - { key: a, url: "http://a", conditions: ["[STATUS] == 200"] }
  - { key: b, url: "http://b", conditions: ["[STATUS] == 200"] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	ids := d.Ent.Endpoint.Query().Order(ent.Asc(endpoint.FieldKey)).IDsX(ctx)

	// a: two successes and a fast failure. b: failures only.
	at := time.Now().UTC().Truncate(5 * time.Minute)
	for i, r := range []struct {
		ep int
		ok bool
		ms int64
	}{{ids[0], true, 100}, {ids[0], true, 300}, {ids[0], false, 5}, {ids[1], false, 7}} {
		d.Ent.Result.Create().SetEndpointID(r.ep).SetSlot(int64(i)).SetCheckedAt(at.Add(time.Duration(i) * time.Second)).
			SetSuccess(r.ok).SetResponseTimeMs(r.ms).ExecX(ctx)
	}
	if err := Rollup(ctx, d); err != nil {
		t.Fatal(err)
	}

	for _, res := range []rollup.Resolution{rollup.Resolution5m, rollup.Resolution1h, rollup.Resolution1d} {
		a := d.Ent.Rollup.Query().Where(rollup.EndpointID(ids[0]), rollup.ResolutionEQ(res)).OnlyX(ctx)
		if a.Checks != 3 || a.Successes != 2 || a.RtSumMs != 400 || a.RtMinMs != 100 || a.RtMaxMs != 300 {
			t.Errorf("%s rollup of a = %+v, want 3 checks and times of the 2 successes", res, a)
		}
		b := d.Ent.Rollup.Query().Where(rollup.EndpointID(ids[1]), rollup.ResolutionEQ(res)).OnlyX(ctx)
		if b.Checks != 1 || b.RtSumMs != 0 || b.RtMinMs != 0 || b.RtMaxMs != 0 {
			t.Errorf("%s rollup of b = %+v, want no response times", res, b)
		}
	}
}
