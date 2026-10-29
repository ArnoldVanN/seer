package checker

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/seer-status/seer/apps/server/internal/condition"
	"github.com/seer-status/seer/apps/server/internal/config"
)

// probeDNS queries the nameserver in the endpoint URL (dns://1.1.1.1) directly.
// [BODY] is the first answer's value, [IP] the first A/AAAA answer.
func probeDNS(ctx context.Context, e *config.Endpoint, out *condition.Context) error {
	server := hostPort(e.URL)
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	qtype, ok := dns.StringToType[strings.ToUpper(e.DNS.Type)]
	if !ok {
		if e.DNS.Type != "" {
			return fmt.Errorf("unknown dns query type %q", e.DNS.Type)
		}
		qtype = dns.TypeA
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(e.DNS.Name), qtype)
	c := new(dns.Client)
	start := time.Now()
	resp, rtt, err := c.ExchangeContext(ctx, m, server)
	if err != nil {
		out.ResponseTime = time.Since(start)
		return err
	}
	out.ResponseTime = rtt
	out.Connected = true
	out.DNSRcode = dns.RcodeToString[resp.Rcode]
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			setFirst(out, v.A.String(), true)
		case *dns.AAAA:
			setFirst(out, v.AAAA.String(), true)
		case *dns.CNAME:
			setFirst(out, v.Target, false)
		case *dns.MX:
			setFirst(out, v.Mx, false)
		case *dns.NS:
			setFirst(out, v.Ns, false)
		case *dns.TXT:
			setFirst(out, strings.Join(v.Txt, ""), false)
		}
	}
	return nil
}

func setFirst(out *condition.Context, v string, ip bool) {
	if out.Body == nil {
		out.Body = []byte(v)
	}
	if ip && out.IP == "" {
		out.IP = v
	}
}
