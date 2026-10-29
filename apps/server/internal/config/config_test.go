package config

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"30s": 30 * time.Second,
		"14d": 14 * 24 * time.Hour,
		"2w":  14 * 24 * time.Hour,
		"2y":  730 * 24 * time.Hour,
		"0":   0,
		"":    0,
	} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseDuration("3x"); err == nil {
		t.Error("expected error for 3x")
	}
}

func TestExampleConfig(t *testing.T) {
	t.Setenv("SEER_DB_DSN", "postgres://x")
	raw, err := os.ReadFile("../../../../seer.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Storage.DSN != "postgres://x" {
		t.Errorf("env not expanded: %q", c.Storage.DSN)
	}
	if len(c.Endpoints) == 0 || c.Endpoints[0].Type != "http" || c.Endpoints[0].Method != "GET" {
		t.Errorf("defaults not applied: %+v", c.Endpoints[0])
	}
}

func TestDefaultsAndValidation(t *testing.T) {
	c, err := Parse([]byte(`
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
  - { key: b, url: "tcp://b:1", conditions: ["[CONNECTED] == true"] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Dashboards) != 1 || c.Dashboards[0].Visibility != "public" {
		t.Errorf("want one implicit public dashboard, got %+v", c.Dashboards)
	}
	if c.Endpoints[1].Type != "tcp" {
		t.Errorf("type not inferred from tcp:// url: %q", c.Endpoints[1].Type)
	}
	if c.Dashboards[0].DefaultRange != "90d" {
		t.Errorf("defaultRange = %q, want 90d", c.Dashboards[0].DefaultRange)
	}

	_, err = Parse([]byte(`
endpoints:
  - { key: Bad Key, url: "", conditions: [] }
  - { key: dup, url: "x", conditions: ["a == b"] }
  - { key: dup, url: "x", conditions: ["a == b"] }
  - { key: linked, url: "x", conditions: ["a == b"], links: [{ name: Runbook }] }
dashboards:
  - { key: d, visibility: secret, endpoints: [nope], defaultRange: 2d }
`))
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"key must match", "url is required", "at least one condition", "duplicate key", "visibility must be", "unknown endpoint", "defaultRange must be", "links[0]: name and url"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}

	if _, err := Parse([]byte("nope: 1\n")); err == nil {
		t.Error("unknown fields should be rejected")
	}
}

func TestSites(t *testing.T) {
	const eps = `
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
`
	c, err := Parse([]byte(eps + `
ui: { title: Base, homeUrl: https://base, footerLinks: [{ name: Privacy, url: /privacy }], hidePoweredBy: true }
dashboards: [{ key: one, groups: [web] }, { key: two, groups: [web] }]
sites:
  - { key: first, dashboards: [one] }
  - { key: second, hosts: [Status.Example.com], ui: { title: Second, hidePoweredBy: false }, dashboards: [two] }
`))
	if err != nil {
		t.Fatal(err)
	}
	s := c.SiteForHost("status.example.com:443")
	if s.Key != "second" || s.UI.Title != "Second" || s.UI.HomeURL != "https://base" || len(s.UI.FooterLinks) != 1 {
		t.Errorf("SiteForHost = %+v, want second with merged ui", s)
	}
	if *s.UI.HidePoweredBy || !*c.SiteByKey("first").UI.HidePoweredBy {
		t.Error("a site's hidePoweredBy should override the top level's")
	}
	if c.SiteForHost("elsewhere").Key != "first" {
		t.Error("unknown host should get the first site")
	}

	all, err := Parse([]byte(eps + `
dashboards: [{ key: one, groups: [web] }, { key: two, groups: [web] }]
sites: [{ key: a, dashboards: [one] }, { key: admin, hosts: [x], dashboards: ["*"], private: true }]
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := all.SiteByKey("admin").Dashboards; len(got) != 2 {
		t.Errorf(`"*" expanded to %v, want both dashboards`, got)
	}

	implicit, err := Parse([]byte(eps))
	if err != nil {
		t.Fatal(err)
	}
	if len(implicit.Sites) != 1 || len(implicit.Sites[0].Dashboards) != 1 {
		t.Errorf("want one implicit site with every dashboard, got %+v", implicit.Sites)
	}

	for name, cfg := range map[string]string{
		"second site without hosts": `
dashboards: [{ key: one, groups: [web] }, { key: two, groups: [web] }]
sites: [{ key: a, dashboards: [one] }, { key: b, dashboards: [two] }]`,
		"duplicate host": `
dashboards: [{ key: one, groups: [web] }]
sites: [{ key: a, hosts: [x], dashboards: [one] }, { key: b, hosts: [x], dashboards: [one] }]`,
		"dashboard on no site": `
dashboards: [{ key: one, groups: [web] }, { key: two, groups: [web] }]
sites: [{ key: a, dashboards: [one] }]`,
		"allowedGroups on a public site": `
dashboards: [{ key: one, groups: [web] }]
sites: [{ key: a, dashboards: [one], allowedGroups: [x] }]`,
		"unknown dashboard": `
dashboards: [{ key: one, groups: [web] }]
sites: [{ key: a, dashboards: [one, nope] }]`,
	} {
		if _, err := Parse([]byte(eps + cfg)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestDashboardCSSFile(t *testing.T) {
	f := t.TempDir() + "/d.css"
	if err := os.WriteFile(f, []byte(".d{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Parse([]byte(`
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
dashboards:
  - { key: one, groups: [web], customCss: ".inline{}", customCssFile: ` + f + ` }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.DashboardByKey("one").CustomCSS; got != ".inline{}\n.d{}" {
		t.Errorf("dashboard css = %q", got)
	}
	if _, err := Parse([]byte(`
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
dashboards:
  - { key: one, groups: [web], customCssFile: /nonexistent.css }
`)); err == nil || !strings.Contains(err.Error(), "dashboards[0]") {
		t.Errorf("missing file: err = %v, want one naming dashboards[0]", err)
	}
}

func TestSiteCSSFile(t *testing.T) {
	f := t.TempDir() + "/site.css"
	if err := os.WriteFile(f, []byte(".site{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Parse([]byte(`
endpoints:
  - { key: a, group: web, url: "https://a", conditions: ["[STATUS] == 200"] }
ui: { customCss: ".base{}" }
sites:
  - { key: s, dashboards: ["*"], ui: { customCss: ".inline{}", customCssFile: ` + f + ` } }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.SiteByKey("s").UI.CustomCSS; got != ".base{}\n.inline{}\n.site{}" {
		t.Errorf("site css = %q", got)
	}
}

func TestNestedSites(t *testing.T) {
	c, err := Parse([]byte(`
endpoints:
  - { key: db, group: infra, url: "tcp://db:5432", conditions: ["[CONNECTED] == true"] }
dashboards:
  - { key: infra, groups: [infra] }
sites:
  - key: home
    endpoints:
      - { key: portfolio, group: Sites, url: "https://a", conditions: ["[STATUS] == 200"] }
    dashboards:
      - { key: home, groups: [Sites], endpoints: [db] }
      - infra
  - key: acme
    hosts: [acme.test]
    endpoints:
      - { key: acme-web, group: Sites, url: "https://b", conditions: ["[STATUS] == 200"] }
    dashboards:
      - { key: acme, groups: [Sites] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.SiteByKey("home").Dashboards; !slices.Equal(got, []string{"home", "infra"}) {
		t.Errorf("home dashboards = %v", got)
	}
	shows := func(d, e string) bool { return c.Shows(c.DashboardByKey(d), c.EndpointByKey(e)) }
	for _, tc := range []struct {
		dash, ep string
		want     bool
	}{
		{"home", "portfolio", true},
		{"home", "acme-web", false}, // same group name, other site
		{"acme", "acme-web", true},
		{"acme", "portfolio", false},
		{"home", "db", true},  // listed by key
		{"infra", "db", true}, // top-level group, top-level endpoint
		{"infra", "portfolio", false},
	} {
		if got := shows(tc.dash, tc.ep); got != tc.want {
			t.Errorf("Shows(%s, %s) = %v, want %v", tc.dash, tc.ep, got, tc.want)
		}
	}

	for name, cfg := range map[string]string{
		"site dashboard using a top-level group": `
endpoints: [{ key: db, group: infra, url: "tcp://db:1", conditions: ["[CONNECTED] == true"] }]
sites: [{ key: a, dashboards: [{ key: d, groups: [infra] }] }]`,
		"duplicate key across sites": `
sites:
  - { key: a, endpoints: [{ key: x, group: g, url: "https://a", conditions: ["[STATUS] == 200"] }], dashboards: [{ key: da, groups: [g] }] }
  - { key: b, hosts: [b], endpoints: [{ key: x, group: g, url: "https://b", conditions: ["[STATUS] == 200"] }], dashboards: [{ key: db, groups: [g] }] }`,
		"unknown field in a nested dashboard": `
endpoints: [{ key: e, group: g, url: "https://a", conditions: ["[STATUS] == 200"] }]
dashboards: [{ key: top, groups: [g] }]
sites: [{ key: a, dashboards: [top, { key: d, grups: [g] }] }]`,
	} {
		_, err := Parse([]byte(cfg))
		if err == nil {
			t.Errorf("%s: want an error", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
