package rpc

import (
	"testing"

	"github.com/ArnoldVanN/seer/apps/server/ent"
)

func TestShows(t *testing.T) {
	dashboards := map[string]*ent.Dashboard{
		"home":  {Key: "home", Site: "home", Groups: []string{"Sites"}, EndpointKeys: []string{"db"}},
		"acme":  {Key: "acme", Site: "acme", Groups: []string{"Sites"}},
		"infra": {Key: "infra", Groups: []string{"infra"}},
	}
	endpoints := map[string]*ent.Endpoint{
		"portfolio": {Key: "portfolio", Site: "home", GroupName: "Sites"},
		"acme-web":  {Key: "acme-web", Site: "acme", GroupName: "Sites"},
		"db":        {Key: "db", GroupName: "infra"},
	}
	for _, tc := range []struct {
		dash, ep string
		want     bool
	}{
		{"home", "portfolio", true},
		{"home", "acme-web", false}, // same group name, other site
		{"acme", "acme-web", true},
		{"acme", "portfolio", false},
		{"home", "db", true},  // listed by key
		{"infra", "db", true}, // top-level group, top-level endpoint
		{"infra", "portfolio", false},
	} {
		if got := shows(dashboards[tc.dash], endpoints[tc.ep]); got != tc.want {
			t.Errorf("shows(%s, %s) = %v, want %v", tc.dash, tc.ep, got, tc.want)
		}
	}
}
