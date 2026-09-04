package monitors

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func pingTestMonitor(target string) Monitor {
	return Monitor{
		Name:            "ping-test",
		Type:            TypePing,
		Target:          target,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
		Config:          map[string]any{},
	}
}

// skipIfPingUnavailable skips explicitly when the sandbox lacks the NET_RAW
// capability (and ping_group_range): neither unprivileged nor privileged ping
// can run here, so live-ping assertions cannot execute.
func skipIfPingUnavailable(t *testing.T, res CheckResult) {
	t.Helper()
	if res.Status == StatusDown && strings.Contains(res.Message, "missing NET_RAW capability") {
		t.Skipf("skipping live ping assertions: %s", res.Message)
	}
}

func TestPingEmptyTarget(t *testing.T) {
	for _, target := range []string{"", "   "} {
		m := pingTestMonitor(target)
		res := CheckPing(context.Background(), m)
		if res.Status != StatusDown {
			t.Errorf("target %q: expected down, got %q", target, res.Status)
		}
		if res.Message == "" {
			t.Errorf("target %q: expected non-empty down message", target)
		}
	}
}

func TestPingInvalidCount(t *testing.T) {
	for _, count := range []any{0, -1, 11} {
		m := pingTestMonitor("192.0.2.1")
		m.Config["count"] = count
		res := CheckPing(context.Background(), m)
		if res.Status != StatusDown {
			t.Errorf("count %v: expected down, got %q", count, res.Status)
		}
		if !strings.Contains(res.Message, "invalid count") {
			t.Errorf("count %v: expected 'invalid count' message, got %q", count, res.Message)
		}
	}
}

func TestPingInvalidPacketSize(t *testing.T) {
	for _, size := range []any{-1, 65401} {
		m := pingTestMonitor("192.0.2.1")
		m.Config["packet_size"] = size
		res := CheckPing(context.Background(), m)
		if res.Status != StatusDown {
			t.Errorf("packet_size %v: expected down, got %q", size, res.Status)
		}
		if !strings.Contains(res.Message, "invalid packet_size") {
			t.Errorf("packet_size %v: expected 'invalid packet_size' message, got %q", size, res.Message)
		}
	}
}

func TestPingInvalidPacketTimeout(t *testing.T) {
	// TimeoutSeconds is 5: zero, negative, unparseable-equivalent and values
	// above the global timeout are all rejected before any socket is opened.
	for _, pt := range []any{0, -1, "0", 30} {
		m := pingTestMonitor("192.0.2.1")
		m.Config["packet_timeout"] = pt
		res := CheckPing(context.Background(), m)
		if res.Status != StatusDown {
			t.Errorf("packet_timeout %v: expected down, got %q", pt, res.Status)
		}
		if !strings.Contains(res.Message, "invalid packet_timeout") {
			t.Errorf("packet_timeout %v: expected 'invalid packet_timeout' message, got %q", pt, res.Message)
		}
	}
}

func TestPingInvalidInterval(t *testing.T) {
	// Minimum is 200ms: zero, sub-200ms numerics and duration strings below
	// the floor are rejected.
	for _, iv := range []any{0, 0.1, "10ms", -1} {
		m := pingTestMonitor("192.0.2.1")
		m.Config["interval_between_packets"] = iv
		res := CheckPing(context.Background(), m)
		if res.Status != StatusDown {
			t.Errorf("interval %v: expected down, got %q", iv, res.Status)
		}
		if !strings.Contains(res.Message, "invalid interval_between_packets") {
			t.Errorf("interval %v: expected 'invalid interval_between_packets' message, got %q", iv, res.Message)
		}
	}
}

