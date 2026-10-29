package config

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Site is a status page on its own hostnames, with its own branding and set
// of dashboards. One deployment can serve several; endpoints are shared, so
// an endpoint shown on two sites is still checked once.
type Site struct {
	Key   string   `yaml:"key"`
	Hosts []string `yaml:"hosts"`
	// Overrides the top-level ui field by field. customCss is appended to the
	// top-level CSS rather than replacing it.
	UI UI `yaml:"ui"`
	// The site's dashboards, in tab order: definitions owned by this site, or
	// keys of dashboards defined elsewhere ("*" for every dashboard). Public
	// ones are on the site's root, private ones under the private path prefix.
	DashboardEntries []SiteDashboard `yaml:"dashboards"`
	// Endpoints owned by this site. Its dashboards match groups among these
	// only, so two sites can each have a group with the same name.
	Endpoints []Endpoint `yaml:"endpoints"`

	// Resolved dashboard keys, in order.
	Dashboards []string `yaml:"-"`
	// Private sites need sign-in for everything, public dashboards included,
	// e.g. an admin view of every site's dashboards on one internal host.
	Private bool `yaml:"private"`
	// Private sites only: groups that may use the site. Empty means any
	// signed-in user. Dashboards' own allowedGroups still apply.
	AllowedGroups []string `yaml:"allowedGroups"`
}

// SiteDashboard is a site's dashboards entry: a key, or a full definition.
type SiteDashboard struct {
	Key string
	Def *Dashboard
}

func (s *SiteDashboard) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		s.Key = n.Value
		return nil
	}
	// Node.Decode doesn't reject unknown fields, so decode through a
	// strict decoder like the rest of the file.
	raw, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var d Dashboard
	if err := dec.Decode(&d); err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	s.Def = &d
	return nil
}

// flattenSites moves site-owned endpoints and dashboards into the top-level
// lists, marked with their site, so the rest of seer sees one list of each.
// It runs before defaults and validation.
func (c *Config) flattenSites() {
	for i := range c.Sites {
		s := &c.Sites[i]
		for _, e := range s.Endpoints {
			e.Site = s.Key
			c.Endpoints = append(c.Endpoints, e)
		}
		s.Endpoints = nil
		s.Dashboards = nil
		for _, entry := range s.DashboardEntries {
			if entry.Def == nil {
				s.Dashboards = append(s.Dashboards, entry.Key)
				continue
			}
			d := *entry.Def
			d.Site = s.Key
			c.Dashboards = append(c.Dashboards, d)
			s.Dashboards = append(s.Dashboards, d.Key)
		}
	}
}

// Shows reports whether a dashboard includes an endpoint: listed by key from
// anywhere, or in one of its groups and owned by the same site (top-level
// dashboards match top-level endpoints).
func (c *Config) Shows(d *Dashboard, e *Endpoint) bool {
	if slices.Contains(d.Endpoints, e.Key) {
		return true
	}
	return slices.Contains(d.Groups, e.Group) && e.Site == d.Site
}

func (c *Config) EndpointByKey(key string) *Endpoint { return c.endpointIndex[key] }

// where names an endpoint or dashboard in error messages by where it's
// defined, since site-owned ones sit at the end of the flattened lists.
func where(kind string, i int, key, site string) string {
	if site != "" {
		return fmt.Sprintf("sites.%s %s %q", site, kind, key)
	}
	return fmt.Sprintf("%s[%d] (%s)", kind, i, key)
}

func (d *Dashboard) where(i int) string { return where("dashboards", i, d.Key, d.Site) }
func (e *Endpoint) where(i int) string  { return where("endpoints", i, e.Key, e.Site) }

// SiteForHost returns the site serving a hostname. Unknown hosts get the
// first site, so a single-site setup works on any hostname, including
// localhost and in-cluster names.
func (c *Config) SiteForHost(host string) *Site {
	host = normalizeHost(host)
	for i := range c.Sites {
		if slices.Contains(c.Sites[i].Hosts, host) {
			return &c.Sites[i]
		}
	}
	return &c.Sites[0]
}

func (c *Config) SiteByKey(key string) *Site {
	for i := range c.Sites {
		if c.Sites[i].Key == key {
			return &c.Sites[i]
		}
	}
	return nil
}

func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

