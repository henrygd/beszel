package monitors

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func ssrfCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSSRFIsPrivateIPBlockedRanges(t *testing.T) {
	blocked := []string{
		// Loopback.
		"127.0.0.1", "127.255.255.255", "::1",
		// RFC 1918.
		"10.0.0.1", "10.255.255.255",
		"172.16.0.1", "172.31.255.255",
		"192.168.0.1", "192.168.255.255",
		// Cloud metadata.
		"169.254.169.254", "169.254.0.1",
		// IPv6 link-local / unique-local.
		"fe80::1", "fc00::1", "fd00::1",
		// Unspecified.
		"0.0.0.0", "::",
	}
	for _, s := range blocked {
		if !IsPrivateIP(net.ParseIP(s)) {
			t.Errorf("IsPrivateIP(%q) = false, want true", s)
		}
	}
}

func TestSSRFIsPrivateIPPublicOK(t *testing.T) {
	public := []string{
		"8.8.8.8", "1.1.1.1", "9.9.9.9", "93.184.216.34",
		"2001:4860:4860::8888", "2606:4700:4700::1111",
		// Boundaries just outside the blocked ranges.
		"172.15.255.255", "172.32.0.1", "11.0.0.1",
		"192.167.255.255", "169.253.255.255", "169.255.0.1",
	}
	for _, s := range public {
		if IsPrivateIP(net.ParseIP(s)) {
			t.Errorf("IsPrivateIP(%q) = true, want false", s)
		}
	}
}

func TestSSRFIsPrivateIPIgnoresEnvOverride(t *testing.T) {
	// The env override only relaxes GuardDialContext; the classifier stays pure.
	t.Setenv("MONITORS_ALLOW_PRIVATE_NETWORK", "true")
	if !IsPrivateIP(net.ParseIP("10.0.0.1")) {
		t.Error("IsPrivateIP(10.0.0.1) = false with override set, want true")
	}
}

func TestSSRFParsePortOrDefault(t *testing.T) {
	cases := []struct {
		name     string
		hostport string
		def      int
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{"host with port", "example.com:8080", 443, "example.com", 8080, false},
		{"bare host uses default", "example.com", 443, "example.com", 443, false},
		{"ipv4 with port", "127.0.0.1:80", 443, "127.0.0.1", 80, false},
		{"bracketed ipv6 with port", "[::1]:80", 443, "::1", 80, false},
		{"bare ipv6 uses default", "::1", 443, "::1", 443, false},
		{"non-numeric port", "example.com:http", 443, "", 0, true},
		{"port zero", "example.com:0", 443, "", 0, true},
		{"port too large", "example.com:99999", 443, "", 0, true},
		{"empty input", "", 443, "", 0, true},
		{"invalid default zero", "example.com", 0, "", 0, true},
		{"invalid default large", "example.com", 99999, "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, err := ParsePortOrDefault(tc.hostport, tc.def)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParsePortOrDefault(%q, %d) = no error, want error", tc.hostport, tc.def)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePortOrDefault(%q, %d) error: %v", tc.hostport, tc.def, err)
			}
			if host != tc.wantHost || port != tc.wantPort {
				t.Errorf("ParsePortOrDefault(%q, %d) = (%q, %d), want (%q, %d)",
					tc.hostport, tc.def, host, port, tc.wantHost, tc.wantPort)
			}
		})
	}
}

func TestSSRFGuardDialContextBlocksLiteralPrivateIP(t *testing.T) {
	// No env override: dialing a private literal must fail before connecting.
	conn, err := GuardDialContext(ssrfCtx(t), "tcp", "127.0.0.1:80")
	if err == nil {
		conn.Close()
		t.Fatal("GuardDialContext to 127.0.0.1:80 succeeded, want private-network block")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("error %q does not mention private network", err)
	}
}

func TestSSRFGuardDialContextBlocksResolvedLoopback(t *testing.T) {
	// Per-hop verification: a hostname resolving to loopback must be blocked.
	conn, err := GuardDialContext(ssrfCtx(t), "tcp", "localhost:80")
	if err == nil {
		conn.Close()
		t.Fatal("GuardDialContext to localhost:80 succeeded, want private-network block")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("error %q does not mention private network", err)
	}
}

func TestSSRFGuardDialContextRejectsBadAddress(t *testing.T) {
	for _, addr := range []string{"", "not-an-address", "[::1"} {
		if conn, err := GuardDialContext(ssrfCtx(t), "tcp", addr); err == nil {
			conn.Close()
			t.Errorf("GuardDialContext(%q) succeeded, want error", addr)
		}
	}
}

func TestSSRFGuardDialContextUnresolvableHost(t *testing.T) {
	if conn, err := GuardDialContext(ssrfCtx(t), "tcp", "does-not-exist.invalid:80"); err == nil {
		conn.Close()
		t.Error("GuardDialContext to unresolvable host succeeded, want error")
	}
}

func TestSSRFGuardDialContextLocalServerBlockedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := GuardDialContext(ssrfCtx(t), "tcp", u.Host); err == nil {
		conn.Close()
		t.Fatal("GuardDialContext to local httptest server succeeded without override, want block")
	}
}

func TestSSRFGuardDialContextLocalServerAllowedWithOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// Env is read on every call, so Setenv takes effect immediately.
	t.Setenv("MONITORS_ALLOW_PRIVATE_NETWORK", "true")
	conn, err := GuardDialContext(ssrfCtx(t), "tcp", u.Host)
	if err != nil {
		t.Fatalf("GuardDialContext with override failed: %v", err)
	}
	conn.Close()
}

func TestSSRFEnvReadOnEveryCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Default: blocked.
	if conn, err := GuardDialContext(ssrfCtx(t), "tcp", u.Host); err == nil {
		conn.Close()
		t.Fatal("expected block by default")
	}
	// Enabled: allowed.
	t.Setenv("MONITORS_ALLOW_PRIVATE_NETWORK", "true")
	conn, err := GuardDialContext(ssrfCtx(t), "tcp", u.Host)
	if err != nil {
		t.Fatalf("expected dial with override, got: %v", err)
	}
	conn.Close()
}
