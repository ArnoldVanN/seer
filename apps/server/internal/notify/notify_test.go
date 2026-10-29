package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seer-status/seer/apps/server/internal/config"
	"github.com/seer-status/seer/apps/server/internal/testdb"
)

var ev = Event{
	Kind: Triggered, AlertType: "x", EndpointKey: "api", EndpointName: "API", Group: "core",
	Description: "Paging the API team", Error: "status 500", At: time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC),
}

// capture serves one request and returns its headers and decoded JSON body.
func capture(t *testing.T, status int) (string, func() (http.Header, map[string]any)) {
	var mu sync.Mutex
	var h http.Header
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		h = r.Header.Clone()
		h.Set("X-Method", r.Method)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() (http.Header, map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		return h, body
	}
}

func TestWebhook(t *testing.T) {
	url, got := capture(t, http.StatusOK)
	w := &webhook{cfg: config.Webhook{URL: url, Method: "PUT", Headers: map[string]string{"Authorization": "Bearer s3cret"}}}
	if err := w.Notify(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	h, body := got()
	if h.Get("X-Method") != "PUT" || h.Get("Authorization") != "Bearer s3cret" || h.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", h)
	}
	if body["kind"] != "triggered" || body["endpointKey"] != "api" || body["error"] != "status 500" || body["at"] != "2026-10-29T08:00:00Z" {
		t.Errorf("body = %v", body)
	}
}

func TestSlackAndDiscord(t *testing.T) {
	url, got := capture(t, http.StatusOK)
	if err := (&slack{url: url}).Notify(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	_, body := got()
	att := body["attachments"].([]any)[0].(map[string]any)
	if body["text"] != "core / API is down" || att["color"] != "#e5484d" || att["text"] != "Paging the API team\nError: status 500" {
		t.Errorf("slack body = %v", body)
	}

	url, got = capture(t, http.StatusNoContent)
	resolved := ev
	resolved.Kind, resolved.Error = Resolved, ""
	if err := (&discord{url: url}).Notify(context.Background(), resolved); err != nil {
		t.Fatal(err)
	}
	_, body = got()
	emb := body["embeds"].([]any)[0].(map[string]any)
	if emb["title"] != "core / API is back up" || emb["color"] != float64(colorUp) || emb["description"] != "Paging the API team" {
		t.Errorf("discord body = %v", body)
	}
}

func TestHTTPError(t *testing.T) {
	url, _ := capture(t, http.StatusBadRequest)
	err := (&slack{url: url}).Notify(context.Background(), ev)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v, want the status", err)
	}
}

// smtpServer accepts one plaintext session and returns the recipients and the
// message data.
func smtpServer(t *testing.T) (string, int, <-chan []string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan []string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		say := func(s string) { conn.Write([]byte(s + "\r\n")) }
		say("220 test")
		var rcpts []string
		var data strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.Fields(line)[0])
			switch cmd {
			case "EHLO", "HELO":
				say("250 test")
			case "RCPT":
				rcpts = append(rcpts, strings.TrimSpace(line))
				say("250 ok")
			case "DATA":
				say("354 go")
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				say("250 queued")
			case "QUIT":
				say("221 bye")
				out <- append(rcpts, data.String())
				return
			default:
				say("250 ok")
			}
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port, out
}

func TestEmail(t *testing.T) {
	host, port, got := smtpServer(t)
	m := &email{cfg: config.Email{Host: host, Port: port, From: "Seer <seer@acme.test>", To: []string{"ops@acme.test", "Dev <dev@acme.test>"}}}
	if err := m.Notify(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	res := <-got
	if res[0] != "RCPT TO:<ops@acme.test>" || res[1] != "RCPT TO:<dev@acme.test>" {
		t.Errorf("recipients = %v", res[:2])
	}
	msg := res[2]
	for _, want := range []string{"Subject: core / API is down\r\n", "To: ops@acme.test, Dev <dev@acme.test>\r\n", "\r\nError: status 500\r\n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

type flaky struct {
	mu    sync.Mutex
	fails int
	got   []Kind
}

func (f *flaky) Notify(_ context.Context, ev Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return errors.New("unavailable")
	}
	f.got = append(f.got, ev.Kind)
	return nil
}

func TestDispatcherRetriesInOrder(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	firstBackoff = time.Millisecond
	ep := d.Ent.Endpoint.Create().SetKey("api").SetName("API").SetType("http").SetIntervalSeconds(60).SetSpec([]byte("{}")).SaveX(ctx)

	resolved := ev
	resolved.Kind = Resolved
	tx, err := d.Ent.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(ctx, tx, ep.ID, []Event{ev, resolved, {AlertType: "unconfigured"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	f := &flaky{fails: 2}
	disp := New(d, config.Alerting{})
	disp.Register("x", f)

	// While another worker holds the trigger, the resolve must wait for it.
	other, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ExecContext(ctx, `SELECT id FROM notifications ORDER BY id LIMIT 1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	if sent, err := disp.sendNext(ctx); err != nil || sent {
		t.Fatalf("sendNext with the trigger locked = %v, %v; want nothing to send", sent, err)
	}
	// That worker dies before sending.
	other.Rollback()

	for range 20 {
		if _, err := disp.sendNext(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(f.got) != 2 || f.got[0] != Triggered || f.got[1] != Resolved {
		t.Errorf("delivered %v, want [triggered resolved]", f.got)
	}
	types, err := d.Ent.Notification.Query().Select("alert_type").Strings(ctx)
	if err != nil || len(types) != 1 || types[0] != "unconfigured" {
		t.Errorf("left in queue: %v, %v; want only the unconfigured one", types, err)
	}
}
