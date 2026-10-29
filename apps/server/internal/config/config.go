// Package config loads seer's YAML config. ${VAR} references are expanded from
// the environment before parsing, so secrets can stay out of the file.
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Storage    Storage     `yaml:"storage"`
	Server     Server      `yaml:"server"`
	UI         UI          `yaml:"ui"`
	Alerting   Alerting    `yaml:"alerting"`
	Sites      []Site      `yaml:"sites"`
	Dashboards []Dashboard `yaml:"dashboards"`
	Endpoints  []Endpoint  `yaml:"endpoints"`

	// Hash of the endpoint and dashboard definitions, used by config sync.
	Hash string `yaml:"-"`
	// Raw is the expanded file contents, served to the UI as needed.
	Raw []byte `yaml:"-"`

	dashboardIndex map[string]*Dashboard
	endpointIndex  map[string]*Endpoint
}

type Storage struct {
	Type      string    `yaml:"type"`
	DSN       string    `yaml:"dsn"`
	Retention Retention `yaml:"retention"`
}

// Retention is how long each tier is kept. Zero means forever.
type Retention struct {
	Raw      Duration `yaml:"raw"`
	Rollup5m Duration `yaml:"rollup5m"`
	Rollup1h Duration `yaml:"rollup1h"`
	Rollup1d Duration `yaml:"rollup1d"`
}

type Server struct {
	PublicPort  int `yaml:"publicPort"`
	PrivatePort int `yaml:"privatePort"`
	OpsPort     int `yaml:"opsPort"`
	// Origins allowed to call the public port from a browser. Empty allows any,
	// which is fine since the public port only serves public data.
	CORSOrigins []string `yaml:"corsOrigins"`
	// Shared HS256 secret the web app signs private-port tokens with.
	InternalJWTSecret string `yaml:"internalJwtSecret"`
}

type UI struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	// Logo URL. Empty uses the Seer logo; "none" hides it.
	Logo string `yaml:"logo"`
	// Dark-mode logo URL. Empty uses Logo.
	LogoDark string `yaml:"logoDark"`
	// Where the logo links to, e.g. the main site. Empty links to the status page.
	HomeURL       string `yaml:"homeUrl"`
	Favicon       string `yaml:"favicon"`
	CustomCSS     string `yaml:"customCss"`
	CustomCSSFile string `yaml:"customCssFile"`
	// Footer text and links, shown above the "Powered by Seer" credit.
	Footer      string `yaml:"footer"`
	FooterLinks []Link `yaml:"footerLinks"`
	// Pointer so a site can turn the credit back on.
	HidePoweredBy *bool `yaml:"hidePoweredBy"`
}

// Alerting configures where alerts are sent. An endpoint alert's type names
// one of these providers.
type Alerting struct {
	Webhook *Webhook `yaml:"webhook"`
	Slack   *Chat    `yaml:"slack"`
	Discord *Chat    `yaml:"discord"`
	Email   *Email   `yaml:"email"`
}

// Webhook POSTs each event as JSON.
type Webhook struct {
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
}

// Chat is a Slack or Discord incoming webhook.
type Chat struct {
	WebhookURL string `yaml:"webhookUrl"`
}