// applySiteDefaults runs after dashboard defaults. Without sites, everything
// is one implicit site on every hostname, using the top-level ui.
func (c *Config) applySiteDefaults() {
	all := make([]string, len(c.Dashboards))
	for i, d := range c.Dashboards {
		all[i] = d.Key
	}
	if len(c.Sites) == 0 {
		c.Sites = []Site{{Key: "default", Dashboards: all}}
	}
	for i := range c.Sites {
		s := &c.Sites[i]
		if len(s.DashboardEntries) == 1 && s.DashboardEntries[0].Key == "*" {
			s.Dashboards = all
		}
		for j, h := range s.Hosts {
			s.Hosts[j] = normalizeHost(h)
		}
		s.UI = mergeUI(c.UI, s.UI)
	}
}

func mergeUI(base, over UI) UI {
	pick := func(b, o string) string {
		if o != "" {
			return o
		}
		return b
	}
	return UI{
		Title:       pick(base.Title, over.Title),
		Description: pick(base.Description, over.Description),
		Logo:        pick(base.Logo, over.Logo),
		LogoDark:    pick(base.LogoDark, over.LogoDark),
		HomeURL:     pick(base.HomeURL, over.HomeURL),
		Favicon:     pick(base.Favicon, over.Favicon),
		// Resolved into CustomCSS by loadSiteCSS.
		CustomCSS:     over.CustomCSS,
		CustomCSSFile: over.CustomCSSFile,
	}
}

// loadSiteCSS gives each site the top-level CSS followed by its own, and
// each dashboard its own, reading customCssFile references.
func (c *Config) loadSiteCSS() error {
	base, err := readCSS(c.UI, "ui")
	if err != nil {
		return err
	}
	c.UI.CustomCSS = base
	for i := range c.Sites {
		s := &c.Sites[i]
		own, err := readCSS(s.UI, fmt.Sprintf("sites[%d].ui", i))
		if err != nil {
			return err
		}
		s.UI.CustomCSS = strings.TrimSpace(base + "\n" + own)
	}
	for i := range c.Dashboards {
		d := &c.Dashboards[i]
		css, err := readCSS(UI{CustomCSS: d.CustomCSS, CustomCSSFile: d.CustomCSSFile}, d.where(i))
		if err != nil {
			return err
		}
		d.CustomCSS = strings.TrimSpace(css)
	}
	return nil
}

func readCSS(ui UI, where string) (string, error) {
	css := ui.CustomCSS
	if ui.CustomCSSFile != "" {
		b, err := os.ReadFile(ui.CustomCSSFile)
		if err != nil {
			return "", fmt.Errorf("%s.customCssFile: %w", where, err)
		}
		css += "\n" + string(b)
	}
	return css, nil
}

func (c *Config) validateSites(dashboards map[string]bool) []error {
	var errs []error
	keys := map[string]bool{}
	hosts := map[string]string{}
	onSite := map[string]bool{}
	for i, s := range c.Sites {
		where := fmt.Sprintf("sites[%d] (%s)", i, s.Key)
		if !keyPattern.MatchString(s.Key) {
			errs = append(errs, fmt.Errorf("%s: key must match %s", where, keyPattern))
		}
		if keys[s.Key] {
			errs = append(errs, fmt.Errorf("%s: duplicate key", where))
		}
		keys[s.Key] = true
		// Every site but the first needs hosts: it's how requests reach it.
		if i > 0 && len(s.Hosts) == 0 {
			errs = append(errs, fmt.Errorf("%s: hosts is required (only the first site catches unknown hosts)", where))
		}
		for _, h := range s.Hosts {
			if other, ok := hosts[h]; ok {
				errs = append(errs, fmt.Errorf("%s: host %q is also on site %s", where, h, other))
			}
			hosts[h] = s.Key
		}
		if len(s.Dashboards) == 0 {
			errs = append(errs, fmt.Errorf("%s: dashboards is required", where))
		}
		for _, e := range s.DashboardEntries {
			if e.Def == nil && e.Key == "*" && len(s.DashboardEntries) > 1 {
				errs = append(errs, fmt.Errorf(`%s: "*" can't be combined with other dashboards`, where))
			}
		}
		if len(s.AllowedGroups) > 0 && !s.Private {
			errs = append(errs, fmt.Errorf("%s: allowedGroups needs private: true", where))
		}
		for _, d := range s.Dashboards {
			if !dashboards[d] {
				errs = append(errs, fmt.Errorf("%s: unknown dashboard %q", where, d))
			}
			onSite[d] = true
		}
	}
	for _, d := range c.Dashboards {
		if !onSite[d.Key] {
			errs = append(errs, fmt.Errorf("dashboard %q isn't on any site", d.Key))
		}
	}
	return errs
}
