package alerts

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nicholas-fedor/shoutrrr/pkg/types"
	"golang.org/x/net/dns/dnsmessage"
)

func TestCheckNotificationAddress(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.0", "100.127.255.255", "0.0.0.0", "224.0.0.1", "255.255.255.255", "::1", "::", "fc00::1", "fe80::1", "ff02::1", "::ffff:127.0.0.1", "::ffff:169.254.169.254", "fe80::1%lo", "localhost", "consul"} {
		t.Run(host, func(t *testing.T) {
			if err := checkNotificationAddress(net.JoinHostPort(host, "80")); !errors.Is(err, errInternalDestination) {
				t.Fatalf("expected blocked address, got %v", err)
			}
		})
	}
	for _, host := range []string{"8.8.8.8", "100.63.255.255", "100.128.0.0", "2001:4860:4860::8888"} {
		if err := checkNotificationAddress(net.JoinHostPort(host, "443")); err != nil {
			t.Errorf("public address %s: %v", host, err)
		}
	}
}

func TestPublicNotificationBlocksInternalRequests(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	for _, rawURL := range []string{
		"generic+http://" + host,
		"generic+https://" + host,
		"generic+http://localhost:" + strings.Split(host, ":")[1],
		"matrix://user:password@" + host + "/room?disabletls=yes",
		"mattermost://" + host + "/token?disabletls=yes",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if err := sendPublicNotification(rawURL, "test"); !errors.Is(err, errInternalDestination) {
				t.Fatalf("expected internal destination error, got %v", err)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatal("internal server received a request")
	}

}

type notificationRoundTripper func(*http.Request) (*http.Response, error)

func (f notificationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublicNotificationRedirect(t *testing.T) {
	client := newPublicNotificationClient()
	defer client.CloseIdleConnections()
	transport := client.Transport
	client.Transport = notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "public.example" {
			return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"http://127.0.0.1/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return transport.RoundTrip(r)
	})
	_, err := client.Get("http://public.example/")
	if !errors.Is(err, errInternalDestination) {
		t.Fatalf("expected redirect to be blocked, got %v", err)
	}
}

func TestPublicNotificationServiceClient(t *testing.T) {
	for _, rawURL := range []string{"generic+http://public.example/path", "discord://token@123456789", "slack://hook:AAAAAAAAA-BBBBBBBBB-123456789123456789123456@webhook"} {
		t.Run(rawURL, func(t *testing.T) {
			var hits int
			client := &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
				hits++
				body := `{"ok":true}`
				if strings.HasPrefix(rawURL, "slack:") {
					body = "ok"
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			service, err := newPublicNotificationService(rawURL, types.SenderOptions{HTTPClient: client})
			if err != nil {
				t.Fatal(err)
			}
			if err := service.Send("test", &types.Params{}); err != nil {
				t.Fatal(err)
			}
			if hits == 0 {
				t.Fatal("injected client was not used")
			}
		})
	}
}

func TestPublicNotificationDNS(t *testing.T) {
	// Supply deterministic DNS responses over an in-memory TCP connection.
	// The first lookup sees a public IP; subsequent lookups see loopback.
	var rebound atomic.Bool
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var size [2]byte
			if _, err := io.ReadFull(server, size[:]); err != nil {
				return
			}
			buf := make([]byte, binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(server, buf); err != nil {
				return
			}
			var msg dnsmessage.Message
			if err := msg.Unpack(buf); err != nil {
				return
			}
			msg.Header.Response = true
			msg.Header.RecursionAvailable = true
			q := msg.Questions[0]
			if q.Type == dnsmessage.TypeA {
				ip := [4]byte{8, 8, 8, 8}
				if rebound.Load() {
					ip = [4]byte{127, 0, 0, 1}
				}
				msg.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: q.Type, Class: dnsmessage.ClassINET}, Body: &dnsmessage.AResource{A: ip}}}
			}
			buf, err := msg.Pack()
			if err != nil {
				return
			}
			binary.BigEndian.PutUint16(size[:], uint16(len(buf)))
			server.Write(append(size[:], buf...))
		}()
		return client, nil
	}}
	// These tests do not run in parallel; restore the process resolver afterward.
	previous := net.DefaultResolver
	net.DefaultResolver = resolver
	t.Cleanup(func() { net.DefaultResolver = previous })
	ips, err := resolver.LookupIP(context.Background(), "ip4", "rebind.example")
	if err != nil || len(ips) != 1 || !ips[0].Equal(net.IPv4(8, 8, 8, 8)) {
		t.Fatalf("initial DNS lookup: %v, %v", ips, err)
	}
	rebound.Store(true)
	client := newPublicNotificationClient()
	defer client.CloseIdleConnections()
	for _, host := range []string{"rebind.example", "consul"} {
		guarded := &notificationClient{Client: client}
		conn, dialErr := guarded.dialContext(context.Background(), "tcp", net.JoinHostPort(host, "25"))
		if conn != nil {
			conn.Close()
		}
		if !errors.Is(dialErr, errInternalDestination) || !guarded.blocked.Load() {
			t.Errorf("expected TCP dial-time rejection for %s, got %v", host, dialErr)
		}
		_, err := client.Get("http://" + host + "/")
		if !errors.Is(err, errInternalDestination) {
			t.Errorf("expected dial-time rejection for %s, got %v", host, err)
		}
	}
}

func TestPublicNotificationTCP(t *testing.T) {
	for _, rawURL := range []string{
		"smtp://user:pass@HOST:25/?fromAddress=sender@example.com&toAddresses=recipient@example.com",
		"smtp://user:pass@HOST:465/?fromAddress=sender@example.com&toAddresses=recipient@example.com",
		"mqtt://HOST:1883/topic",
		"mqtts://HOST:8883/topic",
	} {
		t.Run(rawURL, func(t *testing.T) {
			t.Parallel()
			t.Run("internal destination", func(t *testing.T) {
				err := sendPublicNotification(strings.ReplaceAll(rawURL, "HOST", "127.0.0.1"), "test")
				if !errors.Is(err, errInternalDestination) {
					t.Fatalf("expected blocked destination, got %v", err)
				}
			})
			t.Run("public destination uses injected dialer", func(t *testing.T) {
				var calls atomic.Int32
				stopped := errors.New("test dial stopped")
				service, err := newPublicNotificationService(strings.ReplaceAll(rawURL, "HOST", "8.8.8.8"), types.SenderOptions{
					DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
						calls.Add(1)
						if network != "tcp" || !strings.HasPrefix(address, "8.8.8.8:") {
							t.Errorf("unexpected dial: %s %s", network, address)
						}
						if err := checkNotificationAddress(address); err != nil {
							t.Error(err)
						}
						return nil, stopped
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if closer, ok := service.(io.Closer); ok {
					defer closer.Close()
				}
				if err := service.Send("test", &types.Params{}); err == nil {
					t.Fatal("expected dial failure")
				}
				if calls.Load() == 0 {
					t.Fatal("custom dialer was not used")
				}
			})
		})
	}
}