type Email struct {
	Host string `yaml:"host"`
	// 465 uses implicit TLS; any other port upgrades with STARTTLS when the
	// server offers it.
	Port     int      `yaml:"port"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
}

// AlertTypes are the provider names an alert's type can take.
var AlertTypes = []string{"webhook", "slack", "discord", "email"}

// Configured reports whether the provider for an alert type is set up.
func (a Alerting) Configured(alertType string) bool {
	switch alertType {
	case "webhook":
		return a.Webhook != nil
	case "slack":
		return a.Slack != nil
	case "discord":
		return a.Discord != nil
	case "email":
		return a.Email != nil
	}
	return false
}

type Link struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

type Dashboard struct {
	Key           string   `yaml:"key" json:"key"`
	Title         string   `yaml:"title" json:"title"`
	Description   string   `yaml:"description" json:"description,omitempty"`
	Visibility    string   `yaml:"visibility" json:"visibility"`
	Groups        []string `yaml:"groups" json:"groups"`
	Endpoints     []string `yaml:"endpoints" json:"endpoints"`
	AllowedGroups []string `yaml:"allowedGroups" json:"allowedGroups"`
	// History range shown when the URL doesn't pick one: 24h, 7d, 30d, 90d or 1y.
	DefaultRange string `yaml:"defaultRange" json:"defaultRange,omitempty"`
	// CSS for this dashboard's page, after the site's. Served from config,
	// not the database.
	CustomCSS     string `yaml:"customCss" json:"customCss,omitempty"`
	CustomCSSFile string `yaml:"customCssFile" json:"customCssFile,omitempty"`

	// Key of the site that defines it; empty when defined at the top level.
	Site string `yaml:"-" json:"-"`
}

func (c *Config) DashboardByKey(key string) *Dashboard { return c.dashboardIndex[key] }

type Endpoint struct {
	Key   string `yaml:"key" json:"key"`
	Name  string `yaml:"name" json:"name"`
	Group string `yaml:"group" json:"group"`
	// Shown under the name on dashboards and on the endpoint's page.
	Description string `yaml:"description" json:"description,omitempty"`
	Links       []Link `yaml:"links" json:"links,omitempty"`
	// http, tcp, dns or icmp. Inferred from URL when empty.
	Type     string   `yaml:"type" json:"type"`
	URL      string   `yaml:"url" json:"url"`
	Interval Duration `yaml:"interval" json:"interval"`
	Timeout  Duration `yaml:"timeout" json:"timeout"`
	Enabled  *bool    `yaml:"enabled" json:"enabled,omitempty"`

	Method  string            `yaml:"method" json:"method,omitempty"`
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`
	Body    string            `yaml:"body" json:"body,omitempty"`
	// Skip TLS verification for self-signed internal services.
	Insecure bool `yaml:"insecure" json:"insecure,omitempty"`

	DNS *DNSQuery `yaml:"dns" json:"dns,omitempty"`

	Conditions []string `yaml:"conditions" json:"conditions"`

	// Consecutive results needed to flip the status.
	FailureThreshold int `yaml:"failureThreshold" json:"failureThreshold"`
	SuccessThreshold int `yaml:"successThreshold" json:"successThreshold"`

	Alerts []Alert `yaml:"alerts" json:"alerts,omitempty"`

	// Key of the site that defines it; empty when defined at the top level.
	Site string `yaml:"-" json:"-"`
}

type DNSQuery struct {
	Name string `yaml:"name" json:"name"`
	Type string `yaml:"type" json:"type"`
}

type Alert struct {
	Type             string `yaml:"type" json:"type"`
	FailureThreshold int    `yaml:"failureThreshold" json:"failureThreshold"`
	SuccessThreshold int    `yaml:"successThreshold" json:"successThreshold"`
	SendOnResolved   *bool  `yaml:"sendOnResolved" json:"sendOnResolved,omitempty"`
	Description      string `yaml:"description" json:"description,omitempty"`
}

func (e Endpoint) IsEnabled() bool { return e.Enabled == nil || *e.Enabled }

func (a Alert) ResolvedEnabled() bool { return a.SendOnResolved == nil || *a.SendOnResolved }

// Duration accepts Go duration strings ("30s") plus d, w and y suffixes
// ("14d", "2y") for retention periods. A year is 365 days.
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	v, err := ParseDuration(strings.Trim(string(b), `"`))
	*d = Duration(v)
	return err
}

var longUnits = map[string]time.Duration{
	"d": 24 * time.Hour,
	"w": 7 * 24 * time.Hour,
	"y": 365 * 24 * time.Hour,
}

func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	for suffix, unit := range longUnits {
		if n, ok := strings.CutSuffix(s, suffix); ok {
			if v, err := strconv.Atoi(n); err == nil {
				return time.Duration(v) * unit, nil
			}
		}
	}
	return time.ParseDuration(s)
}

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Ranges are the history ranges the UI offers.
var Ranges = []string{"24h", "7d", "30d", "90d", "1y"}

