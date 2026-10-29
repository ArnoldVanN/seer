// Package rpc implements the status services. One Service type serves both
// ports; the private flag decides whether private dashboards are visible.
package rpc

import (
	"context"
	"errors"
	"slices"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/ent/dashboard"
	"github.com/seer-status/seer/apps/server/ent/endpoint"
	"github.com/seer-status/seer/apps/server/ent/endpointstate"
	"github.com/seer-status/seer/apps/server/ent/result"
	"github.com/seer-status/seer/apps/server/ent/rollup"
	"github.com/seer-status/seer/apps/server/internal/auth"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
	"github.com/seer-status/seer/apps/server/internal/events"
	commonv1 "github.com/seer-status/seer/packages/protos/gen/go/seer/common/v1"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/private/v1/privatev1connect"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/public/v1/publicv1connect"
)

type Service struct {
	DB      *db.DB
	Bus     *events.Bus
	Config  *config.Config
	private bool
}

var (
	_ publicv1connect.StatusServiceHandler         = (*Service)(nil)
	_ privatev1connect.PrivateStatusServiceHandler = (*Service)(nil)
)

func NewPublic(d *db.DB, bus *events.Bus, cfg *config.Config) *Service {
	return &Service{DB: d, Bus: bus, Config: cfg}
}

func NewPrivate(d *db.DB, bus *events.Bus, cfg *config.Config) *Service {
	return &Service{DB: d, Bus: bus, Config: cfg, private: true}
}

var errNotFound = connect.NewError(connect.CodeNotFound, errors.New("not found"))

// canView is the single place visibility is decided. A dashboard must be on
// the request's site, the caller must be allowed on the site if it's private,
// and then the dashboard must be public or allowed for the caller.
func (s *Service) canView(ctx context.Context, d *ent.Dashboard) bool {
	site := s.siteFrom(ctx)
	if !slices.Contains(site.Dashboards, d.Key) {
		return false
	}
	if site.Private && !s.allowed(ctx, site.AllowedGroups) {
		return false
	}
	if d.Visibility == dashboard.VisibilityPublic {
		return true
	}
	return s.allowed(ctx, d.AllowedGroups)
}

// allowed reports whether the caller is signed in (private port only) and in
// one of groups. Empty groups admits any signed-in caller.
func (s *Service) allowed(ctx context.Context, groups []string) bool {
	if !s.private {
		return false
	}
	v, ok := auth.FromContext(ctx)
	if !ok {
		return false
	}
	if len(groups) == 0 {
		return true
	}
	for _, g := range v.Groups {
		if slices.Contains(groups, g) {
			return true
		}
	}
	return false
}

