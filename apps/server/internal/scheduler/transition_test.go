package scheduler

import (
	"testing"
	"time"

	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/notify"
)

func TestNextThresholds(t *testing.T) {
	spec := &config.Endpoint{
		Key: "api", FailureThreshold: 2, SuccessThreshold: 1,
		Alerts: []config.Alert{{Type: "slack", FailureThreshold: 3, SuccessThreshold: 2}},
	}
	s := state{Status: "unknown"}
	var all []notify.Event
	step := func(ok bool) {
		var evs []notify.Event
		s, evs = next(s, ok, "boom", spec, time.Now())
		all = append(all, evs...)
	}

	step(true)
	if s.Status != "up" {
		t.Fatalf("status = %s after first success, want up", s.Status)
	}
	step(false)
	if s.Status != "up" {
		t.Fatalf("status = %s after 1 failure with threshold 2, want up", s.Status)
	}
	step(false)
	if s.Status != "down" {
		t.Fatalf("status = %s after 2 failures, want down", s.Status)
	}
	if len(all) != 0 {
		t.Fatalf("alert fired before its threshold: %+v", all)
	}
	step(false)
	step(false)
	if len(all) != 1 || all[0].Kind != notify.Triggered {
		t.Fatalf("want exactly one trigger after 4 failures, got %+v", all)
	}
	step(true)
	if s.Status != "up" || len(all) != 1 {
		t.Fatalf("resolved too early: status=%s events=%+v", s.Status, all)
	}
	step(true)
	if len(all) != 2 || all[1].Kind != notify.Resolved {
		t.Fatalf("want resolve after 2 successes, got %+v", all)
	}
	step(true)
	if len(all) != 2 {
		t.Fatalf("resolve fired twice: %+v", all)
	}
}
