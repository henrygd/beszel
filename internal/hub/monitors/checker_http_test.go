package monitors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func httpTestMonitor(target string) Monitor {
	return Monitor{
		Name:            "http-test",
		Type:            TypeHTTP,
		Target:          target,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
		Config:          map[string]any{},
	}
}

func allowPrivateNet(t *testing.T) {
	t.Helper()
	t.Setenv(allowPrivateNetworkEnv, "true")
}

func TestHTTPParseAcceptedStatusCodes(t *testing.T) {
	allowPrivateNet(t)
	cases := []struct {
		name    string
		spec    string
		accept  []int
		reject  []int
		wantErr bool
	}{
		{"default range", "200-299", []int{200, 201, 250, 299}, []int{199, 300, 404, 500}, false},
		{"list", "200,201,204", []int{200, 201, 204}, []int{202, 203, 404}, false},
		{"mixed with spaces", " 200 , 201-203 , 299 ", []int{200, 202, 203, 299}, []int{204, 404}, false},
		{"single", "200", []int{200}, []int{201, 404}, false},
		{"full range", "100-599", []int{100, 404, 599}, []int{99, 600}, false},
		{"invalid token", "abc", nil, nil, true},
		{"invalid range order", "299-200", nil, nil, true},
		{"empty", "", nil, nil, true},
		{"blank", "   ", nil, nil, true},
		{"code too small", "99", nil, nil, true},
		{"code too large", "600", nil, nil, true},
		{"range with bad end", "200-abc", nil, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := parseAcceptedStatusCodes(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for spec %q, got nil", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for spec %q: %v", tc.spec, err)
			}
			for _, code := range tc.accept {
				if !matcher(code) {
					t.Errorf("spec %q should accept %d", tc.spec, code)
				}
			}
			for _, code := range tc.reject {
				if matcher(code) {
					t.Errorf("spec %q should reject %d", tc.spec, code)
				}
			}
		})
	}
}

func TestHTTPOK(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res := CheckHTTP(context.Background(), httpTestMonitor(srv.URL))
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
	if res.Code == nil || *res.Code != 200 {
		t.Fatalf("expected code 200, got %+v", res.Code)
	}
	if res.LatencyMs <= 0 {
		t.Fatalf("expected positive latency, got %v", res.LatencyMs)
	}
	if res.Details["final_url"] != srv.URL {
		t.Fatalf("expected final_url %q, got %v", srv.URL, res.Details["final_url"])
	}
}

func TestHTTPStatusNotAccepted(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	res := CheckHTTP(context.Background(), httpTestMonitor(srv.URL))
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if res.Code == nil || *res.Code != 500 {
		t.Fatalf("expected code 500, got %+v", res.Code)
	}
	if !strings.Contains(res.Message, "status 500 not accepted") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPCustomAcceptedCodes(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/created" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	accepted := httpTestMonitor(srv.URL + "/created")
	accepted.Config["accepted_status_codes"] = "200,201,204"
	if res := CheckHTTP(context.Background(), accepted); res.Status != StatusUp {
		t.Fatalf("expected up for custom accepted code, got %q (%s)", res.Status, res.Message)
	}

	rejected := httpTestMonitor(srv.URL + "/missing")
	rejected.Config["accepted_status_codes"] = "200, 201-203"
	res := CheckHTTP(context.Background(), rejected)
	if res.Status != StatusDown {
		t.Fatalf("expected down for non-accepted code, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "status 404 not accepted") {
		t.Fatalf("unexpected message %q", res.Message)
	}

	bad := httpTestMonitor(srv.URL)
	bad.Config["accepted_status_codes"] = "bogus"
	if res := CheckHTTP(context.Background(), bad); res.Status != StatusDown {
		t.Fatalf("expected down for invalid spec, got %q", res.Status)
	}
}

func TestHTTPRedirectFollowed(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("done"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()

	res := CheckHTTP(context.Background(), httpTestMonitor(srv.URL))
	if res.Status != StatusUp {
		t.Fatalf("expected up after redirect, got %q (%s)", res.Status, res.Message)
	}
	if res.Details["final_url"] != srv.URL+"/final" {
		t.Fatalf("expected final_url %q, got %v", srv.URL+"/final", res.Details["final_url"])
	}
}

func TestHTTPRedirectNotFollowed(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["follow_redirects"] = false
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down for unfollowed redirect, got %q", res.Status)
	}
	if res.Code == nil || *res.Code != 302 {
		t.Fatalf("expected code 302, got %+v", res.Code)
	}
	if res.Details["final_url"] != srv.URL {
		t.Fatalf("expected final_url %q, got %v", srv.URL, res.Details["final_url"])
	}
}

func TestHTTPRedirectCap(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL + "/loop")
	m.Config["max_redirects"] = 1
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down when redirect cap hit, got %q", res.Status)
	}
	if res.Code == nil || *res.Code != 302 {
		t.Fatalf("expected code 302, got %+v", res.Code)
	}
	if !strings.Contains(res.Message, "redirect") {
		t.Fatalf("expected redirect message, got %q", res.Message)
	}
}

