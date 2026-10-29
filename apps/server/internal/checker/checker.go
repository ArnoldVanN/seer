// Package checker runs one check for an endpoint and evaluates its conditions.
package checker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/seer-status/seer/apps/server/ent/schema"
	"github.com/seer-status/seer/apps/server/internal/condition"
	"github.com/seer-status/seer/apps/server/internal/config"
)

// Outcome is a finished check, ready to be stored.
type Outcome struct {
	CheckedAt    time.Time
	Success      bool
	ResponseTime time.Duration
	Error        string
	Conditions   []schema.ConditionResult
}

// probe fills in what it observed. An error means the target couldn't be
// reached at all; conditions still run (e.g. "[CONNECTED] == false").
type probe func(ctx context.Context, e *config.Endpoint, out *condition.Context) error

var probes = map[string]probe{
	"http": probeHTTP,
	"tcp":  probeTCP,
	"dns":  probeDNS,
	"icmp": probeICMP,
}

func Run(ctx context.Context, e *config.Endpoint) Outcome {
	start := time.Now()
	p, ok := probes[e.Type]
	if !ok {
		return Outcome{CheckedAt: start, Error: fmt.Sprintf("unknown endpoint type %q", e.Type)}
	}

	ctx, cancel := context.WithTimeout(ctx, e.Timeout.D())
	defer cancel()

	var cctx condition.Context
	probeErr := p(ctx, e, &cctx)
	if cctx.ResponseTime == 0 {
		cctx.ResponseTime = time.Since(start)
	}

	out := Outcome{CheckedAt: start, ResponseTime: cctx.ResponseTime, Success: true}
	var errs []error
	if probeErr != nil {
		errs = append(errs, probeErr)
	}
	for _, c := range e.Conditions {
		ok, err := condition.Evaluate(c, &cctx)
		if err != nil {
			errs = append(errs, err)
		}
		out.Conditions = append(out.Conditions, schema.ConditionResult{Condition: c, Success: ok})
		if !ok {
			out.Success = false
		}
	}
	if err := errors.Join(errs...); err != nil {
		out.Error = err.Error()
		// A connection error only fails the check through the conditions, so
		// "[CONNECTED] == false" endpoints can pass.
	}
	return out
}
