package monitors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Defaults and bounds for the DNS checker (spec §6.3).
const (
	defaultDNSQType        = "A"
	defaultDNSProtocol     = "udp"
	defaultDNSQueryTimeout = 5 // seconds
	maxDNSAnswers          = 5
)

// dnsResolvConfPath is a test-only seam for the system-resolver lookup
// (default /etc/resolv.conf). Tests point it at a missing file to simulate
// an unreadable resolver config without touching the host.
var dnsResolvConfPath = "/etc/resolv.conf"

// allowedDNSQTypes is the supported qtype enum (spec §6.3). It mirrors the
// miekg/dns type constants instead of dns.StringToType so that only the
// spec'd qtypes are accepted.
var allowedDNSQTypes = map[string]uint16{
	"A":     dns.TypeA,
	"AAAA":  dns.TypeAAAA,
	"CNAME": dns.TypeCNAME,
	"MX":    dns.TypeMX,
	"TXT":   dns.TypeTXT,
	"NS":    dns.TypeNS,
	"SOA":   dns.TypeSOA,
	"SRV":   dns.TypeSRV,
	"CAA":   dns.TypeCAA,
	"PTR":   dns.TypePTR,
}

// CheckDNS performs a DNS uptime check from the hub: it sends a single query
// for m.Target with the configured qtype to a custom resolver (IP or
// IP:port) or, when resolver is empty, to the first nameserver listed in the
// system resolv.conf (via dns.ClientConfigFromFile). It never applies
// upside_down: Task 7 applies that inversion. Only stdlib + miekg/dns are
// used.
func CheckDNS(ctx context.Context, m Monitor) CheckResult {
	details := map[string]any{}
	down := func(msg string) CheckResult {
		return CheckResult{Status: StatusDown, Message: msg, Details: details}
	}

	target := strings.TrimSpace(m.Target)
	if target == "" {
		return down("dns: target is required")
	}

	qtypeName := strings.ToUpper(strings.TrimSpace(configString(m.Config["qtype"], defaultDNSQType)))
	qtype, ok := allowedDNSQTypes[qtypeName]
	if !ok {
		return down(fmt.Sprintf("dns: invalid qtype %q: must be A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, CAA or PTR", configString(m.Config["qtype"], "")))
	}

	protocol := strings.ToLower(strings.TrimSpace(configString(m.Config["protocol"], defaultDNSProtocol)))
	if protocol != "udp" && protocol != "tcp" {
		return down(fmt.Sprintf("dns: invalid protocol %q: must be udp or tcp", configString(m.Config["protocol"], "")))
	}

	var qname string
	if qtype == dns.TypePTR {
		ip := net.ParseIP(target)
		if ip == nil {
			return down(fmt.Sprintf("dns: PTR target must be an IP address, got %q", target))
		}
		arpa, err := dns.ReverseAddr(ip.String())
		if err != nil {
			return down(fmt.Sprintf("dns: PTR target must be an IP address, got %q", target))
		}
		qname = arpa
	} else {
		qname = dns.Fqdn(target)
	}

	expected := strings.TrimSpace(configString(m.Config["expected_answer"], ""))
	matchMode := strings.ToLower(strings.TrimSpace(configString(m.Config["match_mode"], "contains")))
	if expected != "" && matchMode != "contains" && matchMode != "exact" {
		return down(fmt.Sprintf("dns: invalid match_mode %q: must be contains or exact", configString(m.Config["match_mode"], "")))
	}

	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	queryTimeout := time.Duration(configInt(m.Config["query_timeout"], defaultDNSQueryTimeout)) * time.Second
	if queryTimeout <= 0 {
		queryTimeout = time.Duration(defaultDNSQueryTimeout) * time.Second
	}
	if queryTimeout > timeout {
		queryTimeout = timeout
	}

	resolverCfg := strings.TrimSpace(configString(m.Config["resolver"], ""))
	server := ""
	resolverLabel := "system"
	if resolverCfg == "" {
		cc, err := dns.ClientConfigFromFile(dnsResolvConfPath)
		if err != nil {
			return down(fmt.Sprintf("dns: cannot read system resolver config: %v", err))
		}
		if len(cc.Servers) == 0 {
			return down("dns: cannot read system resolver config: no nameservers")
		}
		if net.ParseIP(cc.Servers[0]) == nil {
			return down(fmt.Sprintf("dns: cannot use system resolver %q: not an IP address", cc.Servers[0]))
		}
		port := strings.TrimSpace(cc.Port)
		if port == "" {
			port = "53"
		}
		server = net.JoinHostPort(cc.Servers[0], port)
	} else {
		host, port, err := ParsePortOrDefault(resolverCfg, 53)
		if err != nil {
			return down(fmt.Sprintf("dns: invalid resolver %q: %v", resolverCfg, err))
		}
		rip := net.ParseIP(host)
		if rip == nil {
			return down(fmt.Sprintf("dns: invalid resolver %q: must be an IP or IP:port", resolverCfg))
		}
		if os.Getenv(allowPrivateNetworkEnv) != "true" && IsPrivateIP(rip) {
			return down(fmt.Sprintf("dns: resolver is private network %s (set %s=true to allow)", rip.String(), allowPrivateNetworkEnv))
		}
		server = net.JoinHostPort(rip.String(), strconv.Itoa(port))
		resolverLabel = server
	}
	details["qtype"] = qtypeName
	details["resolver"] = resolverLabel

	req := new(dns.Msg)
	req.SetQuestion(qname, qtype)

	client := new(dns.Client)
	client.Net = protocol
	client.Timeout = queryTimeout

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, rtt, err := client.ExchangeContext(reqCtx, req, server)
	latencyMs := float64(rtt) / float64(time.Millisecond)
	if err != nil {
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Message: "dns: query timed out", Details: details}
		}
		return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Message: fmt.Sprintf("dns: query failed: %v", err), Details: details}
	}
	if resp == nil {
		return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Message: "dns: query failed: empty response", Details: details}
	}

	rcode := resp.Rcode
	if rcode != dns.RcodeSuccess {
		var msg string
		switch rcode {
		case dns.RcodeNameError:
			msg = "dns: NXDOMAIN"
		case dns.RcodeServerFailure:
			msg = fmt.Sprintf("dns: SERVFAIL (rcode %d)", rcode)
		default:
			name := dns.RcodeToString[rcode]
			if name == "" {
				name = "ERROR"
			}
			msg = fmt.Sprintf("dns: %s (rcode %d)", name, rcode)
		}
		return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Code: intPtr(rcode), Message: msg, Details: details}
	}

	answers := dnsAnswerStrings(resp)
	details["answers"] = answers
	if len(answers) == 0 {
		return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Code: intPtr(rcode), Message: "dns: no records", Details: details}
	}
	if expected != "" && !dnsMatchAnswer(matchMode, expected, answers) {
		return CheckResult{Status: StatusDown, LatencyMs: latencyMs, Code: intPtr(rcode), Message: fmt.Sprintf("dns: expected answer not found (got: [%s])", strings.Join(answers, ", ")), Details: details}
	}
	return CheckResult{Status: StatusUp, LatencyMs: latencyMs, Code: intPtr(rcode), Message: fmt.Sprintf("dns: NOERROR with %d answer(s)", len(answers)), Details: details}
}