func TestHTTPRedirectPrivateHopBlocked(t *testing.T) {
	t.Setenv(allowPrivateNetworkEnv, "")
	if err := redirectHostAllowed(context.Background(), "127.0.0.1"); err == nil {
		t.Fatal("expected private redirect hop to be blocked")
	}
	if err := redirectHostAllowed(context.Background(), "localhost"); err == nil {
		t.Fatal("expected loopback redirect hop to be blocked")
	}
	t.Setenv(allowPrivateNetworkEnv, "true")
	if err := redirectHostAllowed(context.Background(), "127.0.0.1"); err != nil {
		t.Fatalf("expected override to allow private hop, got %v", err)
	}
	if err := redirectHostAllowed(context.Background(), "no-such-host.invalid."); err == nil {
		t.Fatal("expected unresolvable redirect hop to be blocked")
	}
}

func TestHTTPKeywordFound(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello beszel world"))
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Type = TypeKeyword
	m.Config["keyword"] = "beszel"
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
	if found, _ := res.Details["keyword_found"].(bool); !found {
		t.Fatalf("expected keyword_found=true, got %v", res.Details)
	}
	if res.Message != "keyword found" {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPKeywordNotFound(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("nothing to see here"))
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["keyword"] = "beszel"
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down, got %q", res.Status)
	}
	if res.Message != "keyword not found" {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPKeywordInverted(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("beszel is here"))
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["keyword"] = "beszel"
	m.Config["invert_keyword"] = true
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down for inverted match, got %q", res.Status)
	}
	if res.Message != "keyword found (inverted)" {
		t.Fatalf("unexpected message %q", res.Message)
	}

	m.Config["keyword"] = "absent-word"
	res = CheckHTTP(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up for inverted absence, got %q (%s)", res.Status, res.Message)
	}
}

func TestHTTPKeywordRequired(t *testing.T) {
	allowPrivateNet(t)
	m := httpTestMonitor("http://example.com")
	m.Type = TypeKeyword
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down without keyword, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "keyword is required") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPTimeout(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.TimeoutSeconds = 1
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down on timeout, got %q", res.Status)
	}
	if res.Code != nil {
		t.Fatalf("expected nil code on timeout, got %d", *res.Code)
	}
	if !strings.Contains(res.Message, "timed out") {
		t.Fatalf("expected timeout message, got %q", res.Message)
	}
}

func TestHTTPTruncatedBody(t *testing.T) {
	allowPrivateNet(t)
	big := "KEY:" + strings.Repeat("a", (2<<20)+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["keyword"] = "KEY"
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
	if res.Details["truncated"] != true {
		t.Fatalf("expected truncated=true, got %v", res.Details)
	}
}

func TestHTTPTruncatedKeywordBeyondCutoff(t *testing.T) {
	allowPrivateNet(t)
	big := strings.Repeat("a", 2<<20) + "TAILKEY"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["keyword"] = "TAILKEY"
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down for keyword past cutoff, got %q", res.Status)
	}
	if res.Message != "keyword not found" {
		t.Fatalf("unexpected message %q", res.Message)
	}
	if res.Details["truncated"] != true {
		t.Fatalf("expected truncated=true, got %v", res.Details)
	}
}

