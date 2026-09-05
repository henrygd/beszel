//go:build testing

package agent

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVolumeManagerForTest(server *httptest.Server) *volumeManager {
	return &volumeManager{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
					return net.Dial(network, server.Listener.Addr().String())
				},
			},
		},
	}
}

func TestVolumeManagerRefresh(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		expected map[string]float64
	}{
		{
			name:   "parses volume sizes",
			status: http.StatusOK,
			body: `{"Volumes":[
				{"Name":"immich_data","UsageData":{"Size":2147483648}},
				{"Name":"pg_data","UsageData":{"Size":1073741824}}
			]}`,
			expected: map[string]float64{"immich_data": 2, "pg_data": 1},
		},
		{
			name:   "skips volumes without computed usage",
			status: http.StatusOK,
			body: `{"Volumes":[
				{"Name":"counted","UsageData":{"Size":1073741824}},
				{"Name":"uncounted","UsageData":{"Size":-1}},
				{"Name":"empty","UsageData":{"Size":0}},
				{"Name":"","UsageData":{"Size":1073741824}}
			]}`,
			expected: map[string]float64{"counted": 1},
		},
		{
			name:   "ignores fields returned by engines without the type filter",
			status: http.StatusOK,
			body: `{
				"LayersSize":123,
				"Images":[{"Id":"sha256:abc","Size":456}],
				"Containers":[{"Id":"def","SizeRw":789}],
				"BuildCache":[{"ID":"ghi","Size":10}],
				"Volumes":[{"Name":"vol","UsageData":{"Size":536870912}}]
			}`,
			expected: map[string]float64{"vol": 0.5},
		},
		{
			name:     "no volumes yields no snapshot",
			status:   http.StatusOK,
			body:     `{"Volumes":[]}`,
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/system/df", r.URL.Path)
				assert.Equal(t, "volume", r.URL.Query().Get("type"))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			vm := newVolumeManagerForTest(server)
			vm.refresh()

			assert.Equal(t, tc.expected, vm.snapshot())
		})
	}
}

func TestVolumeManagerRefreshKeepsPreviousSnapshotOnError(t *testing.T) {
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"Volumes":[{"Name":"vol","UsageData":{"Size":1073741824}}]}`))
	}))
	defer server.Close()

	vm := newVolumeManagerForTest(server)
	vm.refresh()
	require.Equal(t, map[string]float64{"vol": 1}, vm.snapshot())

	fail = true
	vm.refresh()
	assert.Equal(t, map[string]float64{"vol": 1}, vm.snapshot())
}

func TestVolumeManagerRefreshKeepsPreviousSnapshotOnMalformedBody(t *testing.T) {
	body := `{"Volumes":[{"Name":"vol","UsageData":{"Size":1073741824}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	vm := newVolumeManagerForTest(server)
	vm.refresh()
	require.Equal(t, map[string]float64{"vol": 1}, vm.snapshot())

	body = `{"Volumes":`
	vm.refresh()
	assert.Equal(t, map[string]float64{"vol": 1}, vm.snapshot())
}

func TestVolumeManagerSnapshotIsACopy(t *testing.T) {
	vm := &volumeManager{sizes: map[string]float64{"vol": 1}}

	snapshot := vm.snapshot()
	snapshot["vol"] = 99
	snapshot["other"] = 5

	assert.Equal(t, map[string]float64{"vol": 1}, vm.snapshot())
}

func TestNewVolumeManager(t *testing.T) {
	tests := []struct {
		name             string
		envValue         string
		envSet           bool
		expectNil        bool
		expectedInterval time.Duration
	}{
		{name: "disabled when unset", expectNil: true},
		{name: "disabled when empty", envValue: "", envSet: true, expectNil: true},
		{name: "disabled when unparseable", envValue: "soon", envSet: true, expectNil: true},
		{name: "disabled when zero", envValue: "0s", envSet: true, expectNil: true},
		{name: "disabled when negative", envValue: "-5m", envSet: true, expectNil: true},
		{name: "clamped to minimum", envValue: "1s", envSet: true, expectedInterval: minVolumeInterval},
		{name: "uses configured interval", envValue: "30m", envSet: true, expectedInterval: 30 * time.Minute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				t.Setenv("DOCKER_VOLUME_INTERVAL", tc.envValue)
			}

			vm := newVolumeManager(http.DefaultTransport)

			if tc.expectNil {
				assert.Nil(t, vm)
				return
			}
			require.NotNil(t, vm)
			assert.Equal(t, tc.expectedInterval, vm.interval)
			assert.Equal(t, volumeTimeout, vm.client.Timeout)
		})
	}
}

func TestDockerManagerVolumeSizes(t *testing.T) {
	var nilManager *dockerManager
	assert.Nil(t, nilManager.volumeSizes())

	assert.Nil(t, (&dockerManager{}).volumeSizes())

	dm := &dockerManager{volumeManager: &volumeManager{sizes: map[string]float64{"vol": 1.5}}}
	assert.Equal(t, map[string]float64{"vol": 1.5}, dm.volumeSizes())
}
