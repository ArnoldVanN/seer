// Package session keeps signed-in viewers' sessions in Postgres, so they can
// be revoked. The web app's cookie holds a random ID; only its
// SHA-256 is stored.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/seer-status/seer/apps/server/ent"
	entsession "github.com/seer-status/seer/apps/server/ent/session"
	"github.com/seer-status/seer/apps/server/internal/db"
)

const (
	// TTL is how long a session lasts from sign-in.
	TTL = 8 * time.Hour
	// How long a lookup is reused. A revoke reaches every API replica within
	// this, and open streams within the next WatchStatus refresh after it.
	cacheFor = 10 * time.Second
	// last_seen_at is only written this often, not on every call.
	touchEvery = 5 * time.Minute
	// Bounds the cache; past it, it starts over.
	maxCached = 10_000
	// Longer user agents are cut off.
	maxUserAgent = 512
)

// ErrInvalid means the session doesn't exist, expired or was revoked.
var ErrInvalid = errors.New("session expired or revoked")

type Store struct {
	db    *db.DB
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	s  *ent.Session
	at time.Time
}

func New(d *db.DB) *Store {
	return &Store{db: d, cache: map[string]cached{}}
}

// Hash is what the sessions table is keyed by.
func Hash(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// User is who a session is for. Email and Username are optional.
type User struct {
	Subject  string
	Name     string
	Email    string
	Username string
	Groups   []string
}

// Create starts a session and returns its ID, which only the caller sees.
func (s *Store) Create(ctx context.Context, u User, userAgent, ip string) (string, *ent.Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	if len(userAgent) > maxUserAgent {
		userAgent = userAgent[:maxUserAgent]
	}
	sess, err := s.db.Ent.Session.Create().
		SetID(Hash(id)).
		SetSubject(u.Subject).
		SetName(u.Name).
		SetEmail(u.Email).
		SetUsername(u.Username).
		SetGroups(u.Groups).
		SetUserAgent(userAgent).
		SetIP(ip).
		SetExpiresAt(time.Now().Add(TTL)).
		Save(ctx)
	if err != nil {
		return "", nil, err
	}
	return id, sess, nil
}

// Active returns the session for id, or ErrInvalid.
func (s *Store) Active(ctx context.Context, id string) (*ent.Session, error) {
	if id == "" {
		return nil, ErrInvalid
	}
	h := Hash(id)
	now := time.Now()
	s.mu.Lock()
	c, ok := s.cache[h]
	s.mu.Unlock()
	if ok && now.Sub(c.at) < cacheFor && now.Before(c.s.ExpiresAt) {
		return c.s, nil
	}

	sess, err := s.db.Ent.Session.Query().
		Where(entsession.ID(h), entsession.RevokedAtIsNil(), entsession.ExpiresAtGT(now)).
		Only(ctx)
	if ent.IsNotFound(err) {
		s.forget(h)
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if now.Sub(sess.LastSeenAt) > touchEvery {
		if err := s.db.Ent.Session.UpdateOneID(h).SetLastSeenAt(now).Exec(ctx); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	if len(s.cache) >= maxCached {
		s.cache = map[string]cached{}
	}
	s.cache[h] = cached{sess, now}
	s.mu.Unlock()
	return sess, nil
}

// Revoke ends the session for id. An unknown or already ended session is not
// an error.
func (s *Store) Revoke(ctx context.Context, id string) error {
	h := Hash(id)
	s.forget(h)
	return s.db.Ent.Session.Update().
		Where(entsession.ID(h), entsession.RevokedAtIsNil()).
		SetRevokedAt(time.Now()).
		Exec(ctx)
}

func (s *Store) forget(h string) {
	s.mu.Lock()
	delete(s.cache, h)
	s.mu.Unlock()
}
