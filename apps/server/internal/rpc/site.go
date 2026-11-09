package rpc

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/ArnoldVanN/seer/apps/server/internal/sites"
)

type siteKey struct{}

// SiteHeader lets the web app name the site directly. Browsers calling /rpc
// don't send it, so for them the site comes from the hostname.
const SiteHeader = "X-Seer-Site"

// WithSite resolves which site a request is for and stores it in the
// context. Sites scope what's listed, not who may see it: on the public port
// anyone can name any site, and private access is still decided by
// allowedGroups.
func WithSite(st *sites.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site, err := st.ByKey(r.Context(), r.Header.Get(SiteHeader))
		if err == nil && site == nil {
			host := r.Header.Get("X-Forwarded-Host")
			if host == "" {
				host = r.Host
			}
			site, err = st.ForHost(r.Context(), host)
		}
		if err != nil {
			log.Error().Err(err).Msg("resolve site")
			http.Error(w, "site lookup failed", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), siteKey{}, site)))
	})
}

// siteFrom returns the request's site as currently stored, so a long-lived
// stream sees changes to it, and a removed site looks missing. Callers that
// skipped WithSite (tests, mostly) get the default site.
func (s *Service) siteFrom(ctx context.Context) (*sites.Site, error) {
	site, ok := ctx.Value(siteKey{}).(*sites.Site)
	if !ok {
		return s.Sites.Default(ctx)
	}
	cur, err := s.Sites.ByKey(ctx, site.Key)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, errNotFound
	}
	return cur, nil
}
