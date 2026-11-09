// Package admin implements the admin API: sites, dashboards, endpoints and
// alert targets created at runtime by another service. Rows written here are owned by the
// API. Config sync leaves them alone, and this package never changes rows the
// config owns: every write is guarded by source = 'api' in the database.
package admin

import (
	"context"
	"crypto/rand"
	stdsql "database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/ArnoldVanN/seer/apps/server/ent"
	"github.com/ArnoldVanN/seer/apps/server/ent/dashboard"
	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/ent/site"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/db"
	"github.com/ArnoldVanN/seer/apps/server/internal/sites"
	adminv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1"
	"github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1/adminv1connect"
)

// Position of API-created sites and dashboards. Config rows count up from 0,
// so the config's first site keeps catching unknown hosts.
const apiPosition = 1000

type Service struct {
	DB *db.DB
	// Alert types an endpoint may use need a provider here.
	Alerting config.Alerting
	// Invalidated after site writes, so this replica serves them at once.
	// Others pick them up within sites.DefaultTTL.
	Sites *sites.Store
}

var _ adminv1connect.AdminServiceHandler = (*Service)(nil)

func invalid(errs []error) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.Join(errs...))
}

func configOwned(kind, key string) error {
	return connect.NewError(connect.CodeFailedPrecondition,
		fmt.Errorf("%s %q is defined in the config file; change it there", kind, key))
}

// guarded runs an upsert that only updates rows the API owns. On a row the
// config owns it updates nothing, which ent reports as sql.ErrNoRows.
func guarded(err error, kind, key string) error {
	if errors.Is(err, stdsql.ErrNoRows) {
		return configOwned(kind, key)
	}
	return err
}

func onlyAPI(field string) sql.ConflictOption {
	return sql.UpdateWhere(sql.EQ(field, "api"))
}

func source(s string) adminv1.Source {
	if s == "api" {
		return adminv1.Source_SOURCE_API
	}
	return adminv1.Source_SOURCE_CONFIG
}

// Sites

func (s *Service) PutSite(ctx context.Context, req *connect.Request[adminv1.PutSiteRequest]) (*connect.Response[adminv1.PutSiteResponse], error) {
	in := req.Msg.GetSite()
	if in == nil {
		return nil, invalid([]error{errors.New("site is required")})
	}
	st, own, errs := siteFromProto(in)
	if len(errs) > 0 {
		return nil, invalid(errs)
	}
	ui, err := json.Marshal(own)
	if err != nil {
		return nil, err
	}
	// Under the config sync lock, so the host check sees a settled set of
	// sites and two writes can't claim the same host at once.
	err = s.DB.WithLock(ctx, db.LockConfigSync, func(ctx context.Context) error {
		others, err := s.DB.Ent.Site.Query().Where(site.KeyNEQ(st.Key)).All(ctx)
		if err != nil {
			return err
		}
		for _, o := range others {
			for _, h := range st.Hosts {
				if slices.Contains(o.Hosts, h) {
					return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("host %q is already on site %q", h, o.Key))
				}
			}
		}
		err = s.DB.Ent.Site.Create().
			SetKey(st.Key).
			SetPosition(apiPosition).
			SetHosts(st.Hosts).
			SetDashboards(st.Dashboards).
			SetAllDashboards(st.AllDashboards).
			SetPrivate(st.Private).
			SetAllowedGroups(st.AllowedGroups).
			SetUI(ui).
			SetSource(site.SourceAPI).
			OnConflict(sql.ConflictColumns(site.FieldKey), sql.ResolveWithNewValues(), onlyAPI(site.FieldSource)).
			Exec(ctx)
		return guarded(err, "site", st.Key)
	})
	if err != nil {
		return nil, err
	}
	if s.Sites != nil {
		s.Sites.Invalidate()
	}
	return connect.NewResponse(&adminv1.PutSiteResponse{Site: siteToProto(st, own)}), nil
}

