package monitors

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// tlsTestPKI is a throwaway CA used to issue server certificates for tests.
// No external network is involved: servers listen on 127.0.0.1.
type tlsTestPKI struct {
	ca   *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newTLSTestPKI(t *testing.T) *tlsTestPKI {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "tls-test-ca"},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &tlsTestPKI{ca: ca, key: key, pool: pool}
}

// serverCert issues a leaf certificate signed by the test CA.
func (p *tlsTestPKI) serverCert(t *testing.T, dns []string, ips []net.IP, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "tls-test-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dns,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.ca, &key.PublicKey, p.key)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}

// startTLSTestServer serves TLS handshakes on 127.0.0.1 and returns
// the host:port address. Callers must allow the private network.
func startTLSTestServer(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	t.Cleanup(func() { _ = tlsLn.Close() })
	go func() {
		for {
			c, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if tc, ok := c.(*tls.Conn); ok {
					_ = tc.Handshake()
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func tlsTestMonitor(target string) Monitor {
	return Monitor{
		Name:            "tls-test",
		Type:            TypeTLS,
		Target:          target,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
		Config:          map[string]any{},
	}
}

// withTLSTestRoots trusts the test CA for the duration of the test.
func withTLSTestRoots(t *testing.T, pki *tlsTestPKI) {
	t.Helper()
	old := tlsTestRoots
	tlsTestRoots = pki.pool
	t.Cleanup(func() { tlsTestRoots = old })
}

func TestCertDaysLeft(t *testing.T) {
	now := time.Now()
	future := &x509.Certificate{NotAfter: now.Add(30 * 24 * time.Hour)}
	if got := CertDaysLeft([]*x509.Certificate{future}, now); got < 29.9 || got > 30.1 {
		t.Fatalf("future cert: got %v days, want ~30", got)
	}
	past := &x509.Certificate{NotAfter: now.Add(-24 * time.Hour)}
	if got := CertDaysLeft([]*x509.Certificate{past}, now); got > -0.9 || got < -1.1 {
		t.Fatalf("expired cert: got %v days, want ~-1", got)
	}
	if got := CertDaysLeft(nil, now); got > -1e6 {
		t.Fatalf("empty chain: got %v, want a very negative value", got)
	}
	if got := CertDaysLeft([]*x509.Certificate{}, now); got > -1e6 {
		t.Fatalf("empty chain: got %v, want a very negative value", got)
	}
}

func TestTLSValidLongLived(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	leafNotAfter := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	dns := []string{"a.example", "b.example", "c.example", "d.example", "e.example", "f.example", "g.example"}
	cert := pki.serverCert(t, dns, []net.IP{net.ParseIP("127.0.0.1")}, leafNotAfter)
	addr := startTLSTestServer(t, cert)

	for _, target := range []string{"https://" + addr + "/some/path", addr} {
		res := CheckTLS(context.Background(), tlsTestMonitor(target))
		if res.Status != StatusUp {
			t.Fatalf("target %q: got status %q (%s), want up", target, res.Status, res.Message)
		}
		if !strings.Contains(res.Message, "certificate expires in") {
			t.Fatalf("target %q: unexpected message %q", target, res.Message)
		}
		if res.Code != nil {
			t.Fatalf("target %q: Code must be nil for TLS, got %d", target, *res.Code)
		}
		if res.CertDays == nil || *res.CertDays < 80 {
			t.Fatalf("target %q: CertDays = %v, want ~90", target, res.CertDays)
		}
		notAfter, ok := res.Details["not_after"].(string)
		if !ok {
			t.Fatalf("target %q: Details[not_after] missing", target)
		}
		if ts, err := time.Parse(time.RFC3339, notAfter); err != nil || ts.Unix() != leafNotAfter.Unix() {
			t.Fatalf("target %q: not_after = %q (err %v), want RFC3339 of leaf expiry", target, notAfter, err)
		}
		if res.Details["issuer"] != "tls-test-ca" {
			t.Fatalf("target %q: issuer = %v, want tls-test-ca", target, res.Details["issuer"])
		}
		names, ok := res.Details["dns_names"].([]string)
		if !ok || len(names) != 5 {
			t.Fatalf("target %q: dns_names = %v, want first 5 of 7", target, res.Details["dns_names"])
		}
	}
}

func TestTLSExpiringSoonWarn(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	cert := pki.serverCert(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(10*24*time.Hour))
	addr := startTLSTestServer(t, cert)

	res := CheckTLS(context.Background(), tlsTestMonitor(addr))
	if res.Status != StatusWarn {
		t.Fatalf("got status %q (%s), want warn", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "(warning)") {
		t.Fatalf("warn message must carry (warning) suffix, got %q", res.Message)
	}
	if res.CertDays == nil || *res.CertDays < 9 || *res.CertDays > 11 {
		t.Fatalf("CertDays = %v, want ~10", res.CertDays)
	}
}

func TestTLSExpiringCritDown(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	cert := pki.serverCert(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(3*24*time.Hour))
	addr := startTLSTestServer(t, cert)

	res := CheckTLS(context.Background(), tlsTestMonitor(addr))
	if res.Status != StatusDown {
		t.Fatalf("got status %q (%s), want down", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "certificate expires in") || strings.Contains(res.Message, "(warning)") {
		t.Fatalf("crit message must be plain expiry without (warning), got %q", res.Message)
	}
}

func TestTLSExpiredStrictDown(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	cert := pki.serverCert(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(-24*time.Hour))
	addr := startTLSTestServer(t, cert)

	res := CheckTLS(context.Background(), tlsTestMonitor(addr))
	if res.Status != StatusDown {
		t.Fatalf("got status %q (%s), want down", res.Status, res.Message)
	}
	if !strings.Contains(strings.ToLower(res.Message), "expired") {
		t.Fatalf("expired message must mention expiry, got %q", res.Message)
	}
}

func TestTLSHostnameMismatch(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	// Cert is valid but names another host: no IP SAN matches 127.0.0.1.
	cert := pki.serverCert(t, []string{"wrong.example"}, nil, time.Now().Add(90*24*time.Hour))
	addr := startTLSTestServer(t, cert)

	res := CheckTLS(context.Background(), tlsTestMonitor(addr))
	if res.Status != StatusDown {
		t.Fatalf("got status %q (%s), want down", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "hostname mismatch") {
		t.Fatalf("mismatch message must contain %q, got %q", "hostname mismatch", res.Message)
	}
}

func TestTLSServerNameOverride(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	cert := pki.serverCert(t, []string{"mytest.local"}, nil, time.Now().Add(90*24*time.Hour))
	addr := startTLSTestServer(t, cert)

	m := tlsTestMonitor(addr)
	m.Config["server_name"] = "mytest.local"
	res := CheckTLS(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("got status %q (%s), want up via server_name override", res.Status, res.Message)
	}
}

func TestTLSIgnoreErrorsSelfSigned(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t) // CA deliberately NOT trusted: no withTLSTestRoots.
	cert := pki.serverCert(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(90*24*time.Hour))
	addr := startTLSTestServer(t, cert)

	m := tlsTestMonitor(addr)
	m.Config["ignore_tls_errors"] = true
	res := CheckTLS(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("got status %q (%s), want up with ignore_tls_errors", res.Status, res.Message)
	}
	if res.Details["tls_insecure"] != true {
		t.Fatalf("tls_insecure detail must be true, got %v", res.Details["tls_insecure"])
	}
	if res.CertDays == nil {
		t.Fatalf("expiry must still be evaluated with ignore_tls_errors")
	}

	// Expiry is still enforced: an expired cert stays down even when ignored.
	expired := pki.serverCert(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(-24*time.Hour))
	expiredAddr := startTLSTestServer(t, expired)
	m2 := tlsTestMonitor(expiredAddr)
	m2.Config["ignore_tls_errors"] = true
	res2 := CheckTLS(context.Background(), m2)
	if res2.Status != StatusDown {
		t.Fatalf("got status %q (%s), want down for expired cert despite ignore", res2.Status, res2.Message)
	}
	if !strings.Contains(res2.Message, "certificate expired") {
		t.Fatalf("want explicit %q message, got %q", "certificate expired", res2.Message)
	}
}

func TestTLSPrivateBlockedByDefault(t *testing.T) {
	t.Setenv(allowPrivateNetworkEnv, "false")
	m := tlsTestMonitor("127.0.0.1:443")
	res := CheckTLS(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("got status %q (%s), want down for private target", res.Status, res.Message)
	}
	if !strings.Contains(strings.ToLower(res.Message), "private") {
		t.Fatalf("block message must mention the private network, got %q", res.Message)
	}
}

func TestTLSInvalidTargets(t *testing.T) {
	allowPrivateNet(t)
	for _, target := range []string{"", "   ", "https://", "127.0.0.1:99999", "127.0.0.1:0", "example.com:abc:def"} {
		res := CheckTLS(context.Background(), tlsTestMonitor(target))
		if res.Status != StatusDown {
			t.Fatalf("target %q: got status %q (%s), want down", target, res.Status, res.Message)
		}
	}
}

func TestTLSInvalidThresholds(t *testing.T) {
	allowPrivateNet(t)
	m := tlsTestMonitor("example.com")
	m.Config["warn_days"] = 7
	m.Config["crit_days"] = 7
	res := CheckTLS(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("got status %q (%s), want down for crit_days >= warn_days", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "crit_days") {
		t.Fatalf("validation message must mention crit_days, got %q", res.Message)
	}
}

func TestTLSUpsideDownNotApplied(t *testing.T) {
	allowPrivateNet(t)
	pki := newTLSTestPKI(t)
	withTLSTestRoots(t, pki)
	cert := pki.serverCert(t, nil, []net.IP{net.ParseIP("127.0.0.1")}, time.Now().Add(90*24*time.Hour))
	addr := startTLSTestServer(t, cert)

	m := tlsTestMonitor(addr)
	m.UpsideDown = true
	res := CheckTLS(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("upside_down must not be applied by CheckTLS: got %q (%s)", res.Status, res.Message)
	}
}
