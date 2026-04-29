//go:build testing

package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testICMPPacketConn struct{}

func (testICMPPacketConn) Close() error { return nil }

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
