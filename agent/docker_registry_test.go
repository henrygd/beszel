//go:build testing

package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type registryTransportFunc func(*http.Request) (*http.Response, error)

func (fn registryTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func registryResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func registryDigest(fill byte) string {
	return "sha256:" + strings.Repeat(string(fill), 64)
}

func newRegistryChecker(t *testing.T, inspectBody string, transport http.RoundTripper) *dockerManager {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/images/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, inspectBody)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	return &dockerManager{
		client:         newDockerManagerForVersionTest(server).client,
		registryClient: &http.Client{Transport: transport},
	}
}

func TestCheckImageUpdateUsesInspectAndManifestDigests(t *testing.T) {
	local := registryDigest('a')
	remote := registryDigest('b')
	var authCalls, manifestCalls atomic.Int32
	dm := newRegistryChecker(t, fmt.Sprintf(`{"RepoDigests":["docker.io/library/alpine@%s"]}`, local), registryTransportFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Host == "auth.docker.io":
			authCalls.Add(1)
			require.Equal(t, "/token", req.URL.Path)
			return registryResponse(http.StatusOK, `{"token":"test-token"}`), nil
		case req.Method == http.MethodHead && req.URL.Host == "registry-1.docker.io":
			manifestCalls.Add(1)
			require.Equal(t, "/v2/library/alpine/manifests/latest", req.URL.Path)
			require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
			resp := registryResponse(http.StatusOK, "")
			resp.Header.Set("Docker-Content-Digest", remote)
			return resp, nil
		default:
			return registryResponse(http.StatusNotFound, ""), nil
		}
	}))

	available, err := dm.checkImageUpdate("alpine")
	require.NoError(t, err)
	require.True(t, available)
	require.EqualValues(t, 1, authCalls.Load())
	require.EqualValues(t, 1, manifestCalls.Load())
}

func TestCheckImageUpdateReportsUnknownInspectState(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "missing field", body: `{}`},
		{name: "empty field", body: `{"RepoDigests":[]}`},
		{name: "malformed reference", body: `{"RepoDigests":["not-a-repo-digest"]}`},
		{name: "wrong repository", body: `{"RepoDigests":["docker.io/library/busybox@` + registryDigest('a') + `"]}`},
		{name: "malformed digest", body: `{"RepoDigests":["docker.io/library/alpine@sha256:not-a-digest"]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var registryCalls atomic.Int32
			dm := newRegistryChecker(t, test.body, registryTransportFunc(func(req *http.Request) (*http.Response, error) {
				registryCalls.Add(1)
				return registryResponse(http.StatusOK, `{"token":"unexpected"}`), nil
			}))

			available, err := dm.checkImageUpdate("alpine")
			require.Error(t, err)
			require.False(t, available)
			require.EqualValues(t, 0, registryCalls.Load(), "invalid local state must not query a registry")
		})
	}
}

func TestCheckImageUpdateChecksInspectAuthAndManifestStatuses(t *testing.T) {
	local := registryDigest('a')
	validInspect := fmt.Sprintf(`{"RepoDigests":["docker.io/library/alpine@%s"]}`, local)

	tests := []struct {
		name         string
		inspectCode  int
		authCode     int
		manifestCode int
		remote       string
		want         string
	}{
		{name: "inspect status", inspectCode: http.StatusNotFound, want: "inspect image"},
		{name: "auth status", inspectCode: http.StatusOK, authCode: http.StatusUnauthorized, want: "registry auth"},
		{name: "manifest status", inspectCode: http.StatusOK, authCode: http.StatusOK, manifestCode: http.StatusNotFound, remote: local, want: "manifest request"},
		{name: "missing digest", inspectCode: http.StatusOK, authCode: http.StatusOK, manifestCode: http.StatusOK, want: "invalid digest"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.inspectCode != http.StatusOK && strings.HasPrefix(r.URL.Path, "/images/") {
					w.WriteHeader(test.inspectCode)
					return
				}
				_, _ = io.WriteString(w, validInspect)
			}))
			t.Cleanup(server.Close)

			calls := 0
			dm := &dockerManager{client: newDockerManagerForVersionTest(server).client, registryClient: &http.Client{Transport: registryTransportFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method == http.MethodGet {
					return registryResponse(test.authCode, `{"token":"test"}`), nil
				}
				response := registryResponse(test.manifestCode, "")
				response.Header.Set("Docker-Content-Digest", test.remote)
				return response, nil
			})}}

			_, err := dm.checkImageUpdate("alpine")
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
			if test.inspectCode != http.StatusOK {
				require.Zero(t, calls)
			}
		})
	}
}

func TestCheckImageUpdateSupportsAnonymousAndLSCRRegistries(t *testing.T) {
	t.Run("anonymous registry", func(t *testing.T) {
		local := registryDigest('a')
		var calls atomic.Int32
		dm := newRegistryChecker(t, fmt.Sprintf(`{"RepoDigests":["example.com/app@%s"]}`, local), registryTransportFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			require.Equal(t, http.MethodHead, req.Method)
			require.Equal(t, "example.com", req.URL.Host)
			resp := registryResponse(http.StatusOK, "")
			resp.Header.Set("Docker-Content-Digest", local)
			return resp, nil
		}))
		available, err := dm.checkImageUpdate("example.com/app")
		require.NoError(t, err)
		require.False(t, available)
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("lscr ghcr alias", func(t *testing.T) {
		local := registryDigest('a')
		var authCalls, manifestCalls atomic.Int32
		dm := newRegistryChecker(t, fmt.Sprintf(`{"RepoDigests":["ghcr.io/linuxserver/app@%s"]}`, local), registryTransportFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet {
				authCalls.Add(1)
				return registryResponse(http.StatusOK, `{"token":"test"}`), nil
			}
			manifestCalls.Add(1)
			require.Equal(t, "lscr.io", req.URL.Host)
			resp := registryResponse(http.StatusOK, "")
			resp.Header.Set("Docker-Content-Digest", local)
			return resp, nil
		}))
		available, err := dm.checkImageUpdate("lscr.io/linuxserver/app")
		require.NoError(t, err)
		require.False(t, available)
		require.EqualValues(t, 1, authCalls.Load())
		require.EqualValues(t, 1, manifestCalls.Load())
	})
}

func TestCheckImageUpdateSkipsPinnedDigest(t *testing.T) {
	image := "docker.io/library/alpine@" + registryDigest('a')
	dm := &dockerManager{}
	available, err := dm.checkImageUpdate(image)
	require.NoError(t, err)
	require.False(t, available)
}
