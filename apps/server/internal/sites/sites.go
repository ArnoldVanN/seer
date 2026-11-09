// Package sites serves the sites stored in the DB, from config sync or the
// write API, to the request path. Sites are read on every request, so they
// are cached per replica and reloaded once the copy is a few seconds old.
package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ArnoldVanN/seer/apps/server/ent"
	"github.com/ArnoldVanN/seer/apps/server/ent/dashboard"
	"github.com/ArnoldVanN/seer/apps/server/ent/site"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/db"
)

// Site is a site as the request path sees it: dashboards resolved and the
// top-level UI merged in.
type Site struct {
	Key           string
	Hosts         []string
	Private       bool
	AllowedGroups []string
	// Dashboard keys in tab order.
	Dashboards []string
	UI         config.UI
}

// DefaultTTL is how stale a replica's copy of the sites may get before a
// request reloads it.
const DefaultTTL = 5 * time.Second

type Store struct {
	db *db.DB
	// Top-level ui from the config file, merged under every site's own.
	base config.UI
	ttl  time.Duration

	mu       sync.Mutex
	snap     *snapshot
	loadedAt time.Time
}

func New(d *db.DB, base config.UI) *Store {
	return &Store{db: d, base: base, ttl: DefaultTTL}
}

type snapshot struct {
	// By position: the first catches hosts no site lists.
	sites  []*Site
	byKey  map[string]*Site
	byHost map[string]*Site
}

// ForHost returns the site serving host, or the first site for an unknown
// host, so a single-site setup works on any hostname.
func (s *Store) ForHost(ctx context.Context, host string) (*Site, error) {
	snap, err := s.get(ctx)
	if err != nil {
		return nil, err
	}
	if st, ok := snap.byHost[config.NormalizeHost(host)]; ok {
		return st, nil
	}
	return snap.sites[0], nil
}

// ByKey returns the site with key, or nil.
func (s *Store) ByKey(ctx context.Context, key string) (*Site, error) {
	snap, err := s.get(ctx)
	if err != nil {
		return nil, err
	}
	return snap.byKey[key], nil
}

// HasHost reports whether a site lists host. Unlike ForHost, an unknown host
// doesn't count.
func (s *Store) HasHost(ctx context.Context, host string) (bool, error) {
	snap, err := s.get(ctx)
	if err != nil {
		return false, err
	}
	_, ok := snap.byHost[config.NormalizeHost(host)]
	return ok, nil
}

// AskHandler answers whether a TLS front end may get a certificate for a
// hostname: 200 if a site lists it, 404 if not. It follows Caddy's on-demand
// TLS "ask" contract, GET with ?domain=. A host added through the admin API
// can take up to DefaultTTL to be allowed on every replica.
func (s *Store) AskHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			http.Error(w, "domain is required", http.StatusBadRequest)
			return
		}
		ok, err := s.HasHost(r.Context(), domain)
		if err != nil {
			log.Error().Err(err).Msg("tls ask: load sites")
			http.Error(w, "site lookup failed", http.StatusServiceUnavailable)
			return
		}
		if !ok {
			http.Error(w, "no site has this host", http.StatusNotFound)
			return
		}
		w.Write([]byte("ok")) //nolint:errcheck
	})
}

// Default returns the site unknown hosts get.
func (s *Store) Default(ctx context.Context) (*Site, error) {
	snap, err := s.get(ctx)
	if err != nil {
		return nil, err
	}
	return snap.sites[0], nil
}

// Invalidate makes the next lookup reload, for writes made on this replica.
func (s *Store) Invalidate() {
	s.mu.Lock()
	s.loadedAt = time.Time{}
	s.mu.Unlock()
}

func (s *Store) get(ctx context.Context) (*snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snap != nil && time.Since(s.loadedAt) < s.ttl {
		return s.snap, nil
	}
	snap, err := s.load(ctx)
	if err != nil {
		if s.snap == nil {
			return nil, err
		}
		// Keep serving the last good copy; the next request tries again.
		log.Warn().Err(err).Msg("reload sites; serving the cached copy")
		return s.snap, nil
	}
	s.snap, s.loadedAt = snap, time.Now()
	return snap, nil
}

func (s *Store) load(ctx context.Context) (*snapshot, error) {
	rows, err := s.db.Ent.Site.Query().Order(ent.Asc(site.FieldPosition), ent.Asc(site.FieldKey)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load sites: %w", err)
	}
	var all []string
	needAll := len(rows) == 0 || slices.ContainsFunc(rows, func(r *ent.Site) bool { return r.AllDashboards })
	if needAll {
		all, err = s.db.Ent.Dashboard.Query().
			Order(ent.Asc(dashboard.FieldPosition), ent.Asc(dashboard.FieldKey)).
			Select(dashboard.FieldKey).Strings(ctx)
		if err != nil {
			return nil, fmt.Errorf("load dashboard keys: %w", err)
		}
	}

	snap := &snapshot{byKey: map[string]*Site{}, byHost: map[string]*Site{}}
	// Before the first config sync there are no rows yet.
	if len(rows) == 0 {
		st := &Site{Key: "default", Dashboards: all, UI: config.MergeUI(s.base, config.UI{})}
		snap.sites = []*Site{st}
		snap.byKey[st.Key] = st
		return snap, nil
	}
	for _, r := range rows {
		var own config.UI
		if err := json.Unmarshal(r.UI, &own); err != nil {
			return nil, fmt.Errorf("site %s: decode ui: %w", r.Key, err)
		}
		st := &Site{
			Key:           r.Key,
			Hosts:         r.Hosts,
			Private:       r.Private,
			AllowedGroups: r.AllowedGroups,
			Dashboards:    r.Dashboards,
			UI:            config.MergeUI(s.base, own),
		}
		if r.AllDashboards {
			st.Dashboards = all
		}
		snap.sites = append(snap.sites, st)
		snap.byKey[st.Key] = st
		for _, h := range st.Hosts {
			// Lower positions win a host listed twice.
			if _, ok := snap.byHost[h]; !ok {
				snap.byHost[h] = st
			}
		}
	}
	return snap, nil
}