func siteFromProto(in *adminv1.Site) (config.Site, config.UI, []error) {
	st := config.Site{
		Key:           in.Key,
		Dashboards:    in.Dashboards,
		AllDashboards: in.AllDashboards,
		Private:       in.Private,
		AllowedGroups: in.AllowedGroups,
	}
	var errs []error
	if !config.ValidKey(st.Key) {
		errs = append(errs, fmt.Errorf("site key %q must be lowercase letters, digits, - and _", st.Key))
	}
	seen := map[string]bool{}
	for _, h := range in.Hosts {
		h = config.NormalizeHost(h)
		if h == "" {
			errs = append(errs, errors.New("hosts: empty host"))
			continue
		}
		if !seen[h] {
			seen[h] = true
			st.Hosts = append(st.Hosts, h)
		}
	}
	if st.AllDashboards && len(st.Dashboards) > 0 {
		errs = append(errs, errors.New("dashboards and all_dashboards can't both be set"))
	}
	for _, d := range st.Dashboards {
		if !config.ValidKey(d) {
			errs = append(errs, fmt.Errorf("dashboards: invalid key %q", d))
		}
	}
	if len(st.AllowedGroups) > 0 && !st.Private {
		errs = append(errs, errors.New("allowed_groups needs private"))
	}
	var own config.UI
	if u := in.Ui; u != nil {
		own = config.UI{
			Title:         u.Title,
			Description:   u.Description,
			Logo:          u.LogoUrl,
			LogoDark:      u.LogoDarkUrl,
			HomeURL:       u.HomeUrl,
			Favicon:       u.FaviconUrl,
			CustomCSS:     strings.TrimSpace(u.CustomCss),
			Footer:        u.Footer,
			FooterLinks:   linksFromProto(u.FooterLinks),
			HidePoweredBy: u.HidePoweredBy,
		}
		errs = append(errs, config.ValidateLinks("ui.footer_links", own.FooterLinks)...)
	}
	return st, own, errs
}

func siteToProto(st config.Site, own config.UI) *adminv1.Site {
	return &adminv1.Site{
		Key:           st.Key,
		Hosts:         st.Hosts,
		Dashboards:    st.Dashboards,
		AllDashboards: st.AllDashboards,
		Private:       st.Private,
		AllowedGroups: st.AllowedGroups,
		Ui: &adminv1.SiteUI{
			Title:         own.Title,
			Description:   own.Description,
			LogoUrl:       own.Logo,
			LogoDarkUrl:   own.LogoDark,
			HomeUrl:       own.HomeURL,
			FaviconUrl:    own.Favicon,
			CustomCss:     own.CustomCSS,
			Footer:        own.Footer,
			FooterLinks:   linksToProto(own.FooterLinks),
			HidePoweredBy: own.HidePoweredBy,
		},
	}
}

func (s *Service) DeleteSite(ctx context.Context, req *connect.Request[adminv1.DeleteSiteRequest]) (*connect.Response[adminv1.DeleteSiteResponse], error) {
	key := req.Msg.Key
	n, err := s.DB.Ent.Site.Delete().Where(site.Key(key), site.SourceEQ(site.SourceAPI)).Exec(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if err := s.checkNotConfig(ctx, s.DB.Ent.Site.Query().Where(site.Key(key)).Exist, "site", key); err != nil {
			return nil, err
		}
	}
	if s.Sites != nil {
		s.Sites.Invalidate()
	}
	return connect.NewResponse(&adminv1.DeleteSiteResponse{}), nil
}

func (s *Service) ListSites(ctx context.Context, _ *connect.Request[adminv1.ListSitesRequest]) (*connect.Response[adminv1.ListSitesResponse], error) {
	rows, err := s.DB.Ent.Site.Query().Order(ent.Asc(site.FieldKey)).All(ctx)
	if err != nil {
		return nil, err
	}
	resp := &adminv1.ListSitesResponse{}
	for _, r := range rows {
		resp.Sites = append(resp.Sites, &adminv1.Item{Key: r.Key, Source: source(string(r.Source))})
	}
	return connect.NewResponse(resp), nil
}

// checkNotConfig runs after a delete matched no API row: a row that still
// exists belongs to the config.
func (s *Service) checkNotConfig(ctx context.Context, exists func(context.Context) (bool, error), kind, key string) error {
	ok, err := exists(ctx)
	if err != nil {
		return err
	}
	if ok {
		return configOwned(kind, key)
	}
	return nil
}

// Dashboards

