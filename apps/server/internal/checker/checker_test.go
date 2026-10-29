package checker

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/seer-status/seer/apps/server/internal/config"
)

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/down" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"status":"UP"}`))
	}))
	defer srv.Close()

	ok := Run(context.Background(), &config.Endpoint{
		Type: "http", Method: "GET", URL: srv.URL, Timeout: config.Duration(time.Second),
		Conditions: []string{"[STATUS] == 200", "[BODY].status == UP", "[RESPONSE_TIME] < 1000"},
	})
	if !ok.Success {
		t.Fatalf("expected success, got %+v", ok)
	}

	down := Run(context.Background(), &config.Endpoint{
		Type: "http", Method: "GET", URL: srv.URL + "/down", Timeout: config.Duration(time.Second),
		Conditions: []string{"[STATUS] == 200"},
	})
	if down.Success || down.Conditions[0].Success {
		t.Fatalf("expected failure, got %+v", down)
	}
}

func TestTCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	addr := l.Addr().String()

	up := Run(context.Background(), &config.Endpoint{
		Type: "tcp", URL: "tcp://" + addr, Timeout: config.Duration(time.Second),
		Conditions: []string{"[CONNECTED] == true"},
	})
	if !up.Success {
		t.Fatalf("expected success, got %+v", up)
	}
	l.Close()

	down := Run(context.Background(), &config.Endpoint{
		Type: "tcp", URL: "tcp://" + addr, Timeout: config.Duration(time.Second),
		Conditions: []string{"[CONNECTED] == true"},
	})
	if down.Success {
		t.Fatalf("expected failure after close, got %+v", down)
	}
}
