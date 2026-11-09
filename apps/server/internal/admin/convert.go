package admin

import (
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	adminv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1"
	commonv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/common/v1"
)

func endpointFromProto(in *adminv1.Endpoint) config.Endpoint {
	e := config.Endpoint{
		Key:              in.Key,
		Name:             in.Name,
		Group:            in.Group,
		Description:      in.Description,
		Links:            linksFromProto(in.Links),
		Type:             in.Type,
		URL:              in.Url,
		Interval:         duration(in.Interval),
		Timeout:          duration(in.Timeout),
		Method:           in.Method,
		Headers:          in.Headers,
		Body:             in.Body,
		Insecure:         in.Insecure,
		Token:            in.Token,
		Grace:            duration(in.Grace),
		Conditions:       in.Conditions,
		FailureThreshold: int(in.FailureThreshold),
		SuccessThreshold: int(in.SuccessThreshold),
		Site:             in.Site,
	}
	if in.Disabled {
		off := false
		e.Enabled = &off
	}
	if in.Dns != nil {
		e.DNS = &config.DNSQuery{Name: in.Dns.Name, Type: in.Dns.Type}
	}
	for _, a := range in.Alerts {
		e.Alerts = append(e.Alerts, config.Alert{
			Type:             a.Type,
			FailureThreshold: int(a.FailureThreshold),
			SuccessThreshold: int(a.SuccessThreshold),
			SendOnResolved:   a.SendOnResolved,
			Description:      a.Description,
			Target:           a.Target,
		})
	}
	return e
}

func endpointToProto(e config.Endpoint) *adminv1.Endpoint {
	out := &adminv1.Endpoint{
		Key:              e.Key,
		Name:             e.Name,
		Group:            e.Group,
		Description:      e.Description,
		Links:            linksToProto(e.Links),
		Type:             e.Type,
		Url:              e.URL,
		Interval:         durationpb.New(e.Interval.D()),
		Timeout:          durationpb.New(e.Timeout.D()),
		Disabled:         !e.IsEnabled(),
		Method:           e.Method,
		Headers:          e.Headers,
		Body:             e.Body,
		Insecure:         e.Insecure,
		Token:            e.Token,
		Conditions:       e.Conditions,
		FailureThreshold: int32(e.FailureThreshold),
		SuccessThreshold: int32(e.SuccessThreshold),
		Site:             e.Site,
	}
	if e.Grace != 0 {
		out.Grace = durationpb.New(e.Grace.D())
	}
	if e.DNS != nil {
		out.Dns = &adminv1.DNSQuery{Name: e.DNS.Name, Type: e.DNS.Type}
	}
	for _, a := range e.Alerts {
		out.Alerts = append(out.Alerts, &adminv1.Alert{
			Type:             a.Type,
			FailureThreshold: int32(a.FailureThreshold),
			SuccessThreshold: int32(a.SuccessThreshold),
			SendOnResolved:   a.SendOnResolved,
			Description:      a.Description,
			Target:           a.Target,
		})
	}
	return out
}

func duration(d *durationpb.Duration) config.Duration {
	if d == nil {
		return 0
	}
	return config.Duration(d.AsDuration())
}

func linksFromProto(ls []*commonv1.Link) []config.Link {
	var out []config.Link
	for _, l := range ls {
		out = append(out, config.Link{Name: l.Name, URL: l.Url})
	}
	return out
}

func linksToProto(ls []config.Link) []*commonv1.Link {
	var out []*commonv1.Link
	for _, l := range ls {
		out = append(out, &commonv1.Link{Name: l.Name, Url: l.URL})
	}
	return out
}

func visibilityFromProto(v commonv1.Visibility) string {
	if v == commonv1.Visibility_VISIBILITY_PRIVATE {
		return "private"
	}
	return "public"
}

func visibilityToProto(v string) commonv1.Visibility {
	if v == "private" {
		return commonv1.Visibility_VISIBILITY_PRIVATE
	}
	return commonv1.Visibility_VISIBILITY_PUBLIC
}

var ranges = map[commonv1.HistoryRange]string{
	commonv1.HistoryRange_HISTORY_RANGE_24H: "24h",
	commonv1.HistoryRange_HISTORY_RANGE_7D:  "7d",
	commonv1.HistoryRange_HISTORY_RANGE_30D: "30d",
	commonv1.HistoryRange_HISTORY_RANGE_90D: "90d",
	commonv1.HistoryRange_HISTORY_RANGE_1Y:  "1y",
}

// rangeFromProto leaves unspecified empty, for ApplyDefaults to fill.
func rangeFromProto(r commonv1.HistoryRange) string { return ranges[r] }

func rangeToProto(r string) commonv1.HistoryRange {
	for k, v := range ranges {
		if v == r {
			return k
		}
	}
	return commonv1.HistoryRange_HISTORY_RANGE_UNSPECIFIED
}
