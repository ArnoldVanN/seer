package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/internal/session"
	privatev1 "github.com/seer-status/seer/packages/protos/gen/go/seer/private/v1"
	"github.com/seer-status/seer/packages/protos/gen/go/seer/private/v1/privatev1connect"
)

// Sessions serves the web app's session calls on the private port, behind
// auth.NewWebInterceptor.
type Sessions struct {
	Store *session.Store
}

var _ privatev1connect.SessionServiceHandler = (*Sessions)(nil)

func (s *Sessions) CreateSession(ctx context.Context, req *connect.Request[privatev1.CreateSessionRequest]) (*connect.Response[privatev1.CreateSessionResponse], error) {
	m := req.Msg
	if m.Subject == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("subject is required"))
	}
	u := session.User{Subject: m.Subject, Name: m.Name, Email: m.Email, Username: m.Username, Groups: m.Groups}
	id, sess, err := s.Store.Create(ctx, u, m.UserAgent, m.Ip)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.CreateSessionResponse{Id: id, Session: toSession(sess)}), nil
}

func (s *Sessions) GetSession(ctx context.Context, req *connect.Request[privatev1.GetSessionRequest]) (*connect.Response[privatev1.GetSessionResponse], error) {
	sess, err := s.Store.Active(ctx, req.Msg.Id)
	if errors.Is(err, session.ErrInvalid) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.GetSessionResponse{Session: toSession(sess)}), nil
}

func (s *Sessions) RevokeSession(ctx context.Context, req *connect.Request[privatev1.RevokeSessionRequest]) (*connect.Response[privatev1.RevokeSessionResponse], error) {
	if err := s.Store.Revoke(ctx, req.Msg.Id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.RevokeSessionResponse{}), nil
}

func toSession(s *ent.Session) *privatev1.Session {
	return &privatev1.Session{
		Subject:   s.Subject,
		Name:      s.Name,
		Groups:    s.Groups,
		ExpiresAt: timestamppb.New(s.ExpiresAt),
	}
}
