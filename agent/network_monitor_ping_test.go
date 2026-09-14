//go:build testing

package agent

import (
	"errors"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/icmp"
)

type testICMPPacketConn struct{}

func (testICMPPacketConn) Close() error { return nil }

type icmpTestReply struct {
	data []byte
	peer net.Addr
}

type scriptedICMPConn struct {
	net.PacketConn
	local        net.Addr
	onWrite      func([]byte, net.Addr)
	replies      []icmpTestReply
	reads        int
	deadlineSets int
}

func (c *scriptedICMPConn) LocalAddr() net.Addr { return c.local }

func (c *scriptedICMPConn) SetDeadline(deadline time.Time) error {
	c.deadlineSets++
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
						elapsed, err := monitorICMPPacket(conn, family, dst)
						if eventuallyMatches {
							require.NoError(t, err)
							assert.GreaterOrEqual(t, elapsed, int64(0))
						} else {
							require.ErrorIs(t, err, os.ErrDeadlineExceeded)
							assert.Equal(t, int64(-1), elapsed)
						}
						assert.Equal(t, 2, conn.reads)
						assert.Equal(t, 1, conn.deadlineSets)
					})
				}
			}
		}
	}
}

func TestMonitorICMPLoopback(t *testing.T) {
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
				elapsed, err := monitorICMPPacket(conn, family, dst)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, elapsed, int64(0))
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
		family, ip, err := resolveICMPTarget("127.0.0.1")
		require.NoError(t, err)
		require.NotNil(t, family)
		assert.False(t, family.isIPv6)
		assert.Equal(t, "127.0.0.1", ip.String())
	})

	t.Run("IPv6 literal", func(t *testing.T) {
		family, ip, err := resolveICMPTarget("::1")
		require.NoError(t, err)
		require.NotNil(t, family)
		assert.True(t, family.isIPv6)
		assert.Equal(t, "::1", ip.String())
	})

	t.Run("IPv4-mapped IPv6 resolves as IPv4", func(t *testing.T) {
		family, ip, err := resolveICMPTarget("::ffff:127.0.0.1")
		require.NoError(t, err)
		require.NotNil(t, family)
		assert.False(t, family.isIPv6)
		assert.Equal(t, "127.0.0.1", ip.String())
	})
}
