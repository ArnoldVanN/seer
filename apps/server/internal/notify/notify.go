// Package notify sends alert notifications. Dispatch is called from the state
// transition that exactly one worker wins, so each event goes out once across
// the whole cluster.
package notify

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

type Kind string

const (
	Triggered Kind = "triggered"
	Resolved  Kind = "resolved"
)

type Event struct {
	Kind         Kind
	AlertType    string
	EndpointKey  string
	EndpointName string
	Description  string
	Error        string
	At           time.Time
}

type Notifier interface {
	Notify(ctx context.Context, ev Event) error
}

// Dispatcher routes an event to the notifier registered for its alert type.
type Dispatcher struct {
	providers map[string]Notifier
	fallback  Notifier
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{providers: map[string]Notifier{}, fallback: unimplemented{}}
}

func (d *Dispatcher) Register(alertType string, n Notifier) { d.providers[alertType] = n }

func (d *Dispatcher) Dispatch(ctx context.Context, ev Event) {
	n, ok := d.providers[ev.AlertType]
	if !ok {
		n = d.fallback
	}
	if err := n.Notify(ctx, ev); err != nil {
		log.Error().Err(err).Str("type", ev.AlertType).Str("endpoint", ev.EndpointKey).Msg("notification failed")
	}
}

// unimplemented stands in for every provider until real ones are written.
type unimplemented struct{}

func (unimplemented) Notify(_ context.Context, ev Event) error {
	log.Warn().
		Str("type", ev.AlertType).
		Str("kind", string(ev.Kind)).
		Str("endpoint", ev.EndpointKey).
		Msg("notification provider not implemented")
	return nil
}
