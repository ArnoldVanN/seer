package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/ArnoldVanN/seer/apps/server/ent"
	"github.com/ArnoldVanN/seer/apps/server/ent/alerttarget"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	adminv1 "github.com/ArnoldVanN/seer/packages/protos/gen/go/seer/admin/v1"
)

func (s *Service) PutAlertTarget(ctx context.Context, req *connect.Request[adminv1.PutAlertTargetRequest]) (*connect.Response[adminv1.PutAlertTargetResponse], error) {
	in := req.Msg.GetTarget()
	if in == nil {
		return nil, invalid([]error{errors.New("target is required")})
	}
	var errs []error
	if !config.ValidKey(in.Key) {
		errs = append(errs, fmt.Errorf("target key %q must be lowercase letters, digits, - and _", in.Key))
	}
	t := targetFromProto(in)
	errs = append(errs, t.Validate(s.Alerting)...)
	if len(errs) > 0 {
		return nil, invalid(errs)
	}
	spec, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	if err := s.DB.Ent.AlertTarget.Create().
		SetKey(in.Key).
		SetSpec(spec).
		OnConflict(sql.ConflictColumns(alerttarget.FieldKey), sql.ResolveWithNewValues()).
		Exec(ctx); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.PutAlertTargetResponse{Target: targetToProto(in.Key, t)}), nil
}

func (s *Service) DeleteAlertTarget(ctx context.Context, req *connect.Request[adminv1.DeleteAlertTargetRequest]) (*connect.Response[adminv1.DeleteAlertTargetResponse], error) {
	if _, err := s.DB.Ent.AlertTarget.Delete().Where(alerttarget.Key(req.Msg.Key)).Exec(ctx); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.DeleteAlertTargetResponse{}), nil
}

func (s *Service) ListAlertTargets(ctx context.Context, _ *connect.Request[adminv1.ListAlertTargetsRequest]) (*connect.Response[adminv1.ListAlertTargetsResponse], error) {
	keys, err := s.DB.Ent.AlertTarget.Query().Order(ent.Asc(alerttarget.FieldKey)).Select(alerttarget.FieldKey).Strings(ctx)
	if err != nil {
		return nil, err
	}
	resp := &adminv1.ListAlertTargetsResponse{}
	for _, k := range keys {
		resp.Targets = append(resp.Targets, &adminv1.Item{Key: k, Source: adminv1.Source_SOURCE_API})
	}
	return connect.NewResponse(resp), nil
}

// missingTargets returns the alert targets e names that don't exist.
func (s *Service) missingTargets(ctx context.Context, e config.Endpoint) ([]error, error) {
	var want []string
	for _, a := range e.Alerts {
		if a.Target != "" && !slices.Contains(want, a.Target) {
			want = append(want, a.Target)
		}
	}
	if len(want) == 0 {
		return nil, nil
	}
	have, err := s.DB.Ent.AlertTarget.Query().Where(alerttarget.KeyIn(want...)).Select(alerttarget.FieldKey).Strings(ctx)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, k := range want {
		if !slices.Contains(have, k) {
			errs = append(errs, fmt.Errorf("endpoint: unknown alert target %q", k))
		}
	}
	return errs, nil
}

func targetFromProto(in *adminv1.AlertTarget) config.AlertTarget {
	switch k := in.Kind.(type) {
	case *adminv1.AlertTarget_Webhook:
		return config.AlertTarget{Type: "webhook", URL: k.Webhook.GetUrl(), Method: k.Webhook.GetMethod(), Headers: k.Webhook.GetHeaders()}
	case *adminv1.AlertTarget_Slack:
		return config.AlertTarget{Type: "slack", URL: k.Slack.GetWebhookUrl()}
	case *adminv1.AlertTarget_Discord:
		return config.AlertTarget{Type: "discord", URL: k.Discord.GetWebhookUrl()}
	case *adminv1.AlertTarget_Email:
		return config.AlertTarget{Type: "email", To: k.Email.GetTo()}
	}
	return config.AlertTarget{}
}

func targetToProto(key string, t config.AlertTarget) *adminv1.AlertTarget {
	out := &adminv1.AlertTarget{Key: key}
	switch t.Type {
	case "webhook":
		out.Kind = &adminv1.AlertTarget_Webhook{Webhook: &adminv1.WebhookTarget{Url: t.URL, Method: t.Method, Headers: t.Headers}}
	case "slack":
		out.Kind = &adminv1.AlertTarget_Slack{Slack: &adminv1.ChatTarget{WebhookUrl: t.URL}}
	case "discord":
		out.Kind = &adminv1.AlertTarget_Discord{Discord: &adminv1.ChatTarget{WebhookUrl: t.URL}}
	case "email":
		out.Kind = &adminv1.AlertTarget_Email{Email: &adminv1.EmailTarget{To: t.To}}
	}
	return out
}
