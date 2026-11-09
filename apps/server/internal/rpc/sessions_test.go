package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ArnoldVanN/seer/apps/server/internal/auth"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/events"
	"github.com/ArnoldVanN/seer/apps/server/internal/jobs"
	"github.com/ArnoldVanN/seer/apps/server/internal/session"
	"github.com/ArnoldVanN/seer/apps/server/internal/testdb"
	commonv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/common/v1"
	privatev1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/private/v1"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/private/v1/privatev1connect"
)

func bearer(tok string) connect.ClientOption {
	return connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+tok)
				return next(ctx, req)
			}
		}))
}

func TestSessions(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	cfg, err := config.Parse([]byte(`
endpoints:
  - { key: db, group: infra, url: "tcp://x:5432", conditions: ["[CONNECTED] == true"] }
dashboards:
  - { key: ops, visibility: private, groups: [infra], allowedGroups: [sre] }
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SyncConfig(ctx, d, cfg, 0, "test"); err != nil {
		t.Fatal(err)
	}
	sessions := session.New(d)
	mux := http.NewServeMux()
	mux.Handle(privatev1connect.NewPrivateStatusServiceHandler(NewPrivate(d, events.NewBus(d.DSN), cfg, sessions),
		connect.WithInterceptors(auth.NewInterceptor(secret, sessions))))
	mux.Handle(privatev1connect.NewSessionServiceHandler(&Sessions{Store: sessions},
		connect.WithInterceptors(auth.NewWebInterceptor(secret))))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	webTok, err := auth.MintWeb(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	web := privatev1connect.NewSessionServiceClient(http.DefaultClient, srv.URL, bearer(webTok))

	created, err := web.CreateSession(ctx, connect.NewRequest(&privatev1.CreateSessionRequest{
		Subject: "oidc:alice", Name: "Alice", Groups: []string{"sre"}, UserAgent: "test", Ip: "10.0.0.1",
		Email: "alice@acme.test", Username: "alice",
	}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.Id
	row := d.Ent.Session.GetX(ctx, session.Hash(id))
	if row.Email != "alice@acme.test" || row.Username != "alice" || row.IP != "10.0.0.1" {
		t.Errorf("stored session = %+v, want its email, username and IP", row)
	}
	got, err := web.GetSession(ctx, connect.NewRequest(&privatev1.GetSessionRequest{Id: id}))
	if err != nil || got.Msg.Session.Name != "Alice" {
		t.Fatalf("GetSession = %v, %v; want Alice's session", got, err)
	}
	if _, err := web.GetSession(ctx, connect.NewRequest(&privatev1.GetSessionRequest{Id: "nope"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("GetSession(unknown) err = %v, want not found", err)
	}

	// Groups come from the session, not the token.
	viewerTok, err := auth.Mint(secret, id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	priv := privatev1connect.NewPrivateStatusServiceClient(http.DefaultClient, srv.URL, bearer(viewerTok))
	list, err := priv.ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(list.Msg); !equal(got, []string{"ops"}) {
		t.Errorf("dashboards = %v, want [ops]", got)
	}

	// Each audience only opens its own service.
	asViewer := privatev1connect.NewSessionServiceClient(http.DefaultClient, srv.URL, bearer(viewerTok))
	if _, err := asViewer.GetSession(ctx, connect.NewRequest(&privatev1.GetSessionRequest{Id: id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("session service with a viewer token err = %v, want unauthenticated", err)
	}
	asWeb := privatev1connect.NewPrivateStatusServiceClient(http.DefaultClient, srv.URL, bearer(webTok))
	if _, err := asWeb.ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("status service with a web token err = %v, want unauthenticated", err)
	}

	if _, err := web.RevokeSession(ctx, connect.NewRequest(&privatev1.RevokeSessionRequest{Id: id})); err != nil {
		t.Fatal(err)
	}
	if _, err := priv.ListDashboards(ctx, connect.NewRequest(&commonv1.ListDashboardsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("after revoke err = %v, want unauthenticated", err)
	}
	if _, err := web.GetSession(ctx, connect.NewRequest(&privatev1.GetSessionRequest{Id: id})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("GetSession after revoke err = %v, want not found", err)
	}

}
