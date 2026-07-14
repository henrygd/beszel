package agent

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	defaultPingTimeout = 3 * time.Second
	defaultPingTarget  = "1.1.1.1:443"
)

// namedTarget is a latency probe endpoint with an optional display name.
type namedTarget struct {
	Name string // chart/table label (e.g. 电信广东)
	Addr string // host:port to dial
}

// latencyManager probes TCP connect RTT to configured targets (Nezha-style delay monitoring).
type latencyManager struct {
	mu          sync.Mutex
	targets     []namedTarget
	envTargets  []namedTarget // defaults from PING_TARGETS / HUB_URL
	hubOverride bool          // true when hub last sent ConfigureLatency
	timeout     time.Duration
}

func newLatencyManager() *latencyManager {
	envTargets := parsePingTargets()
	if len(envTargets) == 0 {
		envTargets = []namedTarget{{Name: defaultPingTarget, Addr: defaultPingTarget}}
	}
	lm := &latencyManager{
		timeout:    defaultPingTimeout,
		envTargets: envTargets,
		targets:    append([]namedTarget(nil), envTargets...),
	}
	slog.Info("Latency targets", "targets", formatTargetsLog(lm.targets))
	return lm
}

// applyHubTargets updates targets from hub config.
// empty raw falls back to env defaults. Returns true if targets changed.
func (lm *latencyManager) applyHubTargets(raw string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var next []namedTarget
	if strings.TrimSpace(raw) == "" {
		next = append([]namedTarget(nil), lm.envTargets...)
		lm.hubOverride = false
	} else {
		next = parseNamedTargets(raw)
		if len(next) == 0 {
			next = append([]namedTarget(nil), lm.envTargets...)
			lm.hubOverride = false
		} else {
			lm.hubOverride = true
		}
	}

	if namedTargetsEqual(lm.targets, next) {
		return false
	}
	lm.targets = next
	slog.Info("Latency targets updated", "targets", formatTargetsLog(lm.targets), "source", map[bool]string{true: "hub", false: "env"}[lm.hubOverride])
	return true
}

// disableHubProbes clears probes so this agent does not measure latency
// (used when hub has targets but this system is not selected).
func (lm *latencyManager) disableHubProbes() bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.hubOverride && len(lm.targets) == 0 {
		return false
	}
	lm.hubOverride = true
	lm.targets = nil
	slog.Info("Latency probes disabled by hub")
	return true
}

func namedTargetsEqual(a, b []namedTarget) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Addr != b[i].Addr {
			return false
		}
	}
	return true
}

func formatTargetsLog(targets []namedTarget) []string {
	out := make([]string, len(targets))
	for i, t := range targets {
		if t.Name == t.Addr {
			out[i] = t.Addr
		} else {
			out[i] = t.Name + "=" + t.Addr
		}
	}
	return out
}

// parsePingTargets reads PING_TARGETS (comma/newline separated, optional name=addr).
// If unset, uses HUB_URL host (with its port or 443) plus the default public target.
func parsePingTargets() []namedTarget {
	if raw, ok := utils.GetEnv("PING_TARGETS"); ok {
		return parseNamedTargets(raw)
	}

	var targets []namedTarget
	if hubURL, ok := utils.GetEnv("HUB_URL"); ok {
		if t := targetFromURL(hubURL); t != "" {
			targets = append(targets, namedTarget{Name: t, Addr: t})
		}
	}
	targets = append(targets, namedTarget{Name: defaultPingTarget, Addr: defaultPingTarget})
	return uniqueNamedTargets(targets)
}

// parseNamedTargets accepts:
//   - name=host:port (display name for charts)
//   - host:port (name defaults to address)
// Separators: comma and/or newline.
func parseNamedTargets(raw string) []namedTarget {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	// split on commas and newlines
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]namedTarget, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		name, addr := splitNameAddr(p)
		addr = ensureHostPort(addr)
		if addr == "" {
			continue
		}
		if name == "" {
			name = addr
		}
		out = append(out, namedTarget{Name: name, Addr: addr})
	}
	return uniqueNamedTargets(out)
}

