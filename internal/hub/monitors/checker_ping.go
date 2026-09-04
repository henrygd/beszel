package monitors

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus-community/pro-bing"
)

// Defaults and bounds for the ping checker (spec §6.4).
const (
	defaultPingCount         = 3
	minPingCount             = 1
	maxPingCount             = 10
	defaultPingPacketSize    = 56
	maxPingPacketSize        = 65400
	defaultPingPacketTimeout = 2 * time.Second
	defaultPingInterval      = time.Second
	minPingInterval          = 200 * time.Millisecond
	pingTimeoutMargin        = 2 * time.Second
)

// pingUnavailableMsg is returned when neither unprivileged nor privileged
// ping can run in this environment (e.g. Docker without NET_RAW nor
// ping_group_range). It is an explicit capability signal, never a silent
// false down.
const pingUnavailableMsg = "ping unavailable: missing NET_RAW capability (see docs)"

// pingPermissionSubstrings classifies socket/capability failures (spec §6.4,
// case-insensitive): unprivileged ping fails this way when the process lacks
// NET_RAW / ping_group_range membership, and privileged ping fails this way
// without root or CAP_NET_RAW.
var pingPermissionSubstrings = []string{
	"permission",
	"operation not permitted",
	"socket",
	"capability",
	"privileged",
}

// CheckPing performs an ICMP echo uptime check from the hub using
// pro-bing in UDP (unprivileged) mode first, with a single fallback to
// privileged (raw socket) mode on permission errors. It never applies
// upside_down: Task 7 applies that inversion. Only pro-bing + stdlib.
func CheckPing(ctx context.Context, m Monitor) CheckResult {
	details := map[string]any{}
	down := func(msg string) CheckResult {
		return CheckResult{Status: StatusDown, Message: msg, Details: details}
	}

	target := strings.TrimSpace(m.Target)
	if target == "" {
		return down("ping: target is required")
	}

	// SSRF guard: only literal IPs are checked. Hostnames are passed as-is
	// to the pinger (which resolves them) and intentionally NOT blocked:
	// their resolution may legitimately be local in tests (localhost), CI,
	// or intranet monitoring, and ping stays informative there.
	if ip := net.ParseIP(stripBrackets(target)); ip != nil {
		if os.Getenv(allowPrivateNetworkEnv) != "true" && IsPrivateIP(ip) {
			return down(fmt.Sprintf("ping: target is private network %s (set %s=true to allow)", ip.String(), allowPrivateNetworkEnv))
		}
	}

	count := configInt(m.Config["count"], defaultPingCount)
	if count < minPingCount || count > maxPingCount {
		return down(fmt.Sprintf("invalid count %v: must be %d..%d", m.Config["count"], minPingCount, maxPingCount))
	}

	size := configInt(m.Config["packet_size"], defaultPingPacketSize)
	if size < 0 || size > maxPingPacketSize {
		return down(fmt.Sprintf("invalid packet_size %v: must be 0..%d", m.Config["packet_size"], maxPingPacketSize))
	}

	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}

	packetTimeout := parsePingDuration(m.Config["packet_timeout"], defaultPingPacketTimeout)
	if packetTimeout <= 0 || packetTimeout > timeout {
		return down(fmt.Sprintf("invalid packet_timeout %v: must be > 0 and <= check timeout (%s)", m.Config["packet_timeout"], timeout))
	}

	interval := parsePingDuration(m.Config["interval_between_packets"], defaultPingInterval)
	if interval < minPingInterval {
		return down(fmt.Sprintf("invalid interval_between_packets %v: must be >= %s", m.Config["interval_between_packets"], minPingInterval))
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Bound the pinger's internal timeout: per-packet budget over all
	// packets plus margin, capped by the global check timeout.
	// RunWithContext(reqCtx) propagates cancellation on top.
	want := packetTimeout*time.Duration(count) + pingTimeoutMargin
	if want > timeout {
		want = timeout
	}

	newPinger := func(privileged bool) (*probing.Pinger, error) {
		p, err := probing.NewPinger(target)
		if err != nil {
			return nil, err
		}
		p.SetPrivileged(privileged)
		p.Count = count
		p.Size = size
		p.Interval = interval
		p.Timeout = want
		p.ResolveTimeout = timeout
		return p, nil
	}

	p, err := newPinger(false)
	if err != nil {
		return down(fmt.Sprintf("cannot resolve host: %v", err))
	}
	if err := p.RunWithContext(reqCtx); err != nil {
		if !isPingPermissionError(err) {
			return down(fmt.Sprintf("ping failed: %v", err))
		}
		// Unprivileged mode lacks socket capability: retry exactly once
		// in privileged mode with a fresh pinger.
		fp, ferr := newPinger(true)
		if ferr != nil {
			return down(fmt.Sprintf("cannot resolve host: %v", ferr))
		}
		if err := fp.RunWithContext(reqCtx); err != nil {
			if isPingPermissionError(err) {
				return down(pingUnavailableMsg)
			}
			return down(fmt.Sprintf("ping failed: %v", err))
		}
		return pingResult(fp.Statistics())
	}
	return pingResult(p.Statistics())
}

// pingResult maps pro-bing statistics to a CheckResult: up with
// "<recv>/<sent> packets received" when at least one reply arrived, else a
// typed 100%-loss down. LatencyMs is the average RTT in fractional
// milliseconds (0 when nothing replied). Code stays nil.
func pingResult(stats *probing.Statistics) CheckResult {
	details := map[string]any{}
	if stats == nil {
		return CheckResult{Status: StatusDown, Message: "ping: no statistics", Details: details}
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	details["min_ms"] = ms(stats.MinRtt)
	details["avg_ms"] = ms(stats.AvgRtt)
	details["max_ms"] = ms(stats.MaxRtt)
	details["loss_pct"] = stats.PacketLoss
	details["received"] = stats.PacketsRecv
	details["sent"] = stats.PacketsSent
	if stats.PacketsRecv >= 1 {
		return CheckResult{
			Status:    StatusUp,
			LatencyMs: ms(stats.AvgRtt),
			Message:   fmt.Sprintf("%d/%d packets received", stats.PacketsRecv, stats.PacketsSent),
			Details:   details,
		}
	}
	return CheckResult{
		Status:    StatusDown,
		LatencyMs: ms(stats.AvgRtt),
		Message:   "ping: no reply, packet loss 100%",
		Details:   details,
	}
}

// isPingPermissionError reports whether err looks like a missing socket
// capability (spec §6.4 substring list, case-insensitive).
func isPingPermissionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, sub := range pingPermissionSubstrings {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}

// parsePingDuration parses packet_timeout / interval_between_packets values:
// nil → default, time.Duration as-is, numerics → seconds, strings →
// time.ParseDuration then integer seconds, anything else → default.
func parsePingDuration(v any, def time.Duration) time.Duration {
	switch t := v.(type) {
	case nil:
		return def
	case time.Duration:
		return t
	case int:
		return time.Duration(t) * time.Second
	case int64:
		return time.Duration(t) * time.Second
	case float64:
		return time.Duration(t * float64(time.Second))
	case string:
		s := strings.TrimSpace(t)
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
		if n, err := strconv.Atoi(s); err == nil {
			return time.Duration(n) * time.Second
		}
		return def
	default:
		return def
	}
}