// visibleDashboards returns what the caller can see, in the site's order.
func (s *Service) visibleDashboards(ctx context.Context) ([]*ent.Dashboard, error) {
	order := s.siteFrom(ctx).Dashboards
	all, err := s.DB.Ent.Dashboard.Query().Where(dashboard.KeyIn(order...)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, d := range all {
		if s.canView(ctx, d) {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b *ent.Dashboard) int {
		return slices.Index(order, a.Key) - slices.Index(order, b.Key)
	})
	return out, nil
}

func (s *Service) dashboard(ctx context.Context, key string) (*ent.Dashboard, error) {
	d, err := s.DB.Ent.Dashboard.Query().Where(dashboard.Key(key)).Only(ctx)
	if ent.IsNotFound(err) || (err == nil && !s.canView(ctx, d)) {
		// Hidden dashboards look missing rather than forbidden.
		return nil, errNotFound
	}
	return d, err
}

// endpointsOf returns a dashboard's enabled endpoints, ordered by the
// dashboard's group order and then by name.
func (s *Service) endpointsOf(ctx context.Context, d *ent.Dashboard) ([]*ent.Endpoint, error) {
	eps, err := s.DB.Ent.Endpoint.Query().
		Where(
			endpoint.Enabled(true),
			endpoint.Or(endpoint.GroupNameIn(d.Groups...), endpoint.KeyIn(d.EndpointKeys...)),
		).
		Order(ent.Asc(endpoint.FieldName)).
		WithState().
		All(ctx)
	if err != nil {
		return nil, err
	}
	eps = slices.DeleteFunc(eps, func(e *ent.Endpoint) bool { return !s.shows(d, e) })
	rank := func(e *ent.Endpoint) int {
		if i := slices.Index(d.Groups, e.GroupName); i >= 0 {
			return i
		}
		return len(d.Groups)
	}
	slices.SortStableFunc(eps, func(a, b *ent.Endpoint) int { return rank(a) - rank(b) })
	return eps, nil
}

func (s *Service) ListDashboards(ctx context.Context, _ *connect.Request[commonv1.ListDashboardsRequest]) (*connect.Response[commonv1.ListDashboardsResponse], error) {
	ds, err := s.visibleDashboards(ctx)
	if err != nil {
		return nil, err
	}
	resp := &commonv1.ListDashboardsResponse{}
	for _, d := range ds {
		resp.Dashboards = append(resp.Dashboards, &commonv1.DashboardSummary{
			Key: d.Key, Title: d.Title, Visibility: visibility(d.Visibility),
		})
	}
	return connect.NewResponse(resp), nil
}

func (s *Service) GetDashboard(ctx context.Context, req *connect.Request[commonv1.GetDashboardRequest]) (*connect.Response[commonv1.GetDashboardResponse], error) {
	d, err := s.dashboard(ctx, req.Msg.Key)
	if err != nil {
		return nil, err
	}
	eps, err := s.endpointsOf(ctx, d)
	if err != nil {
		return nil, err
	}
	hist, err := s.history(ctx, eps, req.Msg.HistoryRange)
	if err != nil {
		return nil, err
	}

	out := &commonv1.Dashboard{Key: d.Key, Title: d.Title, Visibility: visibility(d.Visibility)}
	if cd := s.Config.DashboardByKey(d.Key); cd != nil {
		out.CustomCss = cd.CustomCSS
	}
	var cur *commonv1.EndpointGroup
	for _, e := range eps {
		if cur == nil || cur.Name != e.GroupName {
			cur = &commonv1.EndpointGroup{Name: e.GroupName}
			out.Groups = append(out.Groups, cur)
		}
		cur.Endpoints = append(cur.Endpoints, toEndpoint(e, hist[e.ID]))
	}
	return connect.NewResponse(&commonv1.GetDashboardResponse{Dashboard: out}), nil
}

// shows applies the config's group scoping: a group matches only endpoints
// defined next to the dashboard. The database rows have no owner, so this
// reads it from the config.
func (s *Service) shows(d *ent.Dashboard, e *ent.Endpoint) bool {
	cd, ce := s.Config.DashboardByKey(d.Key), s.Config.EndpointByKey(e.Key)
	if cd == nil || ce == nil {
		return false
	}
	return s.Config.Shows(cd, ce)
}

// visibleEndpoint finds an endpoint that is on at least one dashboard the
// caller can see. Endpoints have no visibility of their own.
func (s *Service) visibleEndpoint(ctx context.Context, key string) (*ent.Endpoint, error) {
	e, err := s.DB.Ent.Endpoint.Query().Where(endpoint.Key(key), endpoint.Enabled(true)).WithState().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	ds, err := s.visibleDashboards(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range ds {
		if s.shows(d, e) {
			return e, nil
		}
	}
	return nil, errNotFound
}

func (s *Service) GetEndpointHistory(ctx context.Context, req *connect.Request[commonv1.GetEndpointHistoryRequest]) (*connect.Response[commonv1.GetEndpointHistoryResponse], error) {
	e, err := s.visibleEndpoint(ctx, req.Msg.Key)
	if err != nil {
		return nil, err
	}
	hist, err := s.history(ctx, []*ent.Endpoint{e}, req.Msg.HistoryRange)
	if err != nil {
		return nil, err
	}
	recent, err := s.DB.Ent.Result.Query().
		Where(result.EndpointID(e.ID)).
		Order(ent.Desc(result.FieldCheckedAt)).
		Limit(50).
		All(ctx)
	if err != nil {
		return nil, err
	}
	resp := &commonv1.GetEndpointHistoryResponse{Endpoint: toEndpoint(e, hist[e.ID])}
	for _, r := range recent {
		resp.RecentResults = append(resp.RecentResults, &commonv1.Result{
			CheckedAt:      timestamppb.New(r.CheckedAt),
			Success:        r.Success,
			ResponseTimeMs: r.ResponseTimeMs,
			Error:          r.Error,
		})
	}
	return connect.NewResponse(resp), nil
}

func (s *Service) GetSiteConfig(ctx context.Context, _ *connect.Request[commonv1.GetSiteConfigRequest]) (*connect.Response[commonv1.GetSiteConfigResponse], error) {
	cur := s.siteFrom(ctx)
	ui := cur.UI
	site := &commonv1.SiteConfig{
		Key:         cur.Key,
		Private:     cur.Private,
		HasPrivate:  s.hasPrivate(cur),
		Title:       ui.Title,
		Description: ui.Description,
		LogoUrl:     ui.Logo,
		LogoDarkUrl: ui.LogoDark,
		HomeUrl:     ui.HomeURL,
		FaviconUrl:  ui.Favicon,
		CustomCss:   ui.CustomCSS,
	}
	return connect.NewResponse(&commonv1.GetSiteConfigResponse{Site: site}), nil
}

func (s *Service) hasPrivate(site *config.Site) bool {
	for _, k := range site.Dashboards {
		if d := s.Config.DashboardByKey(k); d != nil && d.Visibility == "private" {
			return true
		}
	}
	return false
}

const (
	heartbeatEvery   = 15 * time.Second
	refreshKeysEvery = 30 * time.Second
)

func (s *Service) WatchStatus(ctx context.Context, req *connect.Request[commonv1.WatchStatusRequest], stream *connect.ServerStream[commonv1.StatusEvent]) error {
	keysOf := func() (map[string]bool, error) {
		d, err := s.dashboard(ctx, req.Msg.DashboardKey)
		if err != nil {
			return nil, err
		}
		eps, err := s.endpointsOf(ctx, d)
		if err != nil {
			return nil, err
		}
		keys := make(map[string]bool, len(eps))
		for _, e := range eps {
			keys[e.Key] = true
		}
		return keys, nil
	}
	keys, err := keysOf()
	if err != nil {
		return err
	}

	updates, unsubscribe := s.Bus.Subscribe()
	defer unsubscribe()

	heartbeat := time.NewTicker(heartbeatEvery)
	defer heartbeat.Stop()
	refresh := time.NewTicker(refreshKeysEvery)
	defer refresh.Stop()

	// Send a heartbeat right away so clients and proxies see the stream open.
	if err := stream.Send(heartbeatEvent()); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			if err := stream.Send(heartbeatEvent()); err != nil {
				return err
			}
		case <-refresh.C:
			// Picks up config changes, and ends the stream if the dashboard
			// was removed or the caller lost access.
			if keys, err = keysOf(); err != nil {
				return err
			}
		case u := <-updates:
			if !keys[u.Key] {
				continue
			}
			if err := stream.Send(&commonv1.StatusEvent{Event: &commonv1.StatusEvent_EndpointUpdate{
				EndpointUpdate: &commonv1.EndpointUpdate{
					Key:            u.Key,
					Status:         status(u.Status),
					CheckedAt:      timestamppb.New(u.CheckedAt),
					ResponseTimeMs: u.ResponseTimeMs,
					Success:        u.Success,
				},
			}}); err != nil {
				return err
			}
		}
	}
}

