// Package auth verifies the short-lived tokens the web app attaches to calls
// on the private port. The web app handles the actual login (OIDC or the
// admin account) and mints these with the shared internal secret.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
)

const Audience = "seer-private"

type Viewer struct {
	Subject string
	Groups  []string
}

type claims struct {
	Groups []string `json:"groups"`
	jwt.RegisteredClaims
}

type ctxKey struct{}

func FromContext(ctx context.Context) (Viewer, bool) {
	v, ok := ctx.Value(ctxKey{}).(Viewer)
	return v, ok
}

// Interceptor rejects private-port calls without a valid token.
type Interceptor struct {
	secret []byte
}

func NewInterceptor(secret string) *Interceptor {
	return &Interceptor{secret: []byte(secret)}
}

func (i *Interceptor) verify(ctx context.Context, header string) (context.Context, error) {
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithAudience(Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if c.Subject == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("token has no subject"))
	}
	return context.WithValue(ctx, ctxKey{}, Viewer{Subject: c.Subject, Groups: c.Groups}), nil
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

// Mint signs a token. The web app does this in TypeScript; it's here for
// tests and tooling.
func Mint(secret, subject string, groups []string, ttl time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Groups: groups,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Audience:  jwt.ClaimStrings{Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}).SignedString([]byte(secret))
}
