package checker

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/seer-status/seer/apps/server/internal/condition"
	"github.com/seer-status/seer/apps/server/internal/config"
)

// Bodies past this are cut off; conditions only see the first part.
const maxBody = 1 << 20

var (
	clientsMu sync.Mutex
	clients   = map[bool]*http.Client{}
)

func httpClient(insecure bool) *http.Client {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	if c, ok := clients[insecure]; ok {
		return c
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // opt-in per endpoint
	// Keep-alives would hide connection problems and skew response times.
	t.DisableKeepAlives = true
	c := &http.Client{
		Transport: t,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	clients[insecure] = c
	return c
}

func probeHTTP(ctx context.Context, e *config.Endpoint, out *condition.Context) error {
	var body io.Reader
	if e.Body != "" {
		body = strings.NewReader(e.Body)
	}
	req, err := http.NewRequestWithContext(ctx, e.Method, e.URL, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "seer/1.0")
	for k, v := range e.Headers {
		if strings.EqualFold(k, "host") {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}

	req = req.WithContext(httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if addr, ok := info.Conn.RemoteAddr().(*net.TCPAddr); ok {
				out.IP = addr.IP.String()
			}
		},
	}))

	start := time.Now()
	resp, err := httpClient(e.Insecure).Do(req)
	if err != nil {
		out.ResponseTime = time.Since(start)
		return err
	}
	defer resp.Body.Close()
	out.Connected = true
	out.Status = resp.StatusCode
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		out.CertificateExpiration = time.Until(resp.TLS.PeerCertificates[0].NotAfter)
	}
	out.Body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody))
	out.ResponseTime = time.Since(start)
	return err
}
