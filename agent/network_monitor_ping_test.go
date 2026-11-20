//go:build testing

package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/icmp"
)

// setICMPBurst shortens the echo burst for a test.
func setICMPBurst(t *testing.T, count int, spacing, replyTimeout time.Duration) {
	t.Helper()
	prevCount, prevSpacing, prevTimeout := icmpPingCount, icmpPingSpacing, icmpReplyTimeout
	icmpPingCount, icmpPingSpacing, icmpReplyTimeout = count, spacing, replyTimeout
	t.Cleanup(func() {
		icmpPingCount, icmpPingSpacing, icmpReplyTimeout = prevCount, prevSpacing, prevTimeout
	})
}

type testICMPPacketConn struct{}

func (testICMPPacketConn) Close() error { return nil }

type blockingICMPConn struct {
	net.PacketConn
	reading chan struct{}
}

func (c *blockingICMPConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	return len(p), nil
}

func (c *blockingICMPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	close(c.reading)
	return c.PacketConn.ReadFrom(p)
}

func TestMonitorICMPPacketCancellation(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()
	blocking := &blockingICMPConn{PacketConn: conn, reading: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		responses, err := monitorICMPPacket(ctx, blocking, &icmpV4, conn.LocalAddr())
		assert.Nil(t, responses)
		done <- err
	}()
	select {
	case <-blocking.reading:
	case <-time.After(time.Second):
		t.Fatal("probe did not begin reading")
	}
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt the socket read")
	}
}

func TestMonitorICMPExecCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell stub for ping")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ping"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := monitorICMPExec(ctx, "127.0.0.1", false)
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not terminate ping")
	}
}

func TestPingCommand(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin", "freebsd", "openbsd"} {
		for _, ipv6 := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ipv6=%t", goos, ipv6), func(t *testing.T) {
				target, family := "192.0.2.1", "-4"
				if ipv6 {
					target, family = "2001:db8::1", "-6"
				}
				name, args, err := pingCommand(goos, target, ipv6)
				require.NoError(t, err)
				wantName := "ping"
				wantArgs := []string{"-n", "-c", "5", target}
				switch goos {
				case "windows":
					wantArgs = []string{family, "-n", "5", "-w", "3000", target}
				case "linux":
					wantArgs = []string{family, "-n", "-c", "5", "-w", "7", target}
				default:
					if ipv6 {
						wantName = "ping6"
					}
				}
				assert.Equal(t, wantName, name)
				assert.Equal(t, wantArgs, args)
			})
		}
	}
	_, _, err := pingCommand("unsupported", "192.0.2.1", false)
	require.Error(t, err)
}

func TestParsePingResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		wantUs int64
	}{
		{"linux", "64 bytes from 192.0.2.1: icmp_seq=1 ttl=64 time=12.345 ms", 12345},
		{"bsd", "64 bytes from 192.0.2.1: icmp_seq=0 ttl=64 time=0.023 ms", 23},
		{"ipv6", "64 bytes from 2001:db8::1: icmp_seq=0 hlim=64 time=1.234 ms", 1234},
		{"windows", "Reply from 192.0.2.1: bytes=32 time=12ms TTL=128", 12000},
		{"windows submillisecond", "Reply from ::1: time<1ms", 1000},
		{"localized windows", "Antwort von 192.0.2.1: Bytes=32 Zeit=12ms TTL=128", 12000},
		{"decimal comma", "64 bytes from 192.0.2.1: time=1,234 ms", 1234},
		{"rounding", "time=0.1236 ms", 124},
		{"empty", "", -1},
		{"timeout", "Request timed out.", -1},
		{"unreachable", "Reply from 192.0.2.1: Destination host unreachable.", -1},
		{"malformed", "time=oops ms", -1},
		{"negative", "time=-1 ms", -1},
		{"overflow", "time=999999999999999999999 ms", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses, err := parsePingResponses([]byte(tc.output), 1)
			if tc.wantUs < 0 {
				require.Error(t, err)
				assert.Nil(t, responses)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []int64{tc.wantUs}, responses)
			}
		})
	}
}

