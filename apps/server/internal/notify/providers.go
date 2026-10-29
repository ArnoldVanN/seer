package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/seer-status/seer/apps/server/internal/config"
)

const (
	colorDown = 0xe5484d
	colorUp   = 0x30a46c
)

func color(ev Event) int {
	if ev.Kind == Resolved {
		return colorUp
	}
	return colorDown
}

// details is the body text under the title: the alert's description and the
// error that triggered it.
func details(ev Event) string {
	var parts []string
	if ev.Description != "" {
		parts = append(parts, ev.Description)
	}
	if ev.Error != "" {
		parts = append(parts, "Error: "+ev.Error)
	}
	return strings.Join(parts, "\n")
}

type fact struct {
	name, value string
	// Code is a value to show verbatim, such as a condition.
	code bool
}

// facts are the details providers show as labelled fields.
func facts(ev Event) []fact {
	var fs []fact
	if len(ev.FailedConditions) > 0 {
		fs = append(fs, fact{"Failed conditions", strings.Join(ev.FailedConditions, "\n"), true})
	}
	if ev.Failures > 0 {
		fs = append(fs, fact{"Consecutive failures", strconv.Itoa(ev.Failures), false})
	}
	if ev.DownSince != nil {
		fs = append(fs, fact{"Downtime", downtime(ev.At.Sub(*ev.DownSince)), false})
	}
	if ev.URL != "" {
		fs = append(fs, fact{"URL", ev.URL, false})
	}
	return fs
}

// downtime reads like "1h5m" or "45s".
func downtime(d time.Duration) string {
	if d >= time.Minute {
		d = d.Round(time.Minute)
	} else {
		d = d.Round(time.Second)
	}
	s := d.String()
	if d >= time.Minute {
		s = strings.TrimSuffix(s, "0s")
	}
	if d >= time.Hour {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// linkURL is ev.URL if a browser can open it.
func linkURL(ev Event) string {
	if strings.HasPrefix(ev.URL, "http://") || strings.HasPrefix(ev.URL, "https://") {
		return ev.URL
	}
	return ""
}

// webhook sends the event itself as JSON.
type webhook struct{ cfg config.Webhook }

func (w *webhook) Notify(ctx context.Context, ev Event) error {
	return postJSON(ctx, w.cfg.Method, w.cfg.URL, w.cfg.Headers, ev)
}

type slack struct{ url string }

func (s *slack) Notify(ctx context.Context, ev Event) error {
	var fields []map[string]any
	for _, f := range facts(ev) {
		v := f.value
		if f.code {
			v = "```" + v + "```"
		}
		fields = append(fields, map[string]any{"title": f.name, "value": v, "short": !f.code && f.name != "URL"})
	}
	att := map[string]any{
		"color":  fmt.Sprintf("#%06x", color(ev)),
		"text":   details(ev),
		"fields": fields,
		"footer": "Seer",
		"ts":     ev.At.Unix(),
	}
	if u := linkURL(ev); u != "" {
		att["title"], att["title_link"] = ev.EndpointName, u
	}
	return postJSON(ctx, http.MethodPost, s.url, nil, map[string]any{
		"text":        ev.Title(),
		"attachments": []map[string]any{att},
	})
}

type discord struct{ url string }

func (d *discord) Notify(ctx context.Context, ev Event) error {
	var fields []map[string]any
	for _, f := range facts(ev) {
		if f.name == "URL" {
			continue // the title links there
		}
		v := f.value
		if f.code {
			v = "```\n" + v + "\n```"
		}
		fields = append(fields, map[string]any{"name": f.name, "value": v, "inline": !f.code})
	}
	emb := map[string]any{
		"title":       ev.Title(),
		"description": details(ev),
		"fields":      fields,
		"color":       color(ev),
		"timestamp":   ev.At.UTC().Format(time.RFC3339),
		"footer":      map[string]string{"text": "Seer"},
	}
	if u := linkURL(ev); u != "" {
		emb["url"] = u
	}
	return postJSON(ctx, http.MethodPost, d.url, nil, map[string]any{
		"embeds": []map[string]any{emb},
	})
}

func postJSON(ctx context.Context, method, url string, headers map[string]string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "seer")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("%s: %s", res.Status, bytes.TrimSpace(msg))
	}
	return nil
}

type email struct{ cfg config.Email }

func (m *email) Notify(ctx context.Context, ev Event) error {
	c := m.cfg
	from, err := mail.ParseAddress(c.From)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	tlsCfg := &tls.Config{ServerName: c.Host}
	var conn net.Conn
	if c.Port == 465 {
		conn, err = (&tls.Dialer{Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()
	if ok, _ := cl.Extension("STARTTLS"); ok && c.Port != 465 {
		if err := cl.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	// PlainAuth refuses to send the password over an unencrypted connection
	// to anything but localhost.
	if c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return err
		}
	}
	if err := cl.Mail(from.Address); err != nil {
		return err
	}
	for _, to := range c.To {
		a, err := mail.ParseAddress(to)
		if err != nil {
			return fmt.Errorf("to: %w", err)
		}
		if err := cl.Rcpt(a.Address); err != nil {
			return err
		}
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(message(c, ev)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

func message(c config.Email, ev Event) []byte {
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", c.From)
	header("To", strings.Join(c.To, ", "))
	header("Subject", mime.QEncoding.Encode("utf-8", ev.Title()))
	header("Date", ev.At.Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	b.WriteString("\r\n")
	body := ev.Title() + "\n"
	if d := details(ev); d != "" {
		body += "\n" + d + "\n"
	}
	body += "\n"
	for _, f := range facts(ev) {
		if f.code {
			body += f.name + ":\n  " + strings.ReplaceAll(f.value, "\n", "\n  ") + "\n"
		} else {
			body += f.name + ": " + f.value + "\n"
		}
	}
	body += "Endpoint: " + ev.EndpointKey + "\nAt: " + ev.At.UTC().Format(time.RFC3339) + "\n"
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return b.Bytes()
}
