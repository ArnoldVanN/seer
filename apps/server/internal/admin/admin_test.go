package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/internal/admin"
	"github.com/ArnoldVanN/seer/apps/server/internal/auth"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/events"
	"github.com/ArnoldVanN/seer/apps/server/internal/jobs"
	"github.com/ArnoldVanN/seer/apps/server/internal/push"
	"github.com/ArnoldVanN/seer/apps/server/internal/rpc"
	"github.com/ArnoldVanN/seer/apps/server/internal/scheduler"
	"github.com/ArnoldVanN/seer/apps/server/internal/sites"
	"github.com/ArnoldVanN/seer/apps/server/internal/testdb"
	adminv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1/adminv1connect"
	commonv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/common/v1"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/public/v1/publicv1connect"
)

const key = "0123456789abcdef0123456789abcdef"

func bearer(tok string) connect.ClientOption {
	return connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if tok != "" {
				req.Header().Set("Authorization", "Bearer "+tok)
			}
			return next(ctx, req)
		}
	}))
}

func code(err error) connect.Code {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Code()
	}
	return 0
}

func TestAdmin(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()

	cfg, err := config.Parse([]byte(`
alerting:
  webhook: { url: "https://hooks.test" }
endpoints:
  - { key: web, group: web, url: "https://example.com", conditions: ["[STATUS] == 200"] }
dashboards: [{ key: main, groups: [web] }]
sites:
  - { key: main, hosts: [status.aquila.test], dashboards: [main] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}

	st := sites.New(d, cfg.UI)
	mux := http.NewServeMux()
	mux.Handle(adminv1connect.NewAdminServiceHandler(
		&admin.Service{DB: d, Alerting: cfg.Alerting, Sites: st},
		connect.WithInterceptors(auth.NewAdminInterceptor([]string{key})),
	))
	pub := http.NewServeMux()
	pub.Handle(publicv1connect.NewStatusServiceHandler(rpc.NewPublic(d, events.NewBus(d.DSN), st)))
	pub.Handle(push.Path, push.New(d, &scheduler.Recorder{DB: d}))
	mux.Handle("/", rpc.WithSite(st, pub))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := adminv1connect.NewAdminServiceClient(http.DefaultClient, srv.URL, bearer(key))

	t.Run("auth", func(t *testing.T) {
		for name, tok := range map[string]string{"no key": "", "wrong key": key + "x"} {
			anon := adminv1connect.NewAdminServiceClient(http.DefaultClient, srv.URL, bearer(tok))
			_, err := anon.ListSites(ctx, connect.NewRequest(&adminv1.ListSitesRequest{}))
			if code(err) != connect.CodeUnauthenticated {
				t.Errorf("%s: %v, want unauthenticated", name, err)
			}
		}
	})

	t.Run("config rows are read-only", func(t *testing.T) {
		_, err := c.PutEndpoint(ctx, connect.NewRequest(&adminv1.PutEndpointRequest{Endpoint: &adminv1.Endpoint{
			Key: "web", Url: "https://other.test", Conditions: []string{"[STATUS] == 200"},
		}}))
		if code(err) != connect.CodeFailedPrecondition {
			t.Errorf("put endpoint: %v, want failed precondition", err)
		}
		_, err = c.PutDashboard(ctx, connect.NewRequest(&adminv1.PutDashboardRequest{Dashboard: &adminv1.Dashboard{Key: "main"}}))
		if code(err) != connect.CodeFailedPrecondition {
			t.Errorf("put dashboard: %v, want failed precondition", err)
		}
		_, err = c.PutSite(ctx, connect.NewRequest(&adminv1.PutSiteRequest{Site: &adminv1.Site{Key: "main"}}))
		if code(err) != connect.CodeFailedPrecondition {
			t.Errorf("put site: %v, want failed precondition", err)
		}
		for name, del := range map[string]func() error{
			"endpoint": func() error {
				_, err := c.DeleteEndpoint(ctx, connect.NewRequest(&adminv1.DeleteEndpointRequest{Key: "web", Purge: true}))
				return err
			},
			"dashboard": func() error {
				_, err := c.DeleteDashboard(ctx, connect.NewRequest(&adminv1.DeleteDashboardRequest{Key: "main"}))
				return err
			},
			"site": func() error {
				_, err := c.DeleteSite(ctx, connect.NewRequest(&adminv1.DeleteSiteRequest{Key: "main"}))
				return err
			},
		} {
			if err := del(); code(err) != connect.CodeFailedPrecondition {
				t.Errorf("delete %s: %v, want failed precondition", name, err)
			}
		}
		e := d.Ent.Endpoint.Query().Where(endpoint.Key("web")).OnlyX(ctx)
		if !e.Enabled || e.Source != endpoint.SourceConfig {
			t.Errorf("config endpoint changed: %+v", e)
		}
	})

	t.Run("validation", func(t *testing.T) {
		for name, ep := range map[string]*adminv1.Endpoint{
			"bad key":            {Key: "Bad Key"},
			"http without check": {Key: "x", Url: "https://x.test"},
			"unconfigured alert": {Key: "x", Alerts: []*adminv1.Alert{{Type: "slack"}}},
		} {
			_, err := c.PutEndpoint(ctx, connect.NewRequest(&adminv1.PutEndpointRequest{Endpoint: ep}))
			if code(err) != connect.CodeInvalidArgument {
				t.Errorf("%s: %v, want invalid argument", name, err)
			}
		}
		_, err := c.PutSite(ctx, connect.NewRequest(&adminv1.PutSiteRequest{Site: &adminv1.Site{
			Key: "squatter", Hosts: []string{"Status.Aquila.test"},
		}}))
		if code(err) != connect.CodeAlreadyExists {
			t.Errorf("host of another site: %v, want already exists", err)
		}
	})

	t.Run("studio site end to end", func(t *testing.T) {
		put, err := c.PutEndpoint(ctx, connect.NewRequest(&adminv1.PutEndpointRequest{Endpoint: &adminv1.Endpoint{
			Key: "aq-1-eu", Name: "EU servers", Group: "Fleets", Site: "aq-1",
			Interval: durationpb.New(time.Minute),
			Alerts:   []*adminv1.Alert{{Type: "webhook"}},
		}}))
		if err != nil {
			t.Fatal(err)
		}
		tok := put.Msg.Endpoint.Token
		if put.Msg.Endpoint.Type != "push" || len(tok) < config.MinPushToken {
			t.Fatalf("push endpoint = %+v, want a generated token", put.Msg.Endpoint)
		}
		again, err := c.PutEndpoint(ctx, connect.NewRequest(&adminv1.PutEndpointRequest{Endpoint: &adminv1.Endpoint{
			Key: "aq-1-eu", Name: "EU servers", Group: "Fleets", Site: "aq-1", Interval: durationpb.New(time.Minute),
		}}))
		if err != nil {
			t.Fatal(err)
		}
		if again.Msg.Endpoint.Token != tok {
			t.Error("a second put without a token replaced the stored one")
		}

		if _, err := c.PutDashboard(ctx, connect.NewRequest(&adminv1.PutDashboardRequest{Dashboard: &adminv1.Dashboard{
			Key: "aq-1", Title: "Game status", Groups: []string{"Fleets"}, Site: "aq-1",
			DefaultRange: commonv1.HistoryRange_HISTORY_RANGE_30D,
		}})); err != nil {
			t.Fatal(err)
		}
		if _, err := c.PutSite(ctx, connect.NewRequest(&adminv1.PutSiteRequest{Site: &adminv1.Site{
			Key: "aq-1", Hosts: []string{"status.game.test"}, Dashboards: []string{"aq-1"},
			Ui: &adminv1.SiteUI{Title: "Game"},
		}})); err != nil {
			t.Fatal(err)
		}

		status := publicv1connect.NewStatusServiceClient(http.DefaultClient, srv.URL, connect.WithInterceptors(
			connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
				return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
					req.Header().Set("X-Forwarded-Host", "status.game.test")
					return next(ctx, req)
				}
			})))
		sc, err := status.GetSiteConfig(ctx, connect.NewRequest(&commonv1.GetSiteConfigRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if sc.Msg.Site.Key != "aq-1" || sc.Msg.Site.Title != "Game" {
			t.Errorf("site for status.game.test = %s %q, want aq-1 \"Game\"", sc.Msg.Site.Key, sc.Msg.Site.Title)
		}
		dash, err := status.GetDashboard(ctx, connect.NewRequest(&commonv1.GetDashboardRequest{Key: "aq-1"}))
		if err != nil {
			t.Fatal(err)
		}
		gs := dash.Msg.Dashboard.Groups
		if len(gs) != 1 || len(gs[0].Endpoints) != 1 || gs[0].Endpoints[0].Key != "aq-1-eu" {
			t.Errorf("dashboard groups = %v, want the fleet endpoint", gs)
		}
		if dash.Msg.Dashboard.HistoryRange != commonv1.HistoryRange_HISTORY_RANGE_30D {
			t.Errorf("history range = %v, want 30d", dash.Msg.Dashboard.HistoryRange)
		}

		req, _ := http.NewRequest("POST", srv.URL+push.Path+"aq-1-eu?status=down&msg=empty+buffer", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("push with the returned token: status %d", resp.StatusCode)
		}

		// Delete keeps history and a later put brings it back; purge removes it.
		if _, err := c.DeleteEndpoint(ctx, connect.NewRequest(&adminv1.DeleteEndpointRequest{Key: "aq-1-eu"})); err != nil {
			t.Fatal(err)
		}
		if d.Ent.Endpoint.Query().Where(endpoint.Key("aq-1-eu")).OnlyX(ctx).Enabled {
			t.Error("deleted endpoint still enabled")
		}
		if _, err := c.PutEndpoint(ctx, connect.NewRequest(&adminv1.PutEndpointRequest{Endpoint: &adminv1.Endpoint{
			Key: "aq-1-eu", Group: "Fleets", Site: "aq-1", Interval: durationpb.New(time.Minute),
		}})); err != nil {
			t.Fatal(err)
		}
		if !d.Ent.Endpoint.Query().Where(endpoint.Key("aq-1-eu")).OnlyX(ctx).Enabled {
			t.Error("put didn't bring the endpoint back")
		}
		if _, err := c.DeleteEndpoint(ctx, connect.NewRequest(&adminv1.DeleteEndpointRequest{Key: "aq-1-eu", Purge: true})); err != nil {
			t.Fatal(err)
		}
		if d.Ent.Endpoint.Query().Where(endpoint.Key("aq-1-eu")).ExistX(ctx) {
			t.Error("purged endpoint still exists")
		}

		if _, err := c.DeleteSite(ctx, connect.NewRequest(&adminv1.DeleteSiteRequest{Key: "aq-1"})); err != nil {
			t.Fatal(err)
		}
		sc, err = status.GetSiteConfig(ctx, connect.NewRequest(&commonv1.GetSiteConfigRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if sc.Msg.Site.Key != "main" {
			t.Errorf("deleted site's host now serves %s, want the default site", sc.Msg.Site.Key)
		}
	})

	t.Run("alert targets", func(t *testing.T) {
		for name, tg := range map[string]*adminv1.AlertTarget{
			"no kind":            {Key: "t"},
			"bad url":            {Key: "t", Kind: &adminv1.AlertTarget_Slack{Slack: &adminv1.ChatTarget{WebhookUrl: "ftp://x"}}},
			"email without smtp": {Key: "t", Kind: &adminv1.AlertTarget_Email{Email: &adminv1.EmailTarget{To: []string{"ops@game.test"}}}},
		} {
			_, err := c.PutAlertTarget(ctx, connect.NewRequest(&adminv1.PutAlertTargetRequest{Target: tg}))
			if code(err) != connect.CodeInvalidArgument {
				t.Errorf("%s: %v, want invalid argument", name, err)
			}
		}

		withAlert := func(a *adminv1.Alert) error {
			_, err := c.PutEndpoint(ctx, connect.NewRequest(&adminv1.PutEndpointRequest{Endpoint: &adminv1.Endpoint{
				Key: "aq-2-eu", Site: "aq-2", Alerts: []*adminv1.Alert{a},
			}}))
			return err
		}
		if err := withAlert(&adminv1.Alert{Target: "aq-2-slack"}); code(err) != connect.CodeInvalidArgument {
			t.Errorf("unknown target: %v, want invalid argument", err)
		}
		if _, err := c.PutAlertTarget(ctx, connect.NewRequest(&adminv1.PutAlertTargetRequest{Target: &adminv1.AlertTarget{
			Key: "aq-2-slack", Kind: &adminv1.AlertTarget_Slack{Slack: &adminv1.ChatTarget{WebhookUrl: "https://hooks.slack.test/x"}},
		}})); err != nil {
			t.Fatal(err)
		}
		if err := withAlert(&adminv1.Alert{Type: "webhook", Target: "aq-2-slack"}); code(err) != connect.CodeInvalidArgument {
			t.Errorf("type and target: %v, want invalid argument", err)
		}
		if err := withAlert(&adminv1.Alert{Target: "aq-2-slack"}); err != nil {
			t.Errorf("known target: %v", err)
		}
		list, err := c.ListAlertTargets(ctx, connect.NewRequest(&adminv1.ListAlertTargetsRequest{}))
		if err != nil || len(list.Msg.Targets) != 1 || list.Msg.Targets[0].Key != "aq-2-slack" {
			t.Errorf("targets = %v, %v", list.Msg.GetTargets(), err)
		}
		if _, err := c.DeleteAlertTarget(ctx, connect.NewRequest(&adminv1.DeleteAlertTargetRequest{Key: "aq-2-slack"})); err != nil {
			t.Error(err)
		}
	})

	t.Run("deleting a missing key is fine", func(t *testing.T) {
		if _, err := c.DeleteSite(ctx, connect.NewRequest(&adminv1.DeleteSiteRequest{Key: "nope"})); err != nil {
			t.Error(err)
		}
		if _, err := c.DeleteEndpoint(ctx, connect.NewRequest(&adminv1.DeleteEndpointRequest{Key: "nope"})); err != nil {
			t.Error(err)
		}
	})

	t.Run("list", func(t *testing.T) {
		resp, err := c.ListDashboards(ctx, connect.NewRequest(&adminv1.ListDashboardsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]adminv1.Source{}
		for _, it := range resp.Msg.Dashboards {
			got[it.Key] = it.Source
		}
		if got["main"] != adminv1.Source_SOURCE_CONFIG || got["aq-1"] != adminv1.Source_SOURCE_API {
			t.Errorf("dashboards = %v", got)
		}
	})

	t.Run("config sync keeps API rows", func(t *testing.T) {
		if err := jobs.SyncConfig(ctx, d, cfg, 1, "test"); err != nil {
			t.Fatal(err)
		}
		resp, err := c.ListDashboards(ctx, connect.NewRequest(&adminv1.ListDashboardsRequest{}))
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Msg.Dashboards) != 2 {
			t.Errorf("after sync: %d dashboards, want config's and API's", len(resp.Msg.Dashboards))
		}
	})
}
