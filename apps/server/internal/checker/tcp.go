package checker

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/seer-status/seer/apps/server/internal/condition"
	"github.com/seer-status/seer/apps/server/internal/config"
)

// hostPort strips a scheme like tcp:// or icmp:// if present.
func hostPort(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	_, rest, ok := strings.Cut(raw, "://")
	if ok {
		return rest
	}
	return raw
}

func probeTCP(ctx context.Context, e *config.Endpoint, out *condition.Context) error {
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", hostPort(e.URL))
	out.ResponseTime = time.Since(start)
	if err != nil {
		return err
	}
	defer conn.Close()
	out.Connected = true
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		out.IP = addr.IP.String()
	}
	return nil
}
