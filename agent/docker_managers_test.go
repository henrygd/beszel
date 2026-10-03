//go:build testing

package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDockerHosts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty disables", "", nil},
		{"only separators", " , ,", nil},
		{"single", "unix:///var/run/docker.sock", []string{"unix:///var/run/docker.sock"}},
		{
			"multiple with whitespace",
			" unix:///var/run/docker.sock , unix:///run/user/1000/docker.sock ",
			[]string{"unix:///var/run/docker.sock", "unix:///run/user/1000/docker.sock"},
		},
		{"duplicates removed", "tcp://a:2375,tcp://b:2375,tcp://a:2375", []string{"tcp://a:2375", "tcp://b:2375"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseDockerHosts(tt.raw))
		})
	}
}

// fakeEngine serves a minimal Docker API with one running container per id.
func fakeEngine(t *testing.T, name string, ids ...string) *httptest.Server {
	t.Helper()
	known := make(map[string]bool, len(ids))
	var list []string
	for _, id := range ids {
		known[id] = true
		list = append(list, fmt.Sprintf(`{"Id":"%s","Names":["/c-%s"],"Status":"Up 5 minutes","Image":"img"}`, id+strings.Repeat("0", 64-len(id)), id))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.URL.Path == "/version":
			fmt.Fprint(w, `{"Version":"25.0.0"}`)
		case r.URL.Path == "/containers/json":
			fmt.Fprintf(w, "[%s]", strings.Join(list, ","))
		case len(parts) == 3 && parts[0] == "containers" && known[parts[1]]:
			switch parts[2] {
			case "stats":
				fmt.Fprint(w, `{"read":"2026-03-15T21:26:59Z","cpu_stats":{"cpu_usage":{"total_usage":1000},"system_cpu_usage":2000},"memory_stats":{"usage":1048576}}`)
			case "json":
				fmt.Fprintf(w, `{"Id":"%s","Config":{"Env":["SECRET=1"]}}`, parts[1])
			case "logs":
				fmt.Fprintf(w, "log from %s", name)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestDockerManagers(servers ...*httptest.Server) *dockerManagers {
	dms := &dockerManagers{owners: map[string]*dockerManager{}}
	for _, s := range servers {
		dm := newDockerManagerForVersionTest(s)
		dm.sem = make(chan struct{}, 5)
		dms.managers = append(dms.managers, dm)
	}
	return dms
}

func TestDockerManagersMergesEngines(t *testing.T) {
	rootful := fakeEngine(t, "rootful", "aaaaaaaaaaaa")
	rootless := fakeEngine(t, "rootless", "bbbbbbbbbbbb", "cccccccccccc")
	dms := newTestDockerManagers(rootful, rootless)

	stats, err := dms.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)

	var ids []string
	for _, s := range stats {
		ids = append(ids, s.Id)
	}
	assert.ElementsMatch(t, []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb", "cccccccccccc"}, ids)

	// container requests are routed to the engine that owns the container
	logs, err := dms.getLogs(context.Background(), "bbbbbbbbbbbb")
	require.NoError(t, err)
	assert.Equal(t, "log from rootless", logs)

	logs, err = dms.getLogs(context.Background(), "aaaaaaaaaaaa")
	require.NoError(t, err)
	assert.Equal(t, "log from rootful", logs)

	info, err := dms.getContainerInfo(context.Background(), "cccccccccccc")
	require.NoError(t, err)
	assert.Contains(t, string(info), "cccccccccccc")
	assert.NotContains(t, string(info), "SECRET", "Env must still be stripped")
}

func TestDockerManagersUnknownContainerFallsBackToAllEngines(t *testing.T) {
	rootful := fakeEngine(t, "rootful", "aaaaaaaaaaaa")
	rootless := fakeEngine(t, "rootless", "bbbbbbbbbbbb")
	dms := newTestDockerManagers(rootful, rootless)

	// no stats call yet, so no owner is known; the second engine has the container
	logs, err := dms.getLogs(context.Background(), "bbbbbbbbbbbb")
	require.NoError(t, err)
	assert.Equal(t, "log from rootless", logs)

	_, err = dms.getLogs(context.Background(), "dddddddddddd")
	require.Error(t, err)
}

func TestDockerManagersToleratesOneFailingEngine(t *testing.T) {
	rootful := fakeEngine(t, "rootful", "aaaaaaaaaaaa")
	rootless := fakeEngine(t, "rootless", "bbbbbbbbbbbb")
	dms := newTestDockerManagers(rootful, rootless)

	rootless.Close()
	stats, err := dms.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "aaaaaaaaaaaa", stats[0].Id)

	rootful.Close()
	_, err = dms.getDockerStats(defaultCacheTimeMs)
	require.Error(t, err)
}

func TestDockerManagersSingleEngineUnchanged(t *testing.T) {
	engine := fakeEngine(t, "rootful", "aaaaaaaaaaaa")
	dms := newTestDockerManagers(engine)

	stats, err := dms.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "aaaaaaaaaaaa", stats[0].Id)
}

func TestNewDockerManagersEnv(t *testing.T) {
	t.Run("empty disables", func(t *testing.T) {
		t.Setenv("BESZEL_AGENT_DOCKER_HOST", "")
		assert.Nil(t, newDockerManagers(nil))
	})
	t.Run("multiple endpoints", func(t *testing.T) {
		a := fakeEngine(t, "a")
		b := fakeEngine(t, "b")
		t.Setenv("BESZEL_AGENT_DOCKER_HOST", a.URL+","+b.URL)
		dms := newDockerManagers(nil)
		require.NotNil(t, dms)
		assert.Len(t, dms.managers, 2)
	})
}
