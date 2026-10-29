package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/seer-status/seer/apps/server/internal/auth"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/events"
	"github.com/seer-status/seer/apps/server/internal/jobs"
	"github.com/seer-status/seer/apps/server/internal/testdb"
	commonv1 "github.com/seer-status/seer/packages/protos/gen/go/seer/common/v1"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/private/v1/privatev1connect"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/public/v1/publicv1connect"
)

const secret = "test-secret"

func TestVisibility(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	cfg, err := config.Parse([]byte(`
endpoints:
  - { key: web, group: public, url: "http://x", conditions: ["[STATUS] == 200"] }
  - { key: db,  group: infra,  url: "tcp://x:5432", conditions: ["[CONNECTED] == true"] }
dashboards:
  - { key: status, visibility: public,  groups: [public] }
  - { key: ops,    visibility: private, groups: [public, infra], allowedGroups: [sre] }
  - { key: all,    visibility: private, groups: [infra] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus(d.DSN)

	pubMux := http.NewServeMux()
	pubMux.Handle(publicv1connect.NewStatusServiceHandler(NewPublic(d, bus, cfg)))
	pub := httptest.NewServer(pubMux)
	defer pub.Close()

	privMux := http.NewServeMux()
	privMux.Handle(privatev1connect.NewPrivateStatusServiceHandler(NewPrivate(d, bus, cfg),
		connect.WithInterceptors(auth.NewInterceptor(secret))))
	priv := httptest.NewServer(privMux)
	defer priv.Close()

	pc := publicv1connect.NewStatusServiceClient(http.DefaultClient, pub.URL)

	list, err := pc.ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(list.Msg); len(got) != 1 || got[0] != "status" {
		t.Errorf("public dashboards = %v, want [status]", got)
	}
	if _, err := pc.GetDashboard(ctx, connect.NewRequest(&commonv1.GetDashboardRequest{Key: "ops"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("public GetDashboard(ops) err = %v, want not found", err)
	}
	if _, err := pc.GetEndpointHistory(ctx, connect.NewRequest(&commonv1.GetEndpointHistoryRequest{Key: "db"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("public history of private-only endpoint err = %v, want not found", err)
	}
	if _, err := pc.GetEndpointHistory(ctx, connect.NewRequest(&commonv1.GetEndpointHistoryRequest{Key: "web"})); err != nil {
		t.Errorf("public history of public endpoint: %v", err)
	}
	dash, err := pc.GetDashboard(ctx, connect.NewRequest(&commonv1.GetDashboardRequest{Key: "status"}))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(dash.Msg.Dashboard.Groups[0].Endpoints[0].History); n != 90 {
		t.Errorf("default history has %d buckets, want 90", n)
	}

	privClient := func(groups ...string) privatev1connect.PrivateStatusServiceClient {
		var opts []connect.ClientOption
		if groups != nil {
			tok, err := auth.Mint(secret, "alice", groups, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(
				func(next connect.UnaryFunc) connect.UnaryFunc {
					return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
						req.Header().Set("Authorization", "Bearer "+tok)
						return next(ctx, req)
					}
				})))
		}
		return privatev1connect.NewPrivateStatusServiceClient(http.DefaultClient, priv.URL, opts...)
	}

	if _, err := privClient().ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("private without token err = %v, want unauthenticated", err)
	}
	for _, c := range []struct {
		groups []string
		want   []string
	}{
		{[]string{"dev"}, []string{"status", "all"}},
		{[]string{"sre"}, []string{"status", "ops", "all"}},
	} {
		list, err := privClient(c.groups...).ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(list.Msg); !equal(got, c.want) {
			t.Errorf("groups %v see %v, want %v", c.groups, got, c.want)
		}
	}
}

func keys(m *commonv1.ListDashboardsResponse) []string {
	var out []string
	for _, d := range m.Dashboards {
		out = append(out, d.Key)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSites(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	cfg, err := config.Parse([]byte(`
ui: { title: Shared, customCss: ".base{}" }
sites:
  - { key: home, hosts: [status.home.test], dashboards: [home] }
  - key: acme
    hosts: [status.acme.test]
    ui: { title: Acme, customCss: ".acme{}" }
    dashboards: [acme, acme-ops]
  - { key: admin, hosts: [admin.test], dashboards: ["*"], private: true, allowedGroups: [admin] }
endpoints:
  - { key: blog, group: home, url: "http://x", conditions: ["[STATUS] == 200"] }
  - { key: api,  group: acme, url: "http://y", conditions: ["[STATUS] == 200"] }
dashboards:
  - { key: home,     groups: [home] }
  - { key: acme,     groups: [acme], customCss: ".x{}" }
  - { key: acme-ops, groups: [acme], visibility: private }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(publicv1connect.NewStatusServiceHandler(NewPublic(d, events.NewBus(d.DSN), cfg)))
	srv := httptest.NewServer(WithSite(cfg, mux))
	defer srv.Close()

	client := func(header, value string) publicv1connect.StatusServiceClient {
		return publicv1connect.NewStatusServiceClient(http.DefaultClient, srv.URL,
			connect.WithInterceptors(connect.UnaryInterceptorFunc(
				func(next connect.UnaryFunc) connect.UnaryFunc {
					return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
						if header != "" {
							req.Header().Set(header, value)
						}
						return next(ctx, req)
					}
				})))
	}

	for _, c := range []struct {
		name, header, value string
		want                []string
		title, css          string
	}{
		{"forwarded host", "X-Forwarded-Host", "status.acme.test:443", []string{"acme"}, "Acme", ".base{}\n.acme{}"},
		{"site header", SiteHeader, "acme", []string{"acme"}, "Acme", ".base{}\n.acme{}"},
		{"unknown host falls back to first site", "X-Forwarded-Host", "localhost", []string{"home"}, "Shared", ".base{}"},
	} {
		pc := client(c.header, c.value)
		list, err := pc.ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(list.Msg); !equal(got, c.want) {
			t.Errorf("%s: dashboards = %v, want %v", c.name, got, c.want)
		}
		site, err := pc.GetSiteConfig(ctx, connect.NewRequest(&commonv1.GetSiteConfigRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if s := site.Msg.Site; s.Title != c.title || s.CustomCss != c.css {
			t.Errorf("%s: title %q css %q, want %q %q", c.name, s.Title, s.CustomCss, c.title, c.css)
		}
		// acme lists acme-ops; home has only public dashboards.
		if got, want := site.Msg.Site.HasPrivate, c.title == "Acme"; got != want {
			t.Errorf("%s: hasPrivate = %v, want %v", c.name, got, want)
		}
	}

	// Another site's dashboards and endpoints don't exist here.
	home := client(SiteHeader, "home")
	if _, err := home.GetDashboard(ctx, connect.NewRequest(&commonv1.GetDashboardRequest{Key: "acme"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("home GetDashboard(acme) err = %v, want not found", err)
	}
	if _, err := home.GetEndpointHistory(ctx, connect.NewRequest(&commonv1.GetEndpointHistoryRequest{Key: "api"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("home history of acme endpoint err = %v, want not found", err)
	}

	acme, err := client(SiteHeader, "acme").GetDashboard(ctx, connect.NewRequest(&commonv1.GetDashboardRequest{Key: "acme"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := acme.Msg.Dashboard.CustomCss; got != ".x{}" {
		t.Errorf("acme dashboard css = %q", got)
	}

	// The private admin site shows nothing publicly, and everything to its
	// groups on the private port.
	list, err := client(SiteHeader, "admin").ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(list.Msg); len(got) != 0 {
		t.Errorf("public admin site lists %v, want nothing", got)
	}
	privMux := http.NewServeMux()
	privMux.Handle(privatev1connect.NewPrivateStatusServiceHandler(NewPrivate(d, events.NewBus(d.DSN), cfg),
		connect.WithInterceptors(auth.NewInterceptor(secret))))
	priv := httptest.NewServer(WithSite(cfg, privMux))
	defer priv.Close()
	for _, c := range []struct {
		groups []string
		want   []string
	}{
		{[]string{"admin"}, []string{"home", "acme", "acme-ops"}},
		{[]string{"dev"}, nil},
	} {
		tok, err := auth.Mint(secret, "alice", c.groups, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		pc := privatev1connect.NewPrivateStatusServiceClient(http.DefaultClient, priv.URL,
			connect.WithInterceptors(connect.UnaryInterceptorFunc(
				func(next connect.UnaryFunc) connect.UnaryFunc {
					return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
						req.Header().Set("Authorization", "Bearer "+tok)
						req.Header().Set(SiteHeader, "admin")
						return next(ctx, req)
					}
				})))
		list, err := pc.ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if got := keys(list.Msg); !equal(got, c.want) {
			t.Errorf("admin site, groups %v: %v, want %v", c.groups, got, c.want)
		}
	}
}

func TestSiteScopedGroups(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	cfg, err := config.Parse([]byte(`
sites:
  - key: home
    endpoints: [{ key: portfolio, group: Sites, url: "https://a", conditions: ["[STATUS] == 200"] }]
    dashboards: [{ key: home, groups: [Sites] }]
  - key: acme
    hosts: [acme.test]
    endpoints: [{ key: acme-web, group: Sites, url: "https://b", conditions: ["[STATUS] == 200"] }]
    dashboards: [{ key: acme, groups: [Sites] }]
  - { key: all, hosts: [all.test], dashboards: ["*"] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(publicv1connect.NewStatusServiceHandler(NewPublic(d, events.NewBus(d.DSN), cfg)))
	srv := httptest.NewServer(WithSite(cfg, mux))
	defer srv.Close()

	endpointsOn := func(site, dash string) []string {
		pc := publicv1connect.NewStatusServiceClient(http.DefaultClient, srv.URL,
			connect.WithInterceptors(connect.UnaryInterceptorFunc(
				func(next connect.UnaryFunc) connect.UnaryFunc {
					return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
						req.Header().Set(SiteHeader, site)
						return next(ctx, req)
					}
				})))
		res, err := pc.GetDashboard(ctx, connect.NewRequest(&commonv1.GetDashboardRequest{Key: dash}))
		if err != nil {
			t.Fatalf("%s/%s: %v", site, dash, err)
		}
		var out []string
		for _, g := range res.Msg.Dashboard.Groups {
			for _, e := range g.Endpoints {
				out = append(out, e.Key)
			}
		}
		return out
	}
	for _, c := range []struct{ site, dash, want string }{
		{"home", "home", "portfolio"},
		{"acme", "acme", "acme-web"},
		// The same dashboards through the site that lists everything.
		{"all", "home", "portfolio"},
		{"all", "acme", "acme-web"},
	} {
		if got := endpointsOn(c.site, c.dash); !equal(got, []string{c.want}) {
			t.Errorf("%s/%s shows %v, want [%s]", c.site, c.dash, got, c.want)
		}
	}
}
