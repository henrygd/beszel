package monitors

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func dnsTestMonitor(target string) Monitor {
	return Monitor{
		Name:            "dns-test",
		Type:            TypeDNS,
		Target:          target,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
		Config:          map[string]any{},
	}
}

func dnsTestCtx() context.Context {
	return context.Background()
}

// startDNSTestServer runs a fake DNS server on loopback (UDP or TCP) and
// returns its address. It enables the private-network override so the
// loopback resolver is accepted by the SSRF guard.
func startDNSTestServer(t *testing.T, network string, handle dns.HandlerFunc) string {
	t.Helper()
	allowPrivateNet(t)
	mux := dns.NewServeMux()
	mux.HandleFunc(".", handle)
	srv := &dns.Server{Net: network, Handler: mux}
	var addr string
	switch network {
	case "tcp":
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen tcp: %v", err)
		}
		srv.Listener = ln
		addr = ln.Addr().String()
	default:
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen udp: %v", err)
		}
		srv.PacketConn = pc
		addr = pc.LocalAddr().String()
	}
	go func() {
		_ = srv.ActivateAndServe()
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown()
	})
	// Give the server a moment to start accepting.
	time.Sleep(50 * time.Millisecond)
	return addr
}

func dnsAResponder(ip string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP(ip),
			},
		}
		_ = w.WriteMsg(m)
	}
}

func TestDNSNoErrorMatch(t *testing.T) {
	addr := startDNSTestServer(t, "udp", dnsAResponder("93.184.216.34"))
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	m.Config["expected_answer"] = "93.184.216.34"
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
	if res.Code == nil || *res.Code != 0 {
		t.Fatalf("expected rcode 0, got %+v", res.Code)
	}
	if len(res.Details["answers"].([]string)) != 1 {
		t.Fatalf("expected 1 answer, got %v", res.Details["answers"])
	}
	if res.Details["qtype"] != "A" {
		t.Fatalf("expected qtype A, got %v", res.Details["qtype"])
	}
	if res.Details["resolver"] != addr {
		t.Fatalf("expected resolver %q, got %v", addr, res.Details["resolver"])
	}
}

func TestDNSTrailingDotTarget(t *testing.T) {
	addr := startDNSTestServer(t, "udp", dnsAResponder("93.184.216.34"))
	m := dnsTestMonitor("example.com.")
	m.Config["resolver"] = addr
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
}

func TestDNSMismatch(t *testing.T) {
	addr := startDNSTestServer(t, "udp", dnsAResponder("93.184.216.34"))
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	m.Config["expected_answer"] = "1.2.3.4"
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if !strings.HasPrefix(res.Message, "dns: expected answer not found (got: ") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestDNSNXDOMAIN(t *testing.T) {
	addr := startDNSTestServer(t, "udp", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(m)
	})
	m := dnsTestMonitor("nonexistent.example.com")
	m.Config["resolver"] = addr
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if res.Message != "dns: NXDOMAIN" {
		t.Fatalf("unexpected message %q", res.Message)
	}
	if res.Code == nil || *res.Code != 3 {
		t.Fatalf("expected rcode 3, got %+v", res.Code)
	}
}

func TestDNSServfail(t *testing.T) {
	addr := startDNSTestServer(t, "udp", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(m)
	})
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if res.Message != "dns: SERVFAIL (rcode 2)" {
		t.Fatalf("unexpected message %q", res.Message)
	}
	if res.Code == nil || *res.Code != 2 {
		t.Fatalf("expected rcode 2, got %+v", res.Code)
	}
}

func TestDNSNoRecords(t *testing.T) {
	addr := startDNSTestServer(t, "udp", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r) // NOERROR, empty answer section
		_ = w.WriteMsg(m)
	})
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if res.Message != "dns: no records" {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestDNSQueryTimeout(t *testing.T) {
	addr := startDNSTestServer(t, "udp", func(w dns.ResponseWriter, r *dns.Msg) {
		time.Sleep(3 * time.Second)
		m := new(dns.Msg)
		m.SetReply(r)
		_ = w.WriteMsg(m)
	})
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	m.Config["query_timeout"] = 1
	m.TimeoutSeconds = 10
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if res.Message != "dns: query timed out" {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestDNSTCP(t *testing.T) {
	addr := startDNSTestServer(t, "tcp", dnsAResponder("93.184.216.34"))
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	m.Config["protocol"] = "tcp"
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
}

func TestDNSTXTContains(t *testing.T) {
	addr := startDNSTestServer(t, "udp", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{
			&dns.TXT{
				Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 300},
				Txt: []string{"v=spf1 include:_spf.example.com ~all"},
			},
		}
		_ = w.WriteMsg(m)
	})
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = addr
	m.Config["qtype"] = "TXT"
	m.Config["expected_answer"] = "spf1"
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
}

func TestDNSPTRLookup(t *testing.T) {
	addr := startDNSTestServer(t, "udp", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{
			&dns.PTR{
				Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 300},
				Ptr: "host.example.com.",
			},
		}
		_ = w.WriteMsg(m)
	})
	m := dnsTestMonitor("8.8.8.8")
	m.Config["resolver"] = addr
	m.Config["qtype"] = "PTR"
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
}

