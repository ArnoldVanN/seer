// Package events fans out result updates to every API replica over Postgres
// LISTEN/NOTIFY, so a client streaming from any replica sees results written
// by any worker.
package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const channel = "seer_results"

type Update struct {
	Key            string    `json:"k"`
	Status         string    `json:"s"`
	Success        bool      `json:"ok"`
	CheckedAt      time.Time `json:"t"`
	ResponseTimeMs int64     `json:"rt"`
}

// Publish notifies all listeners. Delivery is best effort: a dropped update is
// corrected by the next one or by the client's snapshot refresh.
func Publish(ctx context.Context, db *sql.DB, u Update) error {
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, string(b))
	return err
}

// Bus holds one LISTEN connection per replica and broadcasts to subscribers.
type Bus struct {
	dsn  string
	mu   sync.Mutex
	subs map[chan Update]struct{}
}

func NewBus(dsn string) *Bus {
	return &Bus{dsn: dsn, subs: map[chan Update]struct{}{}}
}

// Subscribe returns a channel of updates and a func to stop receiving them.
func (b *Bus) Subscribe() (<-chan Update, func()) {
	ch := make(chan Update, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

func (b *Bus) broadcast(u Update) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- u:
		default: // slow subscriber; it catches up on the next update
		}
	}
}

// Run listens until ctx is done, reconnecting with backoff.
func (b *Bus) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := b.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Warn().Err(err).Dur("retry_in", backoff).Msg("event listener disconnected")
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (b *Bus) listen(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, b.dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return err
	}
	log.Info().Msg("listening for result events")
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var u Update
		if err := json.Unmarshal([]byte(n.Payload), &u); err != nil {
			log.Warn().Err(err).Msg("bad event payload")
			continue
		}
		b.broadcast(u)
	}
}