func (s *Service) PutDashboard(ctx context.Context, req *connect.Request[adminv1.PutDashboardRequest]) (*connect.Response[adminv1.PutDashboardResponse], error) {
	in := req.Msg.GetDashboard()
	if in == nil {
		return nil, invalid([]error{errors.New("dashboard is required")})
	}
	d := config.Dashboard{
		Key:           in.Key,
		Title:         in.Title,
		Description:   in.Description,
		Visibility:    visibilityFromProto(in.Visibility),
		Groups:        in.Groups,
		Endpoints:     in.EndpointKeys,
		AllowedGroups: in.AllowedGroups,
		DefaultRange:  rangeFromProto(in.DefaultRange),
		CustomCSS:     strings.TrimSpace(in.CustomCss),
		Site:          in.Site,
	}
	d.ApplyDefaults()
	errs := d.Validate("dashboard")
	if d.Site != "" && !config.ValidKey(d.Site) {
		errs = append(errs, fmt.Errorf("site: invalid key %q", d.Site))
	}
	for _, k := range d.Endpoints {
		if !config.ValidKey(k) {
			errs = append(errs, fmt.Errorf("endpoint_keys: invalid key %q", k))
		}
	}
	if len(errs) > 0 {
		return nil, invalid(errs)
	}
	err := s.DB.Ent.Dashboard.Create().
		SetKey(d.Key).
		SetTitle(d.Title).
		SetVisibility(dashboard.Visibility(d.Visibility)).
		SetPosition(apiPosition).
		SetGroups(d.Groups).
		SetEndpointKeys(d.Endpoints).
		SetAllowedGroups(d.AllowedGroups).
		SetSite(d.Site).
		SetDescription(d.Description).
		SetDefaultRange(d.DefaultRange).
		SetCustomCSS(d.CustomCSS).
		SetSource(dashboard.SourceAPI).
		OnConflict(sql.ConflictColumns(dashboard.FieldKey), sql.ResolveWithNewValues(), onlyAPI(dashboard.FieldSource)).
		Exec(ctx)
	if err := guarded(err, "dashboard", d.Key); err != nil {
		return nil, err
	}
	// A site listing every dashboard resolves them when its sites load.
	if s.Sites != nil {
		s.Sites.Invalidate()
	}
	return connect.NewResponse(&adminv1.PutDashboardResponse{Dashboard: &adminv1.Dashboard{
		Key:           d.Key,
		Title:         d.Title,
		Description:   d.Description,
		Visibility:    visibilityToProto(d.Visibility),
		Groups:        d.Groups,
		EndpointKeys:  d.Endpoints,
		AllowedGroups: d.AllowedGroups,
		DefaultRange:  rangeToProto(d.DefaultRange),
		CustomCss:     d.CustomCSS,
		Site:          d.Site,
	}}), nil
}

func (s *Service) DeleteDashboard(ctx context.Context, req *connect.Request[adminv1.DeleteDashboardRequest]) (*connect.Response[adminv1.DeleteDashboardResponse], error) {
	key := req.Msg.Key
	n, err := s.DB.Ent.Dashboard.Delete().Where(dashboard.Key(key), dashboard.SourceEQ(dashboard.SourceAPI)).Exec(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if err := s.checkNotConfig(ctx, s.DB.Ent.Dashboard.Query().Where(dashboard.Key(key)).Exist, "dashboard", key); err != nil {
			return nil, err
		}
	}
	if s.Sites != nil {
		s.Sites.Invalidate()
	}
	return connect.NewResponse(&adminv1.DeleteDashboardResponse{}), nil
}

func (s *Service) ListDashboards(ctx context.Context, _ *connect.Request[adminv1.ListDashboardsRequest]) (*connect.Response[adminv1.ListDashboardsResponse], error) {
	rows, err := s.DB.Ent.Dashboard.Query().Order(ent.Asc(dashboard.FieldKey)).All(ctx)
	if err != nil {
		return nil, err
	}
	resp := &adminv1.ListDashboardsResponse{}
	for _, r := range rows {
		resp.Dashboards = append(resp.Dashboards, &adminv1.Item{Key: r.Key, Source: source(string(r.Source)), Site: r.Site})
	}
	return connect.NewResponse(resp), nil
}