func TestParsePingResponsesBurst(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   []int64
	}{
		{
			name: "linux partial loss",
			output: `PING 192.0.2.1 (192.0.2.1) 56(84) bytes of data.
64 bytes from 192.0.2.1: icmp_seq=1 ttl=64 time=1.10 ms
64 bytes from 192.0.2.1: icmp_seq=2 ttl=64 time=1.20 ms
64 bytes from 192.0.2.1: icmp_seq=2 ttl=64 time=1.25 ms (DUP!)
64 bytes from 192.0.2.1: icmp_seq=4 ttl=64 time=1.40 ms
64 bytes from 192.0.2.1: icmp_seq=5 ttl=64 time=1.50 ms

--- 192.0.2.1 ping statistics ---
5 packets transmitted, 4 received, +1 duplicates, 20% packet loss, time 4005ms
rtt min/avg/max/mdev = 1.100/1.300/1.500/0.158 ms
`,
			want: []int64{1100, 1200, 1400, 1500, -1},
		},
		{
			name: "macos",
			output: `PING 192.0.2.1 (192.0.2.1): 56 data bytes
64 bytes from 192.0.2.1: icmp_seq=0 ttl=64 time=2.001 ms
64 bytes from 192.0.2.1: icmp_seq=1 ttl=64 time=2.002 ms
64 bytes from 192.0.2.1: icmp_seq=2 ttl=64 time=2.003 ms
64 bytes from 192.0.2.1: icmp_seq=3 ttl=64 time=2.004 ms
64 bytes from 192.0.2.1: icmp_seq=4 ttl=64 time=2.005 ms

--- 192.0.2.1 ping statistics ---
5 packets transmitted, 5 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 2.001/2.003/2.005/0.001 ms
`,
			want: []int64{2001, 2002, 2003, 2004, 2005},
		},
		{
			name: "windows partial loss",
			output: "\r\nPinging 192.0.2.1 with 32 bytes of data:\r\n" +
				"Reply from 192.0.2.1: bytes=32 time=12ms TTL=128\r\n" +
				"Request timed out.\r\n" +
				"Reply from 192.0.2.1: bytes=32 time<1ms TTL=128\r\n" +
				"\r\nPing statistics for 192.0.2.1:\r\n" +
				"    Packets: Sent = 3, Received = 2, Lost = 1 (33% loss),\r\n" +
				"Approximate round trip times in milli-seconds:\r\n" +
				"    Minimum = 0ms, Maximum = 12ms, Average = 6ms\r\n",
			want: []int64{12000, 1000, -1, -1, -1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses, err := parsePingResponses([]byte(tc.output), 5)
			require.NoError(t, err)
			assert.Equal(t, tc.want, responses)
		})
	}
}

func TestMonitorICMPExecOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell stub for ping")
	}
	for _, tc := range []struct {
		name   string
		output string
		exit   int
		want   []int64
	}{
		{"success", "time=1.234 ms", 0, []int64{1234, -1, -1, -1, -1}},
		{"missing RTT", "unrecognized output", 0, nil},
		{"partial loss exits non-zero", "time=1.234 ms", 1, []int64{1234, -1, -1, -1, -1}},
		{"failed command", "unrecognized output", 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// Also verify an inherited locale cannot override the C locale.
			script := fmt.Sprintf("#!/bin/sh\n[ \"$LC_ALL\" = C ] || exit 2\nprintf '%%s\\n' '%s'\nexit %d\n", tc.output, tc.exit)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "ping"), []byte(script), 0o755))
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("LC_ALL", "de_DE.UTF-8")
			responses, err := monitorICMPExec(t.Context(), "127.0.0.1", false)
			if tc.want == nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, responses)
		})
	}
}

type icmpTestReply struct {
	data []byte
	peer net.Addr
}

type scriptedICMPConn struct {
	net.PacketConn
	local     net.Addr
	onWrite   func([]byte, net.Addr)
	replies   []icmpTestReply
	reads     int
	deadlines []time.Time
}

func (c *scriptedICMPConn) LocalAddr() net.Addr { return c.local }

func (c *scriptedICMPConn) SetReadDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	return nil
}

func (c *scriptedICMPConn) WriteTo(data []byte, dst net.Addr) (int, error) {
	c.onWrite(data, dst)
	return len(data), nil
}