func heartbeatEvent() *commonv1.StatusEvent {
	return &commonv1.StatusEvent{Event: &commonv1.StatusEvent_Heartbeat{
		Heartbeat: &commonv1.Heartbeat{At: timestamppb.Now()},
	}}
}

type window struct {
	resolution rollup.Resolution
	stride     time.Duration
	buckets    int
}

func windowFor(r commonv1.HistoryRange) window {
	switch r {
	case commonv1.HistoryRange_HISTORY_RANGE_24H:
		return window{rollup.Resolution1h, time.Hour, 24}
	case commonv1.HistoryRange_HISTORY_RANGE_7D:
		return window{rollup.Resolution1h, time.Hour, 168}
	case commonv1.HistoryRange_HISTORY_RANGE_30D:
		return window{rollup.Resolution1d, 24 * time.Hour, 30}
	case commonv1.HistoryRange_HISTORY_RANGE_1Y:
		return window{rollup.Resolution1d, 24 * time.Hour, 365}
	default:
		return window{rollup.Resolution1d, 24 * time.Hour, 90}
	}
}

// history returns a fixed-length bucket series per endpoint ID, oldest first,
// with empty buckets where there was no data. It only reads rollups.
func (s *Service) history(ctx context.Context, eps []*ent.Endpoint, r commonv1.HistoryRange) (map[int][]*commonv1.HistoryBucket, error) {
	w := windowFor(r)
	// Truncate is relative to the zero time, which lines up with UTC
	// hours and days, matching the rollup buckets.
	first := time.Now().UTC().Truncate(w.stride).Add(-time.Duration(w.buckets-1) * w.stride)

	ids := make([]int, len(eps))
	for i, e := range eps {
		ids[i] = e.ID
	}
	rows, err := s.DB.Ent.Rollup.Query().
		Where(
			rollup.EndpointIDIn(ids...),
			rollup.ResolutionEQ(w.resolution),
			rollup.BucketGTE(first),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[int][]*commonv1.HistoryBucket, len(eps))
	for _, id := range ids {
		series := make([]*commonv1.HistoryBucket, w.buckets)
		for i := range series {
			series[i] = &commonv1.HistoryBucket{Start: timestamppb.New(first.Add(time.Duration(i) * w.stride))}
		}
		out[id] = series
	}
	for _, row := range rows {
		i := int(row.Bucket.Sub(first) / w.stride)
		if i < 0 || i >= w.buckets {
			continue
		}
		b := out[row.EndpointID][i]
		b.Checks = row.Checks
		b.Successes = row.Successes
		if row.Checks > 0 {
			b.AvgResponseTimeMs = float64(row.RtSumMs) / float64(row.Checks)
			b.MinResponseTimeMs = row.RtMinMs
			b.MaxResponseTimeMs = row.RtMaxMs
		}
	}
	return out, nil
}

