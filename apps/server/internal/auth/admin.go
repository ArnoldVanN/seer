package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"

	"connectrpc.com/connect"
)

// AdminInterceptor admits calls that carry one of the admin API keys as a
// bearer token.
type AdminInterceptor struct {
	hashes [][32]byte
}

func NewAdminInterceptor(keys []string) *AdminInterceptor {
	i := &AdminInterceptor{}
	for _, k := range keys {
		i.hashes = append(i.hashes, sha256.Sum256([]byte(k)))
	}
	return i
}

// Comparing hashes keeps the time taken independent of the keys' contents
// and lengths. Every key is compared, so it doesn't depend on which matched.
func (i *AdminInterceptor) check(header string) error {
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || raw == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	got := sha256.Sum256([]byte(raw))
	match := 0
	for _, h := range i.hashes {
		match |= subtle.ConstantTimeCompare(got[:], h[:])
	}
	if match != 1 {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid admin API key"))
	}
	return nil
}

func (i *AdminInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.check(req.Header().Get("Authorization")); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *AdminInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *AdminInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.check(conn.RequestHeader().Get("Authorization")); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
