package sites

import (
	"context"
	"testing"

	"github.com/ArnoldVanN/seer/apps/server/ent/dashboard"
	"github.com/ArnoldVanN/seer/apps/server/ent/site"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/jobs"
	"github.com/ArnoldVanN/seer/apps/server/internal/testdb"
)

func TestStore(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()

	cfg, err := config.Parse([]byte(`
ui: { title: Base, homeUrl: https://base, customCss: ".base{}", hidePoweredBy: true }
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
dashboards: [{ key: one, groups: [web] }, { key: two, groups: [web] }]
sites:
  - { key: first, dashboards: [one] }
  - { key: second, hosts: [Status.Example.com], ui: { title: Second, customCss: ".site{}", hidePoweredBy: false }, dashboards: [two] }
  - { key: admin, hosts: [admin.test], dashboards: ["*"], private: true }
`))
	if err != nil {
		t.Fatal(err)
	}

	// Before the first sync, a default site lists every dashboard.
	empty := New(d, cfg.UI)
	if s, err := empty.ForHost(ctx, "anything"); err != nil || s.Key != "default" {
		t.Fatalf("before sync: %+v, %v; want the default site", s, err)
	}

	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	// An API-owned site and dashboard, as the write API will create them.
	d.Ent.Dashboard.Create().SetKey("studio").SetTitle("Studio").
		SetVisibility(dashboard.VisibilityPublic).SetPosition(0).SetSite("studio").
		SetSource(dashboard.SourceAPI).ExecX(ctx)
	d.Ent.Site.Create().SetKey("studio").SetPosition(1000).SetHosts([]string{"status.game.test"}).
		SetDashboards([]string{"studio"}).SetUI([]byte(`{"title":"Game"}`)).SetSource(site.SourceAPI).ExecX(ctx)

	st := New(d, cfg.UI)
	get := func(host string) *Site {
		t.Helper()
		s, err := st.ForHost(ctx, host)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	s := get("status.example.com:443")
	if s.Key != "second" || s.UI.Title != "Second" || s.UI.HomeURL != "https://base" {
		t.Errorf("second = %+v, want its own title over the base ui", s)
	}
	if s.UI.CustomCSS != ".base{}\n.site{}" {
		t.Errorf("second css = %q, want base then own", s.UI.CustomCSS)
	}
	if *s.UI.HidePoweredBy {
		t.Error("a site's hidePoweredBy should override the top level's")
	}
	if s := get("elsewhere"); s.Key != "first" || !*s.UI.HidePoweredBy {
		t.Errorf("unknown host got %+v, want the first site", s)
	}
	if s := get("status.game.test"); s.Key != "studio" || s.UI.Title != "Game" || s.UI.HomeURL != "https://base" {
		t.Errorf("API site = %+v, want studio with the base ui merged in", s)
	}
	if s := get("admin.test"); len(s.Dashboards) != 3 {
		t.Errorf(`"*" site lists %v, want every dashboard including the API's`, s.Dashboards)
	}

	// Without sites in the config, the implicit default site lists the
	// config's dashboards only, not every studio's.
	plain, err := config.Parse([]byte(`
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
dashboards: [{ key: one, groups: [web] }]
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, plain, 1, "test"); err != nil {
		t.Fatal(err)
	}
	st.Invalidate()
	if s := get("elsewhere"); s.Key != "default" || len(s.Dashboards) != 1 || s.Dashboards[0] != "one" {
		t.Errorf("implicit default = %+v, want only the config's dashboard", s)
	}
	if s := get("status.game.test"); s.Key != "studio" {
		t.Errorf("API site gone after config sync: got %s", s.Key)
	}
	if s, _ := st.ByKey(ctx, "second"); s != nil {
		t.Error("site removed from the config is still served")
	}
}