func TestHTTPBasicAuth(t *testing.T) {
	allowPrivateNet(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if u, p, ok := r.BasicAuth(); !ok || u != "u" || p != "p" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["auth_type"] = "basic"
	m.Config["username"] = "u"
	m.Config["password"] = "p"
	if res := CheckHTTP(context.Background(), m); res.Status != StatusUp {
		t.Fatalf("expected up with valid basic auth, got %q (%s)", res.Status, res.Message)
	}

	missing := httpTestMonitor(srv.URL)
	missing.Config["auth_type"] = "basic"
	missing.Config["username"] = "u"
	before := hits
	res := CheckHTTP(context.Background(), missing)
	if res.Status != StatusDown {
		t.Fatalf("expected down with missing password, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "username and password") {
		t.Fatalf("unexpected message %q", res.Message)
	}
	if res.Code != nil {
		t.Fatalf("expected nil code when validation fails before request, got %d", *res.Code)
	}
	if hits != before {
		t.Fatal("no request should be sent when auth config is invalid")
	}
}

func TestHTTPBearerAuth(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer s3cr3t-bearer-xyz" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["auth_type"] = "bearer"
	m.Config["token"] = "s3cr3t-bearer-xyz"
	if res := CheckHTTP(context.Background(), m); res.Status != StatusUp {
		t.Fatalf("expected up with valid bearer token, got %q (%s)", res.Status, res.Message)
	}

	missing := httpTestMonitor(srv.URL)
	missing.Config["auth_type"] = "bearer"
	res := CheckHTTP(context.Background(), missing)
	if res.Status != StatusDown {
		t.Fatalf("expected down with missing token, got %q", res.Status)
	}
	if strings.Contains(res.Message, "s3cr3t-bearer-xyz") {
		t.Fatalf("message must never contain secrets: %q", res.Message)
	}

	unknown := httpTestMonitor(srv.URL)
	unknown.Config["auth_type"] = "digest"
	if res := CheckHTTP(context.Background(), unknown); res.Status != StatusDown {
		t.Fatalf("expected down for unknown auth_type, got %q", res.Status)
	}
}

func TestHTTPForbiddenHeader(t *testing.T) {
	allowPrivateNet(t)
	for _, h := range []string{"Host", "host", "Content-Length"} {
		m := httpTestMonitor("http://example.com")
		m.Config["headers"] = map[string]any{h: "evil"}
		res := CheckHTTP(context.Background(), m)
		if res.Status != StatusDown {
			t.Fatalf("expected down for header %q, got %q", h, res.Status)
		}
		if !strings.Contains(res.Message, "forbidden header") {
			t.Fatalf("unexpected message %q", res.Message)
		}
	}
}

func TestHTTPTooManyHeaders(t *testing.T) {
	allowPrivateNet(t)
	headers := map[string]any{}
	for i := 0; i < 21; i++ {
		headers[fmt.Sprintf("X-Test-%d", i)] = "v"
	}
	m := httpTestMonitor("http://example.com")
	m.Config["headers"] = headers
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down for too many headers, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "too many headers") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPHeaderKeyTooLong(t *testing.T) {
	allowPrivateNet(t)
	m := httpTestMonitor("http://example.com")
	m.Config["headers"] = map[string]any{strings.Repeat("X", 101): "v"}
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down for oversized header name, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "too long") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPBodyTooLarge(t *testing.T) {
	allowPrivateNet(t)
	m := httpTestMonitor("http://example.com")
	m.Config["body"] = strings.Repeat("x", (1<<20)+1)
	if res := CheckHTTP(context.Background(), m); res.Status != StatusDown {
		t.Fatalf("expected down for oversized body, got %q", res.Status)
	}
}

func TestHTTPPostBody(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		buf := make([]byte, 16)
		n, _ := r.Body.Read(buf)
		if string(buf[:n]) != "hello" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["method"] = "POST"
	m.Config["body"] = "hello"
	m.Config["content_type"] = "text/plain"
	if res := CheckHTTP(context.Background(), m); res.Status != StatusUp {
		t.Fatalf("expected up for POST with body, got %q (%s)", res.Status, res.Message)
	}

	lower := httpTestMonitor(srv.URL)
	lower.Config["method"] = "post"
	lower.Config["body"] = "hello"
	lower.Config["content_type"] = "text/plain"
	if res := CheckHTTP(context.Background(), lower); res.Status != StatusUp {
		t.Fatalf("expected lowercase method to be accepted, got %q (%s)", res.Status, res.Message)
	}

	bad := httpTestMonitor(srv.URL)
	bad.Config["method"] = "FETCH"
	res := CheckHTTP(context.Background(), bad)
	if res.Status != StatusDown {
		t.Fatalf("expected down for invalid method, got %q", res.Status)
	}
	if !strings.Contains(res.Message, "invalid method") {
		t.Fatalf("unexpected message %q", res.Message)
	}
}

func TestHTTPUserAgent(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ua:" + r.UserAgent()))
	}))
	defer srv.Close()

	def := httpTestMonitor(srv.URL)
	def.Config["keyword"] = "ua:Beszel-Monitor"
	if res := CheckHTTP(context.Background(), def); res.Status != StatusUp {
		t.Fatalf("expected default User-Agent, got %q (%s)", res.Status, res.Message)
	}

	custom := httpTestMonitor(srv.URL)
	custom.Config["keyword"] = "ua:Custom/1.0"
	custom.Config["headers"] = map[string]any{"User-Agent": "Custom/1.0"}
	if res := CheckHTTP(context.Background(), custom); res.Status != StatusUp {
		t.Fatalf("expected custom User-Agent override, got %q (%s)", res.Status, res.Message)
	}
}

