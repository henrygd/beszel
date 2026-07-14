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
	targets []string
	timeout time.Duration
}

func newLatencyManager() *latencyManager {
	lm := &latencyManager{
		timeout: defaultPingTimeout,
		targets: parsePingTargets(),
	}
	if len(lm.targets) == 0 {
		lm.targets = []string{defaultPingTarget}
	}
	slog.Info("Latency targets", "targets", lm.targets)
	return lm
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
	if len(lm.targets) == 0 {
		return 0, nil
	}

	type result struct {
		target string
		ms     float64
		ok     bool
	}

	ch := make(chan result, len(lm.targets))
	var wg sync.WaitGroup
	for _, target := range lm.targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			ms, err := tcpConnectLatency(t, lm.timeout)
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

	results = make(map[string]float64, len(lm.targets))
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