func (c *scriptedICMPConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	c.reads++
	if len(c.replies) == 0 {
		return 0, nil, os.ErrDeadlineExceeded
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return copy(buf, reply.data), reply.peer, nil
}

func TestMonitorICMPReplyCorrelation(t *testing.T) {
	setICMPBurst(t, 1, time.Second, 3*time.Second)
	for _, family := range []*icmpFamily{&icmpV4, &icmpV6} {
		for _, datagram := range []bool{false, true} {
			network := family.rawNetwork
			ip, other := net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2")
			if family.isIPv6 {
				ip, other = net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")
			}
			var dst net.Addr = &net.IPAddr{IP: ip}
			var wrongPeer net.Addr = &net.IPAddr{IP: other}
			if datagram {
				network = family.dgramNetwork
				dst = &net.UDPAddr{IP: ip}
				wrongPeer = &net.UDPAddr{IP: other}
			}
			for _, mismatch := range []string{"source", "id", "sequence", "payload", "type", "code", "malformed"} {
				for _, eventuallyMatches := range []bool{false, true} {
					ending := "timeout"
					if eventuallyMatches {
						ending = "success"
					}
					t.Run(network+"/"+mismatch+"/"+ending, func(t *testing.T) {
						conn := &scriptedICMPConn{local: &net.IPAddr{IP: net.IPv4zero}}
						if datagram {
							conn.local = &net.UDPAddr{Port: 12345}
							if runtime.GOOS == "linux" {
								// Deliberately differ from the process ID.
								conn.local = &net.UDPAddr{Port: (os.Getpid() % 65534) + 1}
							}
						}
						conn.onWrite = func(data []byte, target net.Addr) {
							require.Equal(t, dst, target)
							request, err := icmp.ParseMessage(family.proto, data)
							require.NoError(t, err)
							echo := request.Body.(*icmp.Echo)
							expectedID := os.Getpid() & 0xffff
							if datagram && runtime.GOOS == "linux" {
								expectedID = conn.local.(*net.UDPAddr).Port
							}
							require.Equal(t, expectedID, echo.ID)
							reply := &icmp.Message{Type: family.replyType, Body: echo}
							valid, err := reply.Marshal(nil)
							require.NoError(t, err)
							peer := dst
							switch mismatch {
							case "source":
								peer = wrongPeer
							case "id":
								echo.ID ^= 1
							case "sequence":
								echo.Seq ^= 1
							case "payload":
								echo.Data[0] ^= 1
							case "type":
								reply.Type = family.echoType
							case "code":
								reply.Code = 1
							}
							invalid, err := reply.Marshal(nil)
							require.NoError(t, err)
							if mismatch == "malformed" {
								invalid = invalid[:2]
							}
							conn.replies = []icmpTestReply{{invalid, peer}}
							if eventuallyMatches {
								conn.replies = append(conn.replies, icmpTestReply{valid, dst})
							}
						}
						responses, err := monitorICMPPacket(context.Background(), conn, family, dst)
						require.Len(t, responses, 1)
						if eventuallyMatches {
							require.NoError(t, err)
							assert.GreaterOrEqual(t, responses[0], int64(0))
						} else {
							require.ErrorIs(t, err, os.ErrDeadlineExceeded)
							assert.Equal(t, int64(-1), responses[0])
						}
						assert.Equal(t, 2, conn.reads)
						// Ignored replies must not extend the reply deadline.
						require.Len(t, conn.deadlines, 2)
						assert.Equal(t, conn.deadlines[0], conn.deadlines[1])
					})
				}
			}
		}
	}
}

func TestMonitorICMPBurst(t *testing.T) {
	setICMPBurst(t, 5, 0, time.Second)
	dst := &net.IPAddr{IP: net.ParseIP("192.0.2.1")}
	conn := &scriptedICMPConn{local: &net.IPAddr{IP: net.IPv4zero}}
	var echoes []*icmp.Echo
	conn.onWrite = func(data []byte, _ net.Addr) {
		request, err := icmp.ParseMessage(icmpV4.proto, data)
		require.NoError(t, err)
		echoes = append(echoes, request.Body.(*icmp.Echo))
		reply := func(echo *icmp.Echo) icmpTestReply {
			data, err := (&icmp.Message{Type: icmpV4.replyType, Body: echo}).Marshal(nil)
			require.NoError(t, err)
			return icmpTestReply{data, dst}
		}
		switch len(echoes) {
		case 2, 3:
			// The second request is answered after the fourth; the third is lost.
		case 4:
			conn.replies = append(conn.replies, reply(echoes[3]), reply(echoes[1]), reply(echoes[1]))
		default:
			conn.replies = append(conn.replies, reply(echoes[len(echoes)-1]))
		}
	}

	responses, err := monitorICMPPacket(context.Background(), conn, &icmpV4, dst)
	require.NoError(t, err)
	require.Len(t, echoes, 5)
	for i, echo := range echoes[1:] {
		assert.NotEqual(t, echoes[i].Seq, echo.Seq, "each request needs its own sequence")
	}
	require.Len(t, responses, 5)
	for i, responseUs := range responses {
		if i == 2 {
			assert.Equal(t, int64(-1), responseUs)
		} else {
			assert.GreaterOrEqual(t, responseUs, int64(0))
		}
	}
}

func TestMonitorICMPLoopback(t *testing.T) {
	setICMPBurst(t, 3, 10*time.Millisecond, 3*time.Second)
	for _, family := range []*icmpFamily{&icmpV4, &icmpV6} {
		for _, network := range []string{family.rawNetwork, family.dgramNetwork} {
			t.Run(network, func(t *testing.T) {
				conn, err := icmp.ListenPacket(network, family.listenAddr)
				if err != nil {
					t.Skipf("ICMP socket unavailable: %v", err)
				}
				defer conn.Close()
				ip := net.ParseIP("127.0.0.1")
				if family.isIPv6 {
					ip = net.ParseIP("::1")
				}
				var dst net.Addr = &net.IPAddr{IP: ip}
				if network == family.dgramNetwork {
					dst = &net.UDPAddr{IP: ip}
				}
				responses, err := monitorICMPPacket(context.Background(), conn, family, dst)
				require.NoError(t, err)
				require.Len(t, responses, 3)
				for _, responseUs := range responses {
					assert.GreaterOrEqual(t, responseUs, int64(0))
				}
			})
		}
	}
}

func TestDetectICMPMode(t *testing.T) {
	tests := []struct {
		name         string
		family       *icmpFamily
		rawErr       error
		udpErr       error
		want         icmpMethod
		wantNetworks []string
	}{
		{
			name:         "IPv4 prefers raw socket when available",
			family:       &icmpV4,
			want:         icmpRaw,
			wantNetworks: []string{"ip4:icmp"},
		},
		{
			name:         "IPv4 uses datagram when raw unavailable",
			family:       &icmpV4,
			rawErr:       errors.New("operation not permitted"),
			want:         icmpDatagram,
			wantNetworks: []string{"ip4:icmp", "udp4"},
		},
		{
			name:         "IPv4 falls back to exec when both unavailable",
			family:       &icmpV4,
			rawErr:       errors.New("operation not permitted"),
			udpErr:       errors.New("protocol not supported"),
			want:         icmpExecFallback,
			wantNetworks: []string{"ip4:icmp", "udp4"},
		},
		{
			name:         "IPv6 prefers raw socket when available",
			family:       &icmpV6,
			want:         icmpRaw,
			wantNetworks: []string{"ip6:ipv6-icmp"},
		},
		{
			name:         "IPv6 uses datagram when raw unavailable",
			family:       &icmpV6,
			rawErr:       errors.New("operation not permitted"),
			want:         icmpDatagram,
			wantNetworks: []string{"ip6:ipv6-icmp", "udp6"},
		},
		{
			name:         "IPv6 falls back to exec when both unavailable",
			family:       &icmpV6,
			rawErr:       errors.New("operation not permitted"),
			udpErr:       errors.New("protocol not supported"),
			want:         icmpExecFallback,
			wantNetworks: []string{"ip6:ipv6-icmp", "udp6"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]string, 0, 2)
			listen := func(network, listenAddr string) (icmpPacketConn, error) {
				require.Equal(t, tt.family.listenAddr, listenAddr)
				calls = append(calls, network)
				switch network {
				case tt.family.rawNetwork:
					if tt.rawErr != nil {
						return nil, tt.rawErr
					}
				case tt.family.dgramNetwork:
					if tt.udpErr != nil {
						return nil, tt.udpErr
					}
				default:
					t.Fatalf("unexpected network %q", network)
				}
				return testICMPPacketConn{}, nil
			}

			assert.Equal(t, tt.want, detectICMPMode(tt.family, listen))
			assert.Equal(t, tt.wantNetworks, calls)
		})
	}
}

func TestResolveICMPTarget(t *testing.T) {
	t.Run("IPv4 literal", func(t *testing.T) {
		family, ip, err := resolveICMPTarget(context.Background(), "127.0.0.1")
		require.NoError(t, err)
		require.NotNil(t, family)
		assert.False(t, family.isIPv6)
		assert.Equal(t, "127.0.0.1", ip.String())
	})

	t.Run("IPv6 literal", func(t *testing.T) {
		family, ip, err := resolveICMPTarget(context.Background(), "::1")
		require.NoError(t, err)
		require.NotNil(t, family)
		assert.True(t, family.isIPv6)
		assert.Equal(t, "::1", ip.String())
	})

	t.Run("IPv4-mapped IPv6 resolves as IPv4", func(t *testing.T) {
		family, ip, err := resolveICMPTarget(context.Background(), "::ffff:127.0.0.1")
		require.NoError(t, err)
		require.NotNil(t, family)
		assert.False(t, family.isIPv6)
		assert.Equal(t, "127.0.0.1", ip.String())
	})
}
