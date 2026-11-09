// Package auth verifies the short-lived tokens the web app attaches to calls
// on the private port. The web app handles the actual login (OIDC or the
// admin account), starts a session through the session service and mints
// these with the shared internal secret.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ArnoldVanN/seer/apps/server/internal/session"
)

const (
	// Audience of tokens for a viewer's calls. They name a session, and the
	// viewer's groups come from it.
	Audience = "seer-private"
	// WebAudience is for the web app's own calls to the session service.
	WebAudience = "seer-web"
)

type Viewer struct {
	Subject string
	Groups  []string
	// The session the call was made in, to check it again on long streams.
	SessionID string
}

type claims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

type ctxKey struct{}

func FromContext(ctx context.Context) (Viewer, bool) {
	v, ok := ctx.Value(ctxKey{}).(Viewer)
	return v, ok
}

// Interceptor rejects private-port calls without a valid token.
type Interceptor struct {
	verify func(ctx context.Context, header string) (context.Context, error)
}

// NewInterceptor accepts viewer tokens whose session is still active.
func NewInterceptor(secret string, sessions *session.Store) *Interceptor {
	return &Interceptor{verify: func(ctx context.Context, header string) (context.Context, error) {
		c, err := parse(secret, Audience, header)
		if err != nil {
			return nil, err
		}
		if c.SessionID == "" {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("token has no session"))
		}
		s, err := sessions.Active(ctx, c.SessionID)
		if errors.Is(err, session.ErrInvalid) {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}
		if err != nil {
			return nil, connect.NewError(connect.CodeUnavailable, err)
		}
		return context.WithValue(ctx, ctxKey{}, Viewer{Subject: s.Subject, Groups: s.Groups, SessionID: c.SessionID}), nil
	}}
}

// NewWebInterceptor accepts the web app's own tokens.
func NewWebInterceptor(secret string) *Interceptor {
	return &Interceptor{verify: func(ctx context.Context, header string) (context.Context, error) {
		if _, err := parse(secret, WebAudience, header); err != nil {
			return nil, err
		}
		return ctx, nil
	}}
}

func parse(secret, audience, header string) (*claims, error) {
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	return &c, nil
}

func (i *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := i.verify(ctx, req.Header().Get("Authorization"))
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *Interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *Interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := i.verify(ctx, conn.RequestHeader().Get("Authorization"))
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// Mint signs a viewer token for a session, and MintWeb one of the web app's
// own. The web app does this in TypeScript; these are for tests and tooling.
func Mint(secret, sessionID string, ttl time.Duration) (string, error) {
	return mint(secret, Audience, "viewer", sessionID, ttl)
}

func MintWeb(secret string, ttl time.Duration) (string, error) {
	return mint(secret, WebAudience, "web", "", ttl)
}

func mint(secret, audience, subject, sessionID string, ttl time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}).SignedString([]byte(secret))
}