func TestHTTPIgnoreTLSErrors(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	strict := httpTestMonitor(srv.URL)
	if res := CheckHTTP(context.Background(), strict); res.Status != StatusDown {
		t.Fatalf("expected down for untrusted cert, got %q", res.Status)
	}

	relaxed := httpTestMonitor(srv.URL)
	relaxed.Config["ignore_tls_errors"] = true
	res := CheckHTTP(context.Background(), relaxed)
	if res.Status != StatusUp {
		t.Fatalf("expected up with ignore_tls_errors, got %q (%s)", res.Status, res.Message)
	}
	if res.Details["tls_insecure"] != true {
		t.Fatalf("expected tls_insecure=true, got %v", res.Details)
	}
}

func TestHTTPCertExpirySkipped(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.Config["check_cert_expiry"] = true
	res := CheckHTTP(context.Background(), m)
	if res.Status != StatusUp {
		t.Fatalf("expected up, got %q (%s)", res.Status, res.Message)
	}
	if res.Details["cert_skipped"] != "tls checker pending (task 4)" {
		t.Fatalf("expected cert_skipped note, got %v", res.Details)
	}
}

func TestHTTPUpsideDownIgnored(t *testing.T) {
	allowPrivateNet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := httpTestMonitor(srv.URL)
	m.UpsideDown = true
	if res := CheckHTTP(context.Background(), m); res.Status != StatusUp {
		t.Fatalf("expected raw up status (upside_down applied by task 7), got %q", res.Status)
	}
}

func TestHTTPInvalidScheme(t *testing.T) {
	allowPrivateNet(t)
	for _, target := range []string{
		"file:///etc/passwd",
		"ftp://example.com/file",
		"gopher://example.com/",
		"http://",
		"://bad",
		"",
		"notaurl",
	} {
		res := CheckHTTP(context.Background(), httpTestMonitor(target))
		if res.Status != StatusDown {
			t.Errorf("expected down for target %q, got %q", target, res.Status)
		}
		if res.Code != nil {
			t.Errorf("expected nil code for target %q, got %d", target, *res.Code)
		}
	}
}
