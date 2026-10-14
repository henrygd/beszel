//go:build testing

package hub

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAppRoute(t *testing.T) {
	tests := []struct {
		path     string
		basePath string
		want     bool
	}{
		// known routes
		{"/", "/", true},
		{"/containers", "/", true},
		{"/containers/", "/", true},
		{"/Containers", "/", true},
		{"/smart", "/", true},
		{"/monitors", "/", true},
		{"/forgot-password", "/", true},
		{"/request-otp", "/", true},
		{"/system/abc123", "/", true},
		{"/system/abc123/", "/", true},
		{"/settings", "/", true},
		{"/settings/general", "/", true},

		// unknown paths
		{"/.env", "/", false},
		{"/phpinfo.php", "/", false},
		{"/wp-admin/", "/", false},
		{"/.git/config", "/", false},
		{"/system", "/", false},
		{"/system/", "/", false},
		{"/system/abc/def", "/", false},
		{"/settings/general/extra", "/", false},
		{"/containersx", "/", false},

		// base path, prefix not stripped by proxy
		{"/beszel", "/beszel/", true},
		{"/beszel/", "/beszel/", true},
		{"/beszel/containers", "/beszel/", true},
		{"/beszel/system/abc123", "/beszel/", true},
		{"/beszel/.env", "/beszel/", false},
		{"/beszelx", "/beszel/", false},

		// base path, prefix stripped by proxy
		{"/", "/beszel/", true},
		{"/containers", "/beszel/", true},
		{"/.env", "/beszel/", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, isAppRoute(tt.path, tt.basePath), "path=%q basePath=%q", tt.path, tt.basePath)
	}
}

// TestApplyCSPHeaders pins the interaction between a custom CSP and the
// default clickjacking protection: X-Frame-Options may only be dropped when
// the CSP itself covers framing via frame-ancestors. Regression target: the
// handler used to delete X-Frame-Options for ANY custom CSP value.
func TestApplyCSPHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Frame-Options", "DENY")
	applyCSPHeaders(headers, "default-src 'self'; frame-ancestors 'self'")
	assert.Empty(t, headers.Get("X-Frame-Options"),
		"a CSP with frame-ancestors replaces X-Frame-Options")
	assert.Equal(t, "default-src 'self'; frame-ancestors 'self'", headers.Get("Content-Security-Policy"))

	headers = http.Header{}
	headers.Set("X-Frame-Options", "DENY")
	applyCSPHeaders(headers, "default-src 'self'")
	assert.Equal(t, "DENY", headers.Get("X-Frame-Options"),
		"an unrelated CSP value must not strip clickjacking protection")
	assert.Equal(t, "default-src 'self'", headers.Get("Content-Security-Policy"))
}