// splitNameAddr parses "电信广东=host:80" or bare "host:80".
// Only the first '=' splits name from address (IPv6 uses brackets, not bare =).
func splitNameAddr(p string) (name, addr string) {
	// Support "name=host:port" but not treat IPv6 as name=
	if i := strings.Index(p, "="); i > 0 {
		left := strings.TrimSpace(p[:i])
		right := strings.TrimSpace(p[i+1:])
		// if left looks like a host:port or IP, treat whole thing as address without name
		if looksLikeAddress(left) {
			return "", p
		}
		if right != "" {
			return left, right
		}
	}
	return "", p
}

func looksLikeAddress(s string) bool {
	if strings.Contains(s, ":") || strings.Contains(s, ".") || strings.HasPrefix(s, "[") {
		// hostname/IP style — not a human label like 电信广东
		if _, _, err := net.SplitHostPort(s); err == nil {
			return true
		}
		if net.ParseIP(s) != nil {
			return true
		}
		// host.domain without port
		if strings.Contains(s, ".") && !strings.Contains(s, " ") {
			return true
		}
	}
	return false
}

func uniqueNamedTargets(in []namedTarget) []namedTarget {
	seen := make(map[string]struct{}, len(in))
	out := make([]namedTarget, 0, len(in))
	for _, t := range in {
		key := t.Name + "\x00" + t.Addr
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out
}

func targetFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" || u.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port)
}

func ensureHostPort(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	// IPv6 with brackets already ok for SplitHostPort
	if _, _, err := net.SplitHostPort(target); err == nil {
		return target
	}
	// bare host / IPv4 / hostname
	if strings.HasPrefix(target, "[") {
		// incomplete ipv6 literal; leave as-is
		return target
	}
	return net.JoinHostPort(target, "443")
}

// updateLatency probes all targets and fills Stats + Info latency fields.
func (a *Agent) updateLatency(systemStats *system.Stats) {
	if a.latencyManager == nil {
		return
	}
	avg, targets := a.latencyManager.probe()
	if avg <= 0 && len(targets) == 0 {
		return
	}
	systemStats.Latency = avg
	systemStats.LatencyTargets = targets
	a.systemInfo.Latency = avg
	a.systemInfo.LatencyTargets = targets
}

func (lm *latencyManager) probe() (avg float64, results map[string]float64) {
	lm.mu.Lock()
	targets := append([]namedTarget(nil), lm.targets...)
	timeout := lm.timeout
	lm.mu.Unlock()

	if len(targets) == 0 {
		return 0, nil
	}

	type result struct {
		name string
		ms   float64
		ok   bool
	}

	ch := make(chan result, len(targets))
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t namedTarget) {
			defer wg.Done()
			ms, err := tcpConnectLatency(t.Addr, timeout)
			if err != nil {
				slog.Debug("Latency probe failed", "name", t.Name, "addr", t.Addr, "err", err)
				ch <- result{name: t.Name, ok: false}
				return
			}
			ch <- result{name: t.Name, ms: ms, ok: true}
		}(target)
	}
	wg.Wait()
	close(ch)

	results = make(map[string]float64, len(targets))
	var sum float64
	var n int
	for r := range ch {
		if !r.ok {
			continue
		}
		results[r.name] = r.ms
		sum += r.ms
		n++
	}
	if n == 0 {
		return 0, nil
	}
	return utils.TwoDecimals(sum / float64(n)), results
}

func tcpConnectLatency(target string, timeout time.Duration) (float64, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(context.Background(), "tcp", target)
	if err != nil {
		return 0, err
	}
	ms := float64(time.Since(start).Microseconds()) / 1000.0
	_ = conn.Close()
	return utils.TwoDecimals(ms), nil
}
