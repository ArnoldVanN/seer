package rpc

import (
	"context"
	"net/http"

	"github.com/seer-status/seer/apps/server/internal/config"
)

type siteKey struct{}

// SiteHeader lets the web app name the site directly. Browsers calling /rpc
// don't send it, so for them the site comes from the hostname.
const SiteHeader = "X-Seer-Site"

// WithSite resolves which site a request is for and stores it in the
// context. Sites scope what's listed, not who may see it: on the public port
// anyone can name any site, and private access is still decided by
// allowedGroups.
func WithSite(cfg *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := cfg.SiteByKey(r.Header.Get(SiteHeader))
		if site == nil {
			host := r.Header.Get("X-Forwarded-Host")
			if host == "" {
				host = r.Host
			}
			site = cfg.SiteForHost(host)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), siteKey{}, site)))
	})
}

// siteFrom falls back to the first site for callers that skipped WithSite
// (tests, mostly).
func (s *Service) siteFrom(ctx context.Context) *config.Site {
	if site, ok := ctx.Value(siteKey{}).(*config.Site); ok {
		return site
	}
	return &s.Config.Sites[0]
}
