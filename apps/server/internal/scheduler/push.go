package scheduler

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ArnoldVanN/seer/apps/server/ent"
	"github.com/ArnoldVanN/seer/apps/server/ent/result"
	"github.com/ArnoldVanN/seer/apps/server/internal/checker"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
)

// pushPoll caps how long a push watcher sleeps (or the endpoint's period, if
// shorter), so it notices pings and keeps its metrics current without being
// told about them.
const pushPoll = 30 * time.Second

// watchPush records a failure whenever a push endpoint goes interval+grace
// without a result. Pings are stored by the API (see internal/push); this
// only fills the silence. Until the first ping the endpoint stays unknown.
func (w *Worker) watchPush(ctx context.Context, endpointID int, spec *config.Endpoint) {
	period := spec.Interval.D() + spec.Grace.D()
	for {
		wait := min(pushPoll, period)
		last, err := w.DB.Ent.Result.Query().
			Where(result.EndpointID(endpointID)).
			Order(ent.Desc(result.FieldCheckedAt)).
			First(ctx)
		switch {
		case ent.IsNotFound(err):
		case err != nil:
			if ctx.Err() == nil {
				log.Error().Err(err).Str("endpoint", spec.Key).Msg("read last ping")
			}
		default:
			observe(spec, last.Success, time.Duration(last.ResponseTimeMs)*time.Millisecond)
			at, missed := missedAt(last.CheckedAt, time.Now(), period)
			if !missed {
				wait = min(wait, time.Until(at))
				break
			}
			out := checker.Outcome{CheckedAt: at, Error: "no ping for " + shortDuration(period)}
			stored, err := w.Recorder.Record(ctx, endpointID, spec, at.UnixMilli(), out)
			if err != nil {
				log.Error().Err(err).Str("endpoint", spec.Key).Msg("record missed ping")
				break
			}
			if stored {
				checksTotal.WithLabelValues(spec.Key, spec.Group, "false").Inc()
			}
			continue // read the new latest result for the next deadline
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// missedAt reports whether a push endpoint whose latest result is at last
// has missed its deadline by now, and returns that deadline (or the next
// one, if not missed). It depends only on last, so two workers owning the
// endpoint during a handoff pick the same slot and the unique key keeps one.
// After a long silence it jumps to the latest deadline passed rather than
// recording each one in between.
func missedAt(last, now time.Time, period time.Duration) (time.Time, bool) {
	k := now.Sub(last) / period
	if k < 1 {
		return last.Add(period), false
	}
	return last.Add(k * period), true
}

// shortDuration drops the zero units time.Duration.String leaves on round
// values: 24h0m0s becomes 24h.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

func observe(spec *config.Endpoint, success bool, rt time.Duration) {
	up := 0.0
	if success {
		up = 1
	}
	endpointUp.WithLabelValues(spec.Key, spec.Group).Set(up)
	endpointResponse.WithLabelValues(spec.Key, spec.Group).Set(rt.Seconds())
}