// Load reads, expands, defaults and validates the config at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Config, error) {
	expanded := []byte(os.ExpandEnv(string(raw)))

	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.Raw = expanded

	c.flattenSites()
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	if err := c.loadSiteCSS(); err != nil {
		return nil, err
	}

	c.dashboardIndex = make(map[string]*Dashboard, len(c.Dashboards))
	for i := range c.Dashboards {
		c.dashboardIndex[c.Dashboards[i].Key] = &c.Dashboards[i]
	}
	c.endpointIndex = make(map[string]*Endpoint, len(c.Endpoints))
	for i := range c.Endpoints {
		c.endpointIndex[c.Endpoints[i].Key] = &c.Endpoints[i]
	}

	h, err := c.definitionHash()
	if err != nil {
		return nil, err
	}
	c.Hash = h
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Storage.Type == "" {
		c.Storage.Type = "postgres"
	}
	if w := c.Alerting.Webhook; w != nil && w.Method == "" {
		w.Method = "POST"
	}
	if m := c.Alerting.Email; m != nil && m.Port == 0 {
		m.Port = 587
	}
	if c.Storage.DSN == "" {
		c.Storage.DSN = os.Getenv("SEER_DB_DSN")
	}
	r := &c.Storage.Retention
	if r.Raw == 0 {
		r.Raw = Duration(14 * 24 * time.Hour)
	}
	if r.Rollup5m == 0 {
		r.Rollup5m = Duration(90 * 24 * time.Hour)
	}
	if r.Rollup1h == 0 {
		r.Rollup1h = Duration(2 * 365 * 24 * time.Hour)
	}
	// Rollup1d stays 0: kept forever.

	// Env overrides the file, so the same config runs next to other local
	// services or on remapped ports.
	envPort(&c.Server.PublicPort, "SEER_PUBLIC_PORT")
	envPort(&c.Server.PrivatePort, "SEER_PRIVATE_PORT")
	envPort(&c.Server.OpsPort, "SEER_OPS_PORT")
	if c.Server.PublicPort == 0 {
		c.Server.PublicPort = 8080
	}
	if c.Server.PrivatePort == 0 {
		c.Server.PrivatePort = 8081
	}
	if c.Server.OpsPort == 0 {
		c.Server.OpsPort = 9090
	}
	if c.Server.InternalJWTSecret == "" {
		c.Server.InternalJWTSecret = os.Getenv("SEER_INTERNAL_JWT_SECRET")
	}

	if c.UI.Title == "" {
		c.UI.Title = "Status"
	}

	for i := range c.Endpoints {
		e := &c.Endpoints[i]
		if e.Name == "" {
			e.Name = e.Key
		}
		if e.Type == "" {
			e.Type = inferType(e.URL)
		}
		if e.Interval == 0 {
			e.Interval = Duration(60 * time.Second)
		}
		if e.Timeout == 0 {
			e.Timeout = Duration(10 * time.Second)
		}
		if e.Method == "" && e.Type == "http" {
			e.Method = "GET"
		}
		if e.FailureThreshold == 0 {
			e.FailureThreshold = 1
		}
		if e.SuccessThreshold == 0 {
			e.SuccessThreshold = 1
		}
		for j := range e.Alerts {
			a := &e.Alerts[j]
			if a.FailureThreshold == 0 {
				a.FailureThreshold = 3
			}
			if a.SuccessThreshold == 0 {
				a.SuccessThreshold = 2
			}
		}
	}

	// With no dashboards configured, everything goes on one public page, which
	// is what a Gatus user would expect.
	if len(c.Dashboards) == 0 {
		groups := []string{}
		seen := map[string]bool{}
		for _, e := range c.Endpoints {
			if !seen[e.Group] {
				seen[e.Group] = true
				groups = append(groups, e.Group)
			}
		}
		c.Dashboards = []Dashboard{{Key: "default", Title: c.UI.Title, Visibility: "public", Groups: groups}}
	}
	for i := range c.Dashboards {
		d := &c.Dashboards[i]
		if d.Title == "" {
			d.Title = d.Key
		}
		if d.Visibility == "" {
			d.Visibility = "public"
		}
		if d.DefaultRange == "" {
			d.DefaultRange = "90d"
		}
	}
	c.applySiteDefaults()
}

func envPort(dst *int, key string) {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		*dst = v
	}
}

func inferType(url string) string {
	switch {
	case strings.HasPrefix(url, "tcp://"):
		return "tcp"
	case strings.HasPrefix(url, "icmp://"):
		return "icmp"
	case strings.HasPrefix(url, "dns://"):
		return "dns"
	default:
		return "http"
	}
}