// dnsAnswerStrings renders the answer section of a DNS response as short
// strings (A/AAAA → IP, CNAME/NS/PTR → target, MX → "pref exchange", TXT →
// concatenated strings, SOA → "ns mbox serial", SRV → "prio weight port
// target", CAA → "flag tag value"), truncated to maxDNSAnswers entries.
func dnsAnswerStrings(r *dns.Msg) []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.Answer))
	for _, rr := range r.Answer {
		switch v := rr.(type) {
		case *dns.A:
			out = append(out, v.A.String())
		case *dns.AAAA:
			out = append(out, v.AAAA.String())
		case *dns.CNAME:
			out = append(out, v.Target)
		case *dns.NS:
			out = append(out, v.Ns)
		case *dns.PTR:
			out = append(out, v.Ptr)
		case *dns.MX:
			out = append(out, fmt.Sprintf("%d %s", v.Preference, v.Mx))
		case *dns.TXT:
			out = append(out, strings.Join(v.Txt, ""))
		case *dns.SOA:
			out = append(out, fmt.Sprintf("%s %s %d", v.Ns, v.Mbox, v.Serial))
		case *dns.SRV:
			out = append(out, fmt.Sprintf("%d %d %d %s", v.Priority, v.Weight, v.Port, v.Target))
		case *dns.CAA:
			out = append(out, fmt.Sprintf("%d %s %s", v.Flag, v.Tag, v.Value))
		default:
			out = append(out, rr.String())
		}
		if len(out) >= maxDNSAnswers {
			break
		}
	}
	return out
}

// dnsMatchAnswer reports whether expected matches any answer. Matching is
// case-insensitive with trailing dots ignored; "exact" requires full
// equality after normalization, anything else behaves as "contains".
func dnsMatchAnswer(mode, expected string, answers []string) bool {
	normalize := func(s string) string {
		return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	}
	want := normalize(expected)
	for _, a := range answers {
		got := normalize(a)
		if mode == "exact" {
			if got == want {
				return true
			}
			continue
		}
		if strings.Contains(got, want) {
			return true
		}
	}
	return false
}
