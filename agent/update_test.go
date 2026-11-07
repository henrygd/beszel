//go:build testing

package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unsetHubURL(t *testing.T) {
	t.Helper()
	t.Setenv("BESZEL_AGENT_HUB_URL", "")
	os.Unsetenv("BESZEL_AGENT_HUB_URL")
	t.Setenv("HUB_URL", "")
	os.Unsetenv("HUB_URL")
}

func TestFetchHubVersion(t *testing.T) {
	t.Run("hub returns version", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/beszel/version", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"v":"0.19.0"}`))
		}))
		defer server.Close()

		t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
		version, err := fetchHubVersion()
		require.NoError(t, err)
		assert.Equal(t, "0.19.0", version.String())
	})

	t.Run("hub url with base path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/beszel/api/beszel/version", r.URL.Path)
			w.Write([]byte(`{"v":"0.19.0"}`))
		}))
		defer server.Close()

		t.Setenv("BESZEL_AGENT_HUB_URL", server.URL+"/beszel")
		version, err := fetchHubVersion()
		require.NoError(t, err)
		assert.Equal(t, "0.19.0", version.String())
	})

	t.Run("hub without version endpoint errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()

		t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
		_, err := fetchHubVersion()
		require.Error(t, err)
	})

	t.Run("invalid version errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"v":"not-a-version"}`))
		}))
		defer server.Close()

		t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
		_, err := fetchHubVersion()
		require.Error(t, err)
	})

	t.Run("hub url not set errors", func(t *testing.T) {
		unsetHubURL(t)
		_, err := fetchHubVersion()
		require.Error(t, err)
	})

	t.Run("invalid hub url errors", func(t *testing.T) {
		t.Setenv("BESZEL_AGENT_HUB_URL", "://invalid")
		_, err := fetchHubVersion()
		require.Error(t, err)
	})
}