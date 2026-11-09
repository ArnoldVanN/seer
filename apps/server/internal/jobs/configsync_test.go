package jobs

import (
	"context"
	"testing"

	"github.com/ArnoldVanN/seer/apps/server/ent/dashboard"
	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/testdb"
)

func TestSyncConfigLeavesAPIRowsAlone(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()

	d.Ent.Endpoint.Create().SetKey("api-ep").SetName("API endpoint").SetType("push").
		SetIntervalSeconds(60).SetSpec([]byte(`{}`)).SetSource(endpoint.SourceAPI).ExecX(ctx)
	d.Ent.Endpoint.Create().SetKey("shared").SetName("owned by API").SetType("push").
		SetIntervalSeconds(60).SetSpec([]byte(`{}`)).SetSource(endpoint.SourceAPI).ExecX(ctx)
	d.Ent.Dashboard.Create().SetKey("api-dash").SetTitle("API dashboard").
		SetVisibility(dashboard.VisibilityPublic).SetPosition(0).SetSource(dashboard.SourceAPI).ExecX(ctx)
	d.Ent.Dashboard.Create().SetKey("shared-dash").SetTitle("owned by API").
		SetVisibility(dashboard.VisibilityPublic).SetPosition(0).SetSource(dashboard.SourceAPI).ExecX(ctx)

	sync := func(gen int64, yaml string) {
		t.Helper()
		cfg, err := config.Parse([]byte(yaml))
		if err != nil {
			t.Fatal(err)
		}
		if err := SyncConfig(ctx, d, cfg, gen, "test"); err != nil {
			t.Fatal(err)
		}
	}

	sync(1, `
dashboards:
  - { key: status, title: Status, visibility: public, endpoints: [a] }
  - { key: shared-dash, title: from config, visibility: public, endpoints: [a] }
endpoints:
  - { key: a, url: "http://a", conditions: ["[STATUS] == 200"] }
  - { key: shared, name: from config, url: "http://shared", conditions: ["[STATUS] == 200"] }
`)

	a := d.Ent.Endpoint.Query().Where(endpoint.Key("a")).OnlyX(ctx)
	if a.Source != endpoint.SourceConfig || !a.Enabled {
		t.Fatalf("a: source %s, enabled %v; want config, enabled", a.Source, a.Enabled)
	}
	shared := d.Ent.Endpoint.Query().Where(endpoint.Key("shared")).OnlyX(ctx)
	if shared.Source != endpoint.SourceAPI || shared.Name != "owned by API" {
		t.Fatalf("shared endpoint taken over by config: source %s, name %q", shared.Source, shared.Name)
	}
	sharedDash := d.Ent.Dashboard.Query().Where(dashboard.Key("shared-dash")).OnlyX(ctx)
	if sharedDash.Source != dashboard.SourceAPI || sharedDash.Title != "owned by API" {
		t.Fatalf("shared dashboard taken over by config: source %s, title %q", sharedDash.Source, sharedDash.Title)
	}

	// Dropping everything from the config disables and deletes only config rows.
	sync(2, `
dashboards:
  - { key: other, title: Other, visibility: public, endpoints: [b] }
endpoints:
  - { key: b, url: "http://b", conditions: ["[STATUS] == 200"] }
`)

	if d.Ent.Endpoint.Query().Where(endpoint.Key("a")).OnlyX(ctx).Enabled {
		t.Error("config endpoint a still enabled after removal from the config")
	}
	for _, k := range []string{"api-ep", "shared"} {
		if !d.Ent.Endpoint.Query().Where(endpoint.Key(k)).OnlyX(ctx).Enabled {
			t.Errorf("API endpoint %s disabled by config sync", k)
		}
	}
	if d.Ent.Dashboard.Query().Where(dashboard.Key("status")).ExistX(ctx) {
		t.Error("config dashboard status not deleted after removal from the config")
	}
	for _, k := range []string{"api-dash", "shared-dash"} {
		if !d.Ent.Dashboard.Query().Where(dashboard.Key(k)).ExistX(ctx) {
			t.Errorf("API dashboard %s deleted by config sync", k)
		}
	}
}
