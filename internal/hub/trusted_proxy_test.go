//go:build testing

package hub

import (
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTrustedProxies(t *testing.T) {
	testCases := []struct {
		name       string
		value      string
		prefixes   []string
		restricted bool
	}{
		{
			name:       "empty",
			value:      "",
			restricted: false,
		},
		{
			name:       "blank",
			value:      "  , ",
			prefixes:   nil,
			restricted: true,
		},
		{
			name:       "single addresses become host prefixes",
			value:      "10.0.0.5, 2001:db8::1",
			prefixes:   []string{"10.0.0.5/32", "2001:db8::1/128"},
			restricted: true,
		},
		{
			name:       "cidrs are masked",
			value:      "172.16.5.9/12,fd00::1/64",
			prefixes:   []string{"172.16.0.0/12", "fd00::/64"},
			restricted: true,
		},
		{
			name:       "ipv4-mapped entries become ipv4",
			value:      "::ffff:10.0.0.5, ::ffff:10.0.0.0/104",
			prefixes:   []string{"10.0.0.5/32", "10.0.0.0/8"},
			restricted: true,
		},
		{
			name:       "invalid entries are skipped, valid ones kept",
			value:      "proxy.internal, 10.0.0.0/8, 300.1.1.1, ::ffff:0.0.0.0/64",
			prefixes:   []string{"10.0.0.0/8"},
			restricted: true,
		},
		{
			name:       "only invalid entries trust nobody",
			value:      "proxy.internal",
			prefixes:   nil,
			restricted: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXY_IPS", tc.value)
			prefixes, restricted := parseTrustedProxies()
			assert.Equal(t, tc.restricted, restricted)
			var got []string
			for _, p := range prefixes {
				got = append(got, p.String())
			}
			assert.Equal(t, tc.prefixes, got)
		})
	}

	t.Run("unset", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXY_IPS", "")
		os.Unsetenv("TRUSTED_PROXY_IPS")
		prefixes, restricted := parseTrustedProxies()
		assert.False(t, restricted)
		assert.Nil(t, prefixes)
	})

	t.Run("prefixed env var takes precedence", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXY_IPS", "10.0.0.0/8")
		t.Setenv("BESZEL_HUB_TRUSTED_PROXY_IPS", "192.168.0.0/16")
		prefixes, restricted := parseTrustedProxies()
		assert.True(t, restricted)
		require.Len(t, prefixes, 1)
		assert.Equal(t, "192.168.0.0/16", prefixes[0].String())
	})
}

func TestIsTrustedProxy(t *testing.T) {
	prefixes := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("fe80::/10"),
	}

	testCases := []struct {
		name       string
		remoteAddr string
		trusted    bool
	}{
		{"ipv4 in prefix", "10.20.30.40:51234", true},
		{"ipv4 outside prefix", "11.0.0.1:51234", false},
		{"ipv6 in prefix", "[2001:db8:1::2]:443", true},
		{"ipv6 outside prefix", "[2001:db9::1]:443", false},
		{"ipv4-mapped ipv6 matches ipv4 prefix", "[::ffff:10.1.2.3]:80", true},
		{"zone is ignored", "[fe80::1%eth0]:80", true},
		{"no port", "10.1.2.3", true},
		{"empty", "", false},
		{"garbage", "not-an-address:80", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.trusted, isTrustedProxy(prefixes, tc.remoteAddr))
		})
	}

	t.Run("empty allowlist trusts nobody", func(t *testing.T) {
		assert.False(t, isTrustedProxy(nil, "10.0.0.1:1"))
	})
}