func TestDNSValidation(t *testing.T) {
	allowPrivateNet(t)
	cases := []struct {
		name   string
		mon    func() Monitor
		prefix string
	}{
		{"empty target", func() Monitor { return dnsTestMonitor("") }, "dns: target is required"},
		{"unknown qtype", func() Monitor {
			m := dnsTestMonitor("example.com")
			m.Config["qtype"] = "NOPE"
			return m
		}, "dns: invalid qtype"},
		{"resolver hostname", func() Monitor {
			m := dnsTestMonitor("example.com")
			m.Config["resolver"] = "dns.google"
			return m
		}, "dns: invalid resolver"},
		{"resolver bad port", func() Monitor {
			m := dnsTestMonitor("example.com")
			m.Config["resolver"] = "8.8.8.8:99999"
			return m
		}, "dns: invalid resolver"},
		{"PTR non-IP target", func() Monitor {
			m := dnsTestMonitor("example.com")
			m.Config["qtype"] = "PTR"
			return m
		}, "dns: PTR target must be an IP"},
		{"bad protocol", func() Monitor {
			m := dnsTestMonitor("example.com")
			m.Config["protocol"] = "icmp"
			return m
		}, "dns: invalid protocol"},
		{"bad match mode", func() Monitor {
			m := dnsTestMonitor("example.com")
			m.Config["expected_answer"] = "x"
			m.Config["match_mode"] = "fuzzy"
			return m
		}, "dns: invalid match_mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := CheckDNS(dnsTestCtx(), tc.mon())
			if res.Status != StatusDown {
				t.Fatalf("expected down, got %q", res.Status)
			}
			if !strings.HasPrefix(res.Message, tc.prefix) {
				t.Fatalf("expected message prefix %q, got %q", tc.prefix, res.Message)
			}
		})
	}
}

func TestDNSPrivateResolverBlocked(t *testing.T) {
	// No override: private resolver IPs must be rejected.
	m := dnsTestMonitor("example.com")
	m.Config["resolver"] = "10.0.0.1"
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if !strings.HasPrefix(res.Message, "dns: resolver is private network") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestDNSSystemResolverUnreadable(t *testing.T) {
	old := dnsResolvConfPath
	dnsResolvConfPath = "/nonexistent/resolv.conf"
	t.Cleanup(func() { dnsResolvConfPath = old })
	m := dnsTestMonitor("example.com")
	res := CheckDNS(dnsTestCtx(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if !strings.HasPrefix(res.Message, "dns: cannot read system resolver config") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestDnsAnswerStrings(t *testing.T) {
	qname := "example.com."
	hdr := func(t uint16) dns.RR_Header {
		return dns.RR_Header{Name: qname, Rrtype: t, Class: dns.ClassINET, Ttl: 300}
	}
	cases := []struct {
		name string
		rr   dns.RR
		want []string
	}{
		{"A", &dns.A{Hdr: hdr(dns.TypeA), A: net.ParseIP("93.184.216.34")}, []string{"93.184.216.34"}},
		{"AAAA", &dns.AAAA{Hdr: hdr(dns.TypeAAAA), AAAA: net.ParseIP("2606:2800:220:1:248:1893:25c8:1946")}, []string{"2606:2800:220:1:248:1893:25c8:1946"}},
		{"CNAME", &dns.CNAME{Hdr: hdr(dns.TypeCNAME), Target: "alias.example.com."}, []string{"alias.example.com."}},
		{"MX", &dns.MX{Hdr: hdr(dns.TypeMX), Preference: 10, Mx: "mail.example.com."}, []string{"10 mail.example.com."}},
		{"TXT", &dns.TXT{Hdr: hdr(dns.TypeTXT), Txt: []string{"hello ", "world"}}, []string{"hello world"}},
		{"NS", &dns.NS{Hdr: hdr(dns.TypeNS), Ns: "ns1.example.com."}, []string{"ns1.example.com."}},
		{"SOA", &dns.SOA{
			Hdr: hdr(dns.TypeSOA), Ns: "ns1.example.com.",
			Mbox: "hostmaster.example.com.", Serial: 2024010101,
		}, []string{"ns1.example.com. hostmaster.example.com. 2024010101"}},
		{"SRV", &dns.SRV{Hdr: hdr(dns.TypeSRV), Priority: 10, Weight: 20, Port: 80, Target: "web.example.com."}, []string{"10 20 80 web.example.com."}},
		{"CAA", &dns.CAA{Hdr: hdr(dns.TypeCAA), Flag: 0, Tag: "issue", Value: "letsencrypt.org"}, []string{"0 issue letsencrypt.org"}},
		{"PTR", &dns.PTR{Hdr: hdr(dns.TypePTR), Ptr: "host.example.com."}, []string{"host.example.com."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := new(dns.Msg)
			msg.Answer = []dns.RR{tc.rr}
			got := dnsAnswerStrings(msg)
			if len(got) != len(tc.want) || got[0] != tc.want[0] {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	// Truncation to 5.
	many := new(dns.Msg)
	for i := 1; i <= 6; i++ {
		many.Answer = append(many.Answer, &dns.A{
			Hdr: hdr(dns.TypeA),
			A:   net.ParseIP(fmt.Sprintf("10.0.0.%d", i)),
		})
	}
	if got := dnsAnswerStrings(many); len(got) != 5 {
		t.Fatalf("expected 5 answers, got %d", len(got))
	}
}

func TestDnsMatchAnswer(t *testing.T) {
	answers := []string{"93.184.216.34", "Mail.Example.COM."}
	if !dnsMatchAnswer("contains", "93.184.216", answers) {
		t.Error("contains should match substring")
	}
	if !dnsMatchAnswer("contains", "mail.example.com", answers) {
		t.Error("contains should be case-insensitive and ignore trailing dot")
	}
	if dnsMatchAnswer("contains", "1.2.3.4", answers) {
		t.Error("contains should not match absent value")
	}
	if !dnsMatchAnswer("exact", "mail.example.com.", answers) {
		t.Error("exact should match after normalization")
	}
	if dnsMatchAnswer("exact", "mail.example", answers) {
		t.Error("exact should not match partial value")
	}
}
