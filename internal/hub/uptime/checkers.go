package uptime

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// maxBodyBytes limits how much response body we read for expected_body checks.
const maxBodyBytes = 64 * 1024

// pingID identifies our echo requests among other traffic.
var pingID = int16(os.Getpid() & 0x7fff)

var (
	httpClientOnce sync.Once
	secureClient   *http.Client
	insecureClient *http.Client
)

func initHTTPClients() {
	redirectLimit := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	secureClient = &http.Client{CheckRedirect: redirectLimit}
	insecureClient = &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // user opt-in per monitor
		CheckRedirect: redirectLimit,
	}
}

// monitorTarget returns the host and port parsed from the record.
func monitorTarget(rec *core.Record) (host string, port int) {
	host = strings.TrimSpace(rec.GetString("host"))
	port = rec.GetInt("port")
	// fall back to the url for http monitors without an explicit host
	if host == "" {
		if u, err := url.Parse(rec.GetString("url")); err == nil && u.Host != "" {
			host = u.Host
		}
	}
	return host, port
}

// runCheck performs a single check of the given monitor type and returns
// success, elapsed time in ms and an error message (empty on success).
func runCheck(ctx context.Context, rec *core.Record) (bool, int64, string) {
	switch rec.GetString("type") {
	case "http":
		return checkHTTP(ctx, rec)
	case "tcp":
		return checkTCP(ctx, rec)
	case "ping":
		return checkPing(ctx, rec)
	default:
		return false, 0, "Unknown monitor type"
	}
}

// checkHTTP performs an HTTP(S) check.
func checkHTTP(ctx context.Context, rec *core.Record) (bool, int64, string) {
	httpClientOnce.Do(initHTTPClients)

	target := strings.TrimSpace(rec.GetString("url"))
	if target == "" {
		return false, 0, "No URL provided"
	}
	// prepend scheme if missing
	if !strings.Contains(target, "://") {
		if rec.GetBool("secure") {
			target = "https://" + target
		} else {
			target = "http://" + target
		}
	}

	method := strings.ToUpper(strings.TrimSpace(rec.GetString("method")))
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return false, 0, "Invalid URL"
	}

	// custom headers (values stored as strings in the json field)
	var headers map[string]any
	if err := rec.UnmarshalJSONField("headers", &headers); err == nil {
		for k, v := range headers {
			if s, ok := v.(string); ok && s != "" {
				req.Header.Set(k, s)
			}
		}
	}

	client := secureClient
	if !rec.GetBool("secure") && strings.HasPrefix(target, "https://") {
		client = insecureClient
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return false, elapsed.Milliseconds(), err.Error()
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))

	// expected status: empty means 2xx/3xx, otherwise a status code prefix (e.g. "20")
	expectedStatus := strings.TrimSpace(rec.GetString("expected_status"))
	statusOK := true
	if expectedStatus == "" {
		statusOK = resp.StatusCode >= 200 && resp.StatusCode < 400
	} else {
		statusOK = strings.HasPrefix(strconv.Itoa(resp.StatusCode), expectedStatus)
	}
	if !statusOK {
		return false, elapsed.Milliseconds(), fmt.Sprintf("Status %d", resp.StatusCode)
	}

	expectedBody := strings.TrimSpace(rec.GetString("expected_body"))
	if expectedBody != "" && !strings.Contains(string(body), expectedBody) {
		return false, elapsed.Milliseconds(), "Expected body not found"
	}

	return true, elapsed.Milliseconds(), ""
}

// checkTCP performs a TCP port check.
func checkTCP(ctx context.Context, rec *core.Record) (bool, int64, string) {
	host, port := monitorTarget(rec)
	if host == "" || port == 0 {
		return false, 0, "No host or port provided"
	}

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	elapsed := time.Since(start)
	if err != nil {
		return false, elapsed.Milliseconds(), err.Error()
	}
	conn.Close()
	return true, elapsed.Milliseconds(), ""
}

// checkPing performs an ICMP echo check.
func checkPing(ctx context.Context, rec *core.Record) (bool, int64, string) {
	host, _ := monitorTarget(rec)
	if host == "" {
		return false, 0, "No host provided"
	}

	// resolve to IP addresses and ping each one
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return false, 0, "Could not resolve " + host
	}

	var lastErr string
	start := time.Now()
	for _, ip := range ips {
		up, elapsed, msg := pingIP(ctx, ip.String())
		if up {
			return true, elapsed, ""
		}
		if msg != "" {
			lastErr = msg
		}
	}
	return false, time.Since(start).Milliseconds(), lastErr
}

// pingConn opens an ICMP connection to the given IP. It prefers the unprivileged
// UDP pseudo-socket (works in containers) and falls back to a raw socket.
func pingConn(ctx context.Context, ip string) (net.PacketConn, error) {
	network := "udp4"
	if strings.Contains(ip, ":") {
		network = "udp6"
	}
	// unprivileged ping (ICMP over UDP) — works without root on Linux/Darwin
	if conn, err := icmp.ListenPacket(network, ""); err == nil {
		return conn, nil
	}
	// fallback: raw socket (requires root/CAP_NET_RAW)
	rawNetwork := "ip4:icmp"
	if strings.Contains(ip, ":") {
		rawNetwork = "ip6:ipv6-icmp"
	}
	return icmp.ListenPacket(rawNetwork, "")
}

// pingIP sends a single ICMP echo request to the given IP address and waits for the reply.
func pingIP(ctx context.Context, ip string) (bool, int64, string) {
	isIPv6 := strings.Contains(ip, ":")

	conn, err := pingConn(ctx, ip)
	if err != nil {
		return false, 0, "ICMP unavailable: " + err.Error()
	}
	defer conn.Close()

	echo := &icmp.Echo{ID: int(pingID), Seq: 1, Data: []byte("beszel")}
	var proto int
	var msgType icmp.Type
	if isIPv6 {
		proto = 58
		msgType = ipv6.ICMPTypeEchoRequest
	} else {
		proto = 1
		msgType = ipv4.ICMPTypeEcho
	}

	b, err := (&icmp.Message{Type: msgType, Body: echo}).Marshal(nil)
	if err != nil {
		return false, 0, err.Error()
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if _, err := conn.WriteTo(b, &net.IPAddr{IP: net.ParseIP(ip)}); err != nil {
		return false, 0, err.Error()
	}

	buf := make([]byte, 1500)
	start := time.Now()
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return false, time.Since(start).Milliseconds(), err.Error()
		}
		reply, err := icmp.ParseMessage(proto, buf[:n])
		if err != nil || reply == nil {
			continue
		}
		if body, ok := reply.Body.(*icmp.Echo); ok && body.ID == int(pingID) {
			return true, time.Since(start).Milliseconds(), ""
		}
	}
}
