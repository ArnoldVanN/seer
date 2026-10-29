package checker

import (
	"context"
	"errors"
	"os"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/seer-status/seer/apps/server/internal/condition"
	"github.com/seer-status/seer/apps/server/internal/config"
)

// probeICMP sends one echo. It uses unprivileged (UDP) ping unless
// SEER_ICMP_PRIVILEGED=true, which needs CAP_NET_RAW.
func probeICMP(ctx context.Context, e *config.Endpoint, out *condition.Context) error {
	p, err := probing.NewPinger(hostPort(e.URL))
	if err != nil {
		return err
	}
	p.Count = 1
	p.SetPrivileged(os.Getenv("SEER_ICMP_PRIVILEGED") == "true")
	if deadline, ok := ctx.Deadline(); ok {
		p.Timeout = time.Until(deadline)
	}
	if err := p.RunWithContext(ctx); err != nil {
		return err
	}
	stats := p.Statistics()
	if stats.PacketsRecv == 0 {
		return errors.New("no echo reply")
	}
	out.Connected = true
	out.ResponseTime = stats.AvgRtt
	if stats.IPAddr != nil {
		out.IP = stats.IPAddr.IP.String()
	}
	return nil
}
