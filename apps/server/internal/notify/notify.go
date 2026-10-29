// Package notify sends alert notifications. Events are queued in the
// notifications table, in the same transaction as the state change that
// caused them, which exactly one worker wins. Every worker then sends from
// that table, so an event goes out once and survives the worker that queued
// it dying.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
)

type Kind string

const (
	Triggered Kind = "triggered"
	Resolved  Kind = "resolved"
)

type Event struct {
	Kind         Kind      `json:"kind"`
	AlertType    string    `json:"alertType"`
	EndpointKey  string    `json:"endpointKey"`
	EndpointName string    `json:"endpointName"`
	Group        string    `json:"group,omitempty"`
	Description  string    `json:"description,omitempty"`
	Error        string    `json:"error,omitempty"`
	At           time.Time `json:"at"`
}

// Title is the one-line summary providers lead with. The group tells apart
// endpoints with the same name on different sites.
func (ev Event) Title() string {
	name := ev.EndpointName
	if ev.Group != "" {
		name = ev.Group + " / " + name
	}
	if ev.Kind == Resolved {
		return name + " is back up"
	}
	return name + " is down"
}

type Notifier interface {
	Notify(ctx context.Context, ev Event) error
}

const (
	timeout      = 10 * time.Second
	maxAttempts  = 10
	pollInterval = 5 * time.Second
)

// Backoff after a failed send: 10s, doubling up to 10m. Ten attempts span
// about 40 minutes, enough to ride out a short provider outage. A var so
// tests can shorten it.
var (
	firstBackoff = 10 * time.Second
	maxBackoff   = 10 * time.Minute
)

// Enqueue queues evs in tx. The caller commits it with the state change.
func Enqueue(ctx context.Context, tx *ent.Tx, endpointID int, evs []Event) error {
	for _, ev := range evs {
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if err := tx.Notification.Create().
			SetEndpointID(endpointID).
			SetAlertType(ev.AlertType).
			SetEvent(b).
			Exec(ctx); err != nil {
			return fmt.Errorf("queue notification: %w", err)
		}
	}
	return nil
}

// Dispatcher sends queued events through the providers this replica has
// configured.
type Dispatcher struct {
	db        *db.DB
	providers map[string]Notifier
	wake      chan struct{}
}

func New(d *db.DB, a config.Alerting) *Dispatcher {
	disp := &Dispatcher{db: d, providers: map[string]Notifier{}, wake: make(chan struct{}, 1)}
	if a.Webhook != nil {
		disp.Register("webhook", &webhook{cfg: *a.Webhook})
	}
	if a.Slack != nil {
		disp.Register("slack", &slack{url: a.Slack.WebhookURL})
	}
	if a.Discord != nil {
		disp.Register("discord", &discord{url: a.Discord.WebhookURL})
	}
	if a.Email != nil {
		disp.Register("email", &email{cfg: *a.Email})
	}
	return disp
}

func (d *Dispatcher) Register(alertType string, n Notifier) { d.providers[alertType] = n }

// Wake makes Run look for events now rather than at its next poll. Call it
// after committing new ones.
func (d *Dispatcher) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Run sends queued events until ctx is done. A send in progress finishes
// first.
func (d *Dispatcher) Run(ctx context.Context) {
	if len(d.providers) == 0 {
		return
	}
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		for ctx.Err() == nil {
			sent, err := d.sendNext(ctx)
			if err != nil {
				log.Error().Err(err).Msg("send notifications")
			}
			if !sent || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-d.wake:
		}
	}
}

// claimQuery locks the oldest due event this replica can send. Only the
// first queued event per endpoint and provider qualifies, so a resolve can't
// overtake its trigger while that's being retried or sent by another worker.
const claimQuery = `
SELECT id, alert_type, event, attempts FROM notifications n
WHERE next_attempt_at <= $1 AND alert_type = ANY($2)
  AND NOT EXISTS (
    SELECT 1 FROM notifications o
    WHERE o.endpoint_id = n.endpoint_id AND o.alert_type = n.alert_type AND o.id < n.id)
ORDER BY id LIMIT 1
FOR UPDATE SKIP LOCKED`

// sendNext sends one event, holding its row lock while it does: if this
// worker dies mid-send, the lock goes with its connection and another worker
// retries. It reports whether there was an event to send.
func (d *Dispatcher) sendNext(ctx context.Context) (bool, error) {
	// Not cancelled on shutdown, so the send and its bookkeeping finish.
	ctx = context.WithoutCancel(ctx)
	tx, err := d.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var (
		id       int64
		typ      string
		raw      []byte
		attempts int
	)
	types := make([]string, 0, len(d.providers))
	for t := range d.providers {
		types = append(types, t)
	}
	slices.Sort(types)
	err = tx.QueryRowContext(ctx, claimQuery, time.Now(), types).Scan(&id, &typ, &raw, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		log.Error().Err(err).Int64("id", id).Msg("dropping unreadable notification")
		if _, err := tx.ExecContext(ctx, `DELETE FROM notifications WHERE id = $1`, id); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}

	l := log.With().Str("type", typ).Str("kind", string(ev.Kind)).Str("endpoint", ev.EndpointKey).Logger()
	sctx, cancel := context.WithTimeout(ctx, timeout)
	sendErr := d.providers[typ].Notify(sctx, ev)
	cancel()
	attempts++
	switch {
	case sendErr == nil:
		l.Info().Msg("notification sent")
		_, err = tx.ExecContext(ctx, `DELETE FROM notifications WHERE id = $1`, id)
	case attempts >= maxAttempts:
		l.Error().Err(sendErr).Int("attempts", attempts).Msg("notification failed; giving up")
		_, err = tx.ExecContext(ctx, `DELETE FROM notifications WHERE id = $1`, id)
	default:
		wait := min(firstBackoff<<(attempts-1), maxBackoff)
		l.Warn().Err(sendErr).Int("attempt", attempts).Dur("retryIn", wait).Msg("notification failed")
		_, err = tx.ExecContext(ctx,
			`UPDATE notifications SET attempts = $2, next_attempt_at = $3, last_error = $4 WHERE id = $1`,
			id, attempts, time.Now().Add(wait), truncate(sendErr.Error(), 1000))
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