func (c *Config) validate() error {
	var errs []error
	if c.Storage.Type != "postgres" {
		errs = append(errs, fmt.Errorf("storage.type %q is not supported (only postgres)", c.Storage.Type))
	}
	keys := map[string]bool{}
	for i, e := range c.Endpoints {
		where := e.where(i)
		if !keyPattern.MatchString(e.Key) {
			errs = append(errs, fmt.Errorf("%s: key must match %s", where, keyPattern))
		}
		if keys[e.Key] {
			errs = append(errs, fmt.Errorf("%s: duplicate key", where))
		}
		keys[e.Key] = true
		if e.URL == "" {
			errs = append(errs, fmt.Errorf("%s: url is required", where))
		}
		switch e.Type {
		case "http", "tcp", "icmp":
		case "dns":
			if e.DNS == nil || e.DNS.Name == "" {
				errs = append(errs, fmt.Errorf("%s: dns.name is required for dns endpoints", where))
			}
		default:
			errs = append(errs, fmt.Errorf("%s: unknown type %q", where, e.Type))
		}
		if e.Interval.D() < time.Second {
			errs = append(errs, fmt.Errorf("%s: interval must be at least 1s", where))
		}
		if len(e.Conditions) == 0 {
			errs = append(errs, fmt.Errorf("%s: at least one condition is required", where))
		}
		for _, a := range e.Alerts {
			switch {
			case a.Type == "":
				errs = append(errs, fmt.Errorf("%s: alert type is required", where))
			case !slices.Contains(AlertTypes, a.Type):
				errs = append(errs, fmt.Errorf("%s: alert type must be one of %s", where, strings.Join(AlertTypes, ", ")))
			case !c.Alerting.Configured(a.Type):
				errs = append(errs, fmt.Errorf("%s: alert type %q has no provider under alerting.%s", where, a.Type, a.Type))
			}
		}
		errs = append(errs, validateLinks(where+": links", e.Links)...)
	}
	errs = append(errs, validateLinks("ui.footerLinks", c.UI.FooterLinks)...)
	errs = append(errs, c.Alerting.validate()...)

	// Groups that exist per scope: site key, or "" for the top level.
	groupsIn := map[string]map[string]bool{}
	for _, e := range c.Endpoints {
		if groupsIn[e.Site] == nil {
			groupsIn[e.Site] = map[string]bool{}
		}
		groupsIn[e.Site][e.Group] = true
	}
	dkeys := map[string]bool{}
	for i, d := range c.Dashboards {
		where := d.where(i)
		if !keyPattern.MatchString(d.Key) {
			errs = append(errs, fmt.Errorf("%s: key must match %s", where, keyPattern))
		}
		if dkeys[d.Key] {
			errs = append(errs, fmt.Errorf("%s: duplicate key", where))
		}
		dkeys[d.Key] = true
		if d.Visibility != "public" && d.Visibility != "private" {
			errs = append(errs, fmt.Errorf("%s: visibility must be public or private", where))
		}
		if !slices.Contains(Ranges, d.DefaultRange) {
			errs = append(errs, fmt.Errorf("%s: defaultRange must be one of %s", where, strings.Join(Ranges, ", ")))
		}
		for _, k := range d.Endpoints {
			if !keys[k] {
				errs = append(errs, fmt.Errorf("%s: unknown endpoint %q", where, k))
			}
		}
		for _, g := range d.Groups {
			if !groupsIn[d.Site][g] {
				errs = append(errs, fmt.Errorf("%s: no endpoints in group %q here; groups only match endpoints defined next to the dashboard (use endpoints: [key] for others)", where, g))
			}
		}
	}
	errs = append(errs, c.validateSites(dkeys)...)
	return errors.Join(errs...)
}

func (a Alerting) validate() []error {
	var errs []error
	if w := a.Webhook; w != nil && w.URL == "" {
		errs = append(errs, errors.New("alerting.webhook: url is required"))
	}
	if a.Slack != nil && a.Slack.WebhookURL == "" {
		errs = append(errs, errors.New("alerting.slack: webhookUrl is required"))
	}
	if a.Discord != nil && a.Discord.WebhookURL == "" {
		errs = append(errs, errors.New("alerting.discord: webhookUrl is required"))
	}
	if m := a.Email; m != nil {
		if m.Host == "" || m.From == "" || len(m.To) == 0 {
			errs = append(errs, errors.New("alerting.email: host, from and to are required"))
		}
		for _, addr := range append([]string{m.From}, m.To...) {
			if _, err := mail.ParseAddress(addr); addr != "" && err != nil {
				errs = append(errs, fmt.Errorf("alerting.email: %q: %w", addr, err))
			}
		}
	}
	return errs
}

func validateLinks(where string, links []Link) []error {
	var errs []error
	for i, l := range links {
		if l.Name == "" || l.URL == "" {
			errs = append(errs, fmt.Errorf("%s[%d]: name and url are required", where, i))
		}
	}
	return errs
}

// definitionHash covers only what config sync writes to the DB, so changing a
// port or UI title doesn't count as a new endpoint config.
func (c *Config) definitionHash() (string, error) {
	b, err := yaml.Marshal(struct {
		D []Dashboard
		E []Endpoint
	}{c.Dashboards, c.Endpoints})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
