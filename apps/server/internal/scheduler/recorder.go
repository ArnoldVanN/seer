package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/rs/zerolog/log"

	"github.com/seer-status/seer/apps/server/ent"
	"github.com/seer-status/seer/apps/server/ent/endpointstate"
	"github.com/seer-status/seer/apps/server/ent/result"
	"github.com/seer-status/seer/apps/server/internal/checker"
	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/db"
	"github.com/seer-status/seer/apps/server/internal/events"
	"github.com/seer-status/seer/apps/server/internal/notify"
)

// Recorder stores results and applies state transitions.
type Recorder struct {
	DB     *db.DB
	Notify *notify.Dispatcher
}

// Record stores out for slot and updates the endpoint's state. It returns
// false if another worker already stored this slot, in which case nothing
// else happens: the counters and alerts only move once per slot.
func (r *Recorder) Record(ctx context.Context, endpointID int, spec *config.Endpoint, slot int64, out checker.Outcome) (bool, error) {
	_, err := r.DB.Ent.Result.Create().
		SetEndpointID(endpointID).
		SetSlot(slot).
		SetCheckedAt(out.CheckedAt).
		SetSuccess(out.Success).
		SetResponseTimeMs(out.ResponseTime.Milliseconds()).
		SetError(truncate(out.Error, 1000)).
		SetConditions(out.Conditions).
		OnConflict(entsql.ConflictColumns(result.FieldEndpointID, result.FieldSlot)).
		DoNothing().
		ID(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert result: %w", err)
	}

	status, err := r.applyState(ctx, endpointID, spec, out)
	if err != nil {
		return true, err
	}
	if err := events.Publish(ctx, r.DB.SQL, events.Update{
		Key:            spec.Key,
		Status:         status,
		Success:        out.Success,
		CheckedAt:      out.CheckedAt,
		ResponseTimeMs: out.ResponseTime.Milliseconds(),
	}); err != nil {
		log.Warn().Err(err).Str("endpoint", spec.Key).Msg("publish event")
	}
	return true, nil
}

// applyState is an optimistic read-modify-write on the state row's version.
// Results for different slots can land concurrently during a handoff, so it
// retries on conflict. Whichever write wins queues that transition's events,
// in the same transaction.
func (r *Recorder) applyState(ctx context.Context, endpointID int, spec *config.Endpoint, out checker.Outcome) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		st, err := r.loadState(ctx, endpointID)
		if err != nil {
			return "", err
		}
		prev := state{
			Status:    string(st.Status),
			Failures:  st.ConsecutiveFailures,
			Successes: st.ConsecutiveSuccesses,
			Triggered: st.TriggeredAlerts,
		}
		n, evs := next(prev, out.Success, out.Error, spec, out.CheckedAt)

		won, err := r.saveState(ctx, st, n, prev.Status, out, endpointID, evs)
		if err != nil {
			return "", err
		}
		if won {
			if n.Status != prev.Status {
				log.Info().Str("endpoint", spec.Key).Str("from", prev.Status).Str("to", n.Status).Msg("status changed")
			}
			if len(evs) > 0 {
				r.Notify.Wake()
			}
			return n.Status, nil
		}
		time.Sleep(time.Duration(attempt*20) * time.Millisecond)
	}
	return "", errors.New("update state: too many concurrent writers")
}

// saveState writes n if st is still the current version, along with evs. It
// reports false if another writer got there first.
func (r *Recorder) saveState(ctx context.Context, st *ent.EndpointState, n state, prevStatus string, out checker.Outcome, endpointID int, evs []notify.Event) (bool, error) {
	tx, err := r.DB.Ent.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	upd := tx.EndpointState.Update().
		Where(endpointstate.ID(st.ID), endpointstate.Version(st.Version)).
		SetVersion(st.Version + 1).
		SetStatus(endpointstate.Status(n.Status)).
		SetConsecutiveFailures(n.Failures).
		SetConsecutiveSuccesses(n.Successes).
		SetTriggeredAlerts(n.Triggered).
		SetLastCheckedAt(out.CheckedAt).
		SetLastResponseTimeMs(out.ResponseTime.Milliseconds()).
		SetLastError(truncate(out.Error, 1000))
	if n.Status != prevStatus {
		upd.SetChangedAt(out.CheckedAt)
	}
	affected, err := upd.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("update state: %w", err)
	}
	if affected != 1 {
		return false, nil
	}
	if err := notify.Enqueue(ctx, tx, endpointID, evs); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("update state: %w", err)
	}
	return true, nil
}

func (r *Recorder) loadState(ctx context.Context, endpointID int) (*ent.EndpointState, error) {
	st, err := r.DB.Ent.EndpointState.Query().Where(endpointstate.EndpointID(endpointID)).Only(ctx)
	if ent.IsNotFound(err) {
		err = r.DB.Ent.EndpointState.Create().
			SetEndpointID(endpointID).
			OnConflict(entsql.ConflictColumns(endpointstate.FieldEndpointID)).
			DoNothing().
			Exec(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("create state: %w", err)
		}
		st, err = r.DB.Ent.EndpointState.Query().Where(endpointstate.EndpointID(endpointID)).Only(ctx)
	}
	return st, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
