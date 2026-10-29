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

// webhook sends the event itself as JSON.
type webhook struct{ cfg config.Webhook }

func (w *webhook) Notify(ctx context.Context, ev Event) error {
	return postJSON(ctx, w.cfg.Method, w.cfg.URL, w.cfg.Headers, ev)
}

type slack struct{ url string }

func (s *slack) Notify(ctx context.Context, ev Event) error {
	return postJSON(ctx, http.MethodPost, s.url, nil, map[string]any{
		"text": ev.Title(),
		"attachments": []map[string]any{{
			"color":  fmt.Sprintf("#%06x", color(ev)),
			"text":   details(ev),
			"footer": "Seer",
			"ts":     ev.At.Unix(),
		}},
	})
}

type discord struct{ url string }

func (d *discord) Notify(ctx context.Context, ev Event) error {
	return postJSON(ctx, http.MethodPost, d.url, nil, map[string]any{
		"embeds": []map[string]any{{
			"title":       ev.Title(),
			"description": details(ev),
			"color":       color(ev),
			"timestamp":   ev.At.UTC().Format(time.RFC3339),
			"footer":      map[string]string{"text": "Seer"},
		}},
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
	body += "\nEndpoint: " + ev.EndpointKey + "\nAt: " + ev.At.UTC().Format(time.RFC3339) + "\n"
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return b.Bytes()
}
