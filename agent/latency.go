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

// latencyManager probes TCP connect RTT to configured targets (Nezha-style delay monitoring).
type latencyManager struct {
	mu          sync.Mutex
	targets     []string
	envTargets  []string // defaults from PING_TARGETS / HUB_URL
	hubOverride bool     // true when hub last sent ConfigureLatency
	timeout     time.Duration
}

func newLatencyManager() *latencyManager {
	envTargets := parsePingTargets()
	if len(envTargets) == 0 {
		envTargets = []string{defaultPingTarget}
	}
	lm := &latencyManager{
		timeout:    defaultPingTimeout,
		envTargets: envTargets,
		targets:    append([]string(nil), envTargets...),
	}
	slog.Info("Latency targets", "targets", lm.targets)
	return lm
}

// applyHubTargets updates targets from hub config.
// empty raw falls back to env defaults. Returns true if targets changed.
func (lm *latencyManager) applyHubTargets(raw string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var next []string
	if strings.TrimSpace(raw) == "" {
		next = append([]string(nil), lm.envTargets...)
		lm.hubOverride = false
	} else {
		next = normalizeTargets(strings.Split(raw, ","))
		if len(next) == 0 {
			next = append([]string(nil), lm.envTargets...)
			lm.hubOverride = false
		} else {
			lm.hubOverride = true
		}
	}

	if targetsEqual(lm.targets, next) {
		return false
	}
	lm.targets = next
	slog.Info("Latency targets updated", "targets", lm.targets, "source", map[bool]string{true: "hub", false: "env"}[lm.hubOverride])
	return true
}

func targetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parsePingTargets reads PING_TARGETS (comma-separated host or host:port).
// If unset, uses HUB_URL host (with its port or 443) plus the default public target.
func parsePingTargets() []string {
	if raw, ok := utils.GetEnv("PING_TARGETS"); ok {
		return normalizeTargets(strings.Split(raw, ","))
	}

	var targets []string
	if hubURL, ok := utils.GetEnv("HUB_URL"); ok {
		if t := targetFromURL(hubURL); t != "" {
			targets = append(targets, t)
		}
	}
	targets = append(targets, defaultPingTarget)
	return uniqueTargets(targets)
}

func normalizeTargets(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, ensureHostPort(p))
	}
	return uniqueTargets(out)
}

func uniqueTargets(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
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
}

func (lm *latencyManager) probe() (avg float64, results map[string]float64) {
	lm.mu.Lock()
	targets := append([]string(nil), lm.targets...)
	timeout := lm.timeout
	lm.mu.Unlock()

	if len(targets) == 0 {
		return 0, nil
	}

	type result struct {
		target string
		ms     float64
		ok     bool
	}

	ch := make(chan result, len(targets))
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			ms, err := tcpConnectLatency(t, timeout)
			if err != nil {
				slog.Debug("Latency probe failed", "target", t, "err", err)
				ch <- result{target: t, ok: false}
				return
			}
			ch <- result{target: t, ms: ms, ok: true}
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
		results[r.target] = r.ms
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
