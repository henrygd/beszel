package agent

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPVEManagerDoesNotConnectAtStartup(t *testing.T) {
	t.Setenv("BESZEL_AGENT_PROXMOX_URL", "https://127.0.0.1:1/api2/json")
	t.Setenv("BESZEL_AGENT_PROXMOX_NODE", "pve")
	t.Setenv("BESZEL_AGENT_PROXMOX_TOKENID", "root@pam!test")
	t.Setenv("BESZEL_AGENT_PROXMOX_SECRET", "secret")

	pm := newPVEManager()
	require.NotNil(t, pm)
	assert.Zero(t, pm.cpuCount)
}

func TestPVEManagerRetriesInitialization(t *testing.T) {
	var nodeRequests atomic.Int32
	var clusterRequests atomic.Int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve/status":
			nodeRequests.Add(1)
			fmt.Fprint(w, `{"data":{"cpuinfo":{"cpus":8}}}`)
		case "/api2/json/cluster/status":
			fmt.Fprint(w, `{"data":[{"type":"cluster","name":"test-cluster","id":"test-cluster","version":1,"quorate":1}]}`)
		case "/api2/json/cluster/resources":
			clusterRequests.Add(1)
			fmt.Fprint(w, `{"data":[{"id":"qemu/101","type":"qemu","node":"pve","status":"running","name":"vm-101","cpu":0.5,"maxcpu":4,"maxmem":4096,"mem":2048,"netin":1024,"netout":2048,"diskread":10,"diskwrite":20,"maxdisk":8192,"uptime":60}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	pm := &pveManager{
		client: proxmox.NewClient(server.URL+"/api2/json",
			proxmox.WithHTTPClient(&http.Client{
				Transport: &failOnceRoundTripper{
					base: server.Client().Transport,
				},
			}),
			proxmox.WithAPIToken("root@pam!test", "secret"),
		),
		nodeName:     "pve",
		nodeStatsMap: make(map[string]*container.PveNodeStats),
	}

	stats, err := pm.getPVEStats()
	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Zero(t, pm.cpuCount)

	pm.lastInitTry = time.Now().Add(-31 * time.Second)
	stats, err = pm.getPVEStats()
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, int32(1), nodeRequests.Load())
	assert.Equal(t, int32(1), clusterRequests.Load())
	assert.Equal(t, 8, pm.cpuCount)
	assert.Equal(t, "qemu/101", stats[0].Id)
	assert.Equal(t, 25.0, stats[0].Cpu)
	assert.Equal(t, uint64(1024), stats[0].NetIn)
	assert.Equal(t, uint64(2048), stats[0].NetOut)
}

type failOnceRoundTripper struct {
	base   http.RoundTripper
	failed atomic.Bool
}

func (rt *failOnceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api2/json/nodes/pve/status" && !rt.failed.Swap(true) {
		return nil, errors.New("dial tcp 127.0.0.1:8006: connect: connection refused")
	}
	return rt.base.RoundTrip(req)
}

var _ http.RoundTripper = (*failOnceRoundTripper)(nil)
