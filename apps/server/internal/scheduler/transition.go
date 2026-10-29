package scheduler

import (
	"time"

	"github.com/seer-status/seer/apps/server/internal/checker"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/notify"
)

// state is the part of EndpointState a result changes.
type state struct {
	Status    string
	Failures  int
	Successes int
	Triggered []bool
	ChangedAt time.Time
}

// next applies one result to s. It's pure so the threshold logic can be
// tested without a database; the caller persists the new state and, only if
// that write wins, sends the returned events.
func next(s state, out checker.Outcome, spec *config.Endpoint) (state, []notify.Event) {
	success, at := out.Success, out.CheckedAt
	n := state{Status: s.Status, Failures: s.Failures, Successes: s.Successes, ChangedAt: s.ChangedAt}
	n.Triggered = make([]bool, len(spec.Alerts))
	copy(n.Triggered, s.Triggered)

	if success {
		n.Successes++
		n.Failures = 0
		if n.Status != "up" && n.Successes >= spec.SuccessThreshold {
			n.Status = "up"
		}
	} else {
		n.Failures++
		n.Successes = 0
		if n.Status != "down" && n.Failures >= spec.FailureThreshold {
			n.Status = "down"
		}
	}
	if n.Status != s.Status {
		n.ChangedAt = at
	}

	var events []notify.Event
	for i, a := range spec.Alerts {
		ev := notify.Event{
			AlertType:    a.Type,
			EndpointKey:  spec.Key,
			EndpointName: spec.Name,
			Group:        spec.Group,
			Description:  a.Description,
			At:           at,
			URL:          spec.URL,
		}
		switch {
		case !n.Triggered[i] && !success && n.Failures >= a.FailureThreshold:
			n.Triggered[i] = true
			ev.Kind, ev.Error = notify.Triggered, out.Error
			ev.FailedConditions, ev.Failures = out.Failed, n.Failures
			events = append(events, ev)
		case n.Triggered[i] && success && n.Successes >= a.SuccessThreshold:
			n.Triggered[i] = false
			if a.ResolvedEnabled() {
				ev.Kind = notify.Resolved
				// Unknown if the endpoint came back up before this alert
				// resolved, as ChangedAt has moved on.
				if s.Status == "down" && !s.ChangedAt.IsZero() {
					ev.DownSince = &s.ChangedAt
				}
				events = append(events, ev)
			}
		}
	}
	return n, events
}