func toEndpoint(e *ent.Endpoint, hist []*commonv1.HistoryBucket) *commonv1.Endpoint {
	out := &commonv1.Endpoint{
		Key:     e.Key,
		Name:    e.Name,
		Group:   e.GroupName,
		Status:  commonv1.Status_STATUS_UNKNOWN,
		History: hist,
	}
	if st := e.Edges.State; st != nil {
		out.Status = status(string(st.Status))
		out.LastResponseTimeMs = st.LastResponseTimeMs
		if st.LastCheckedAt != nil {
			out.LastCheckedAt = timestamppb.New(*st.LastCheckedAt)
		}
	}
	var checks, ok int64
	for _, b := range hist {
		checks += b.Checks
		ok += b.Successes
	}
	if checks > 0 {
		out.Uptime = float64(ok) / float64(checks)
	}
	return out
}

func status(s string) commonv1.Status {
	switch s {
	case string(endpointstate.StatusUp):
		return commonv1.Status_STATUS_UP
	case string(endpointstate.StatusDown):
		return commonv1.Status_STATUS_DOWN
	default:
		return commonv1.Status_STATUS_UNKNOWN
	}
}

func visibility(v dashboard.Visibility) commonv1.Visibility {
	if v == dashboard.VisibilityPrivate {
		return commonv1.Visibility_VISIBILITY_PRIVATE
	}
	return commonv1.Visibility_VISIBILITY_PUBLIC
}