func TestPingUnresolvableHost(t *testing.T) {
	m := pingTestMonitor("does-not-exist.invalid")
	res := CheckPing(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if !strings.HasPrefix(res.Message, "cannot resolve host") {
		t.Fatalf("expected 'cannot resolve host ...' message, got %q", res.Message)
	}
}

func TestPingPrivateTargetBlocked(t *testing.T) {
	t.Setenv(allowPrivateNetworkEnv, "")
	m := pingTestMonitor("10.0.0.1")
	res := CheckPing(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "private network") {
		t.Fatalf("expected private-network guard message, got %q", res.Message)
	}
}

func TestPingPrivateTargetAllowedWithOverride(t *testing.T) {
	allowPrivateNet(t)
	m := pingTestMonitor("10.0.0.1")
	m.Config["count"] = 1
	m.Config["packet_timeout"] = 1
	res := CheckPing(context.Background(), m)
	// The override must let the check proceed past the SSRF guard: any
	// outcome is fine except the private-network block.
	if strings.Contains(res.Message, "private network") {
		t.Fatalf("override set but guard still blocked: %q", res.Message)
	}
}

func TestPingLoopback(t *testing.T) {
	allowPrivateNet(t)
	m := pingTestMonitor("127.0.0.1")
	m.Config["count"] = 1
	res := CheckPing(context.Background(), m)
	skipIfPingUnavailable(t, res)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%q)", res.Status, res.Message)
	}
	if res.Message != "1/1 packets received" {
		t.Errorf("expected '1/1 packets received', got %q", res.Message)
	}
	if res.Code != nil {
		t.Errorf("expected nil code, got %d", *res.Code)
	}
	if res.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %v", res.LatencyMs)
	}
	if res.Details["sent"] != 1 || res.Details["received"] != 1 {
		t.Errorf("expected sent=1 received=1, got details %v", res.Details)
	}
	for _, k := range []string{"min_ms", "avg_ms", "max_ms", "loss_pct"} {
		if _, ok := res.Details[k].(float64); !ok {
			t.Errorf("expected float64 detail %q, got %v", k, res.Details)
		}
	}
}

func TestPingUnreachableLoss(t *testing.T) {
	// TEST-NET-1 is reserved and unroutable: no external network needed.
	// count=1 + packet_timeout=1 keeps this test to ~1-2s.
	m := pingTestMonitor("192.0.2.1")
	m.Config["count"] = 1
	m.Config["packet_timeout"] = 1
	start := time.Now()
	res := CheckPing(context.Background(), m)
	elapsed := time.Since(start)
	t.Logf("unreachable ping took %s: status=%s msg=%q", elapsed, res.Status, res.Message)
	if elapsed > 10*time.Second {
		t.Errorf("unreachable check took too long: %s", elapsed)
	}
	if strings.Contains(res.Message, "missing NET_RAW capability") {
		t.Skipf("skipping loss assertions: %s", res.Message)
	}
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "packet loss 100%") {
		t.Fatalf("expected typed 100%% loss message, got %q", res.Message)
	}
}

func TestPingUpsideDownIgnored(t *testing.T) {
	allowPrivateNet(t)
	m := pingTestMonitor("127.0.0.1")
	m.Config["count"] = 1
	m.UpsideDown = true
	res := CheckPing(context.Background(), m)
	skipIfPingUnavailable(t, res)
	// upside_down inversion is Task 7's job: a reachable host stays up here.
	if res.Status != StatusUp {
		t.Fatalf("expected up (upside_down ignored), got %q (%q)", res.Status, res.Message)
	}
}

func TestPingPermissionErrorClassifier(t *testing.T) {
	yes := []string{
		"socket: permission denied",
		"listen ip4:icmp 0.0.0.0: socket: operation not permitted",
		"Operation Not Permitted",
		"missing CAP_NET_RAW capability",
		"privileged ping requires root",
		"could not create socket",
	}
	for _, msg := range yes {
		if !isPingPermissionError(errors.New(msg)) {
			t.Errorf("expected permission error for %q", msg)
		}
	}
	no := []error{
		nil,
		errors.New("no route to host"),
		errors.New("i/o timeout"),
		errors.New("cannot resolve host: no such host"),
		errors.New("no reply, packet loss 100%"),
	}
	for _, err := range no {
		if isPingPermissionError(err) {
			t.Errorf("expected non-permission error for %v", err)
		}
	}
}

func TestPingUnavailableMessage(t *testing.T) {
	if !strings.Contains(pingUnavailableMsg, "missing NET_RAW capability") {
		t.Fatalf("unavailable message must contain 'missing NET_RAW capability', got %q", pingUnavailableMsg)
	}
}