// Endpoints

func (s *Service) PutEndpoint(ctx context.Context, req *connect.Request[adminv1.PutEndpointRequest]) (*connect.Response[adminv1.PutEndpointResponse], error) {
	in := req.Msg.GetEndpoint()
	if in == nil {
		return nil, invalid([]error{errors.New("endpoint is required")})
	}
	e := endpointFromProto(in)
	if e.Site != "" && !config.ValidKey(e.Site) {
		return nil, invalid([]error{fmt.Errorf("site: invalid key %q", e.Site)})
	}

	// A push endpoint without a token keeps its stored one, so a reconcile
	// loop can send the same definition every pass, or gets a new one.
	push := e.Type == "push" || (e.Type == "" && e.URL == "")
	if push {
		e.Type = "push"
		if e.Token == "" {
			tok, err := s.storedToken(ctx, e.Key)
			if err != nil {
				return nil, err
			}
			if tok == "" {
				tok, err = newToken()
				if err != nil {
					return nil, err
				}
			}
			e.Token = tok
		}
	}
	e.ApplyDefaults()
	errs := e.Validate("endpoint", s.Alerting)
	missing, err := s.missingTargets(ctx, e)
	if err != nil {
		return nil, err
	}
	if errs = append(errs, missing...); len(errs) > 0 {
		return nil, invalid(errs)
	}

	spec, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	err = s.DB.Ent.Endpoint.Create().
		SetKey(e.Key).
		SetName(e.Name).
		SetGroupName(e.Group).
		SetType(e.Type).
		SetIntervalSeconds(max(1, int(e.Interval.D().Seconds()))).
		SetEnabled(e.IsEnabled()).
		SetSpec(spec).
		SetSite(e.Site).
		SetSource(endpoint.SourceAPI).
		OnConflict(sql.ConflictColumns(endpoint.FieldKey), sql.ResolveWithNewValues(), onlyAPI(endpoint.FieldSource)).
		Exec(ctx)
	if err := guarded(err, "endpoint", e.Key); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.PutEndpointResponse{Endpoint: endpointToProto(e)}), nil
}

// storedToken returns the token of an API-owned push endpoint, or "".
func (s *Service) storedToken(ctx context.Context, key string) (string, error) {
	row, err := s.DB.Ent.Endpoint.Query().Where(endpoint.Key(key), endpoint.SourceEQ(endpoint.SourceAPI)).Only(ctx)
	if ent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var spec config.Endpoint
	if err := json.Unmarshal(row.Spec, &spec); err != nil {
		return "", fmt.Errorf("endpoint %s: decode spec: %w", key, err)
	}
	return spec.Token, nil
}

func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) DeleteEndpoint(ctx context.Context, req *connect.Request[adminv1.DeleteEndpointRequest]) (*connect.Response[adminv1.DeleteEndpointResponse], error) {
	key := req.Msg.Key
	var n int
	var err error
	if req.Msg.Purge {
		n, err = s.DB.Ent.Endpoint.Delete().Where(endpoint.Key(key), endpoint.SourceEQ(endpoint.SourceAPI)).Exec(ctx)
	} else {
		n, err = s.DB.Ent.Endpoint.Update().Where(endpoint.Key(key), endpoint.SourceEQ(endpoint.SourceAPI)).SetEnabled(false).Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if err := s.checkNotConfig(ctx, s.DB.Ent.Endpoint.Query().Where(endpoint.Key(key)).Exist, "endpoint", key); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&adminv1.DeleteEndpointResponse{}), nil
}

func (s *Service) ListEndpoints(ctx context.Context, _ *connect.Request[adminv1.ListEndpointsRequest]) (*connect.Response[adminv1.ListEndpointsResponse], error) {
	rows, err := s.DB.Ent.Endpoint.Query().Order(ent.Asc(endpoint.FieldKey)).All(ctx)
	if err != nil {
		return nil, err
	}
	resp := &adminv1.ListEndpointsResponse{}
	for _, r := range rows {
		resp.Endpoints = append(resp.Endpoints, &adminv1.Item{Key: r.Key, Source: source(string(r.Source)), Site: r.Site, Enabled: r.Enabled})
	}
	return connect.NewResponse(resp), nil
}
