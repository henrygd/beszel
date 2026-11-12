//go:build testing

package systems

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeNilEntries(t *testing.T) {
	data := &system.CombinedData{
		Containers: []*container.Stats{nil, {Id: "abc"}, nil},
		Stats: system.Stats{
			ExtraFs:  map[string]*system.FsStats{"bad": nil, "good": {DiskTotal: 1}},
			ZfsPools: map[string]*system.ZfsPool{"bad": nil},
		},
	}
	sanitizeNilEntries(data)
	assert.Len(t, data.Containers, 1)
	assert.Equal(t, "abc", data.Containers[0].Id)
	assert.Len(t, data.Stats.ExtraFs, 1)
	assert.Contains(t, data.Stats.ExtraFs, "good")
	assert.Empty(t, data.Stats.ZfsPools)

	// only nils: container slice becomes empty, so Containers[0] is never read
	data = &system.CombinedData{Containers: []*container.Stats{nil}}
	sanitizeNilEntries(data)
	assert.Empty(t, data.Containers)

	assert.NotPanics(t, func() { sanitizeNilEntries(nil) })
}

func TestExtractPowerMetricsNilAttribute(t *testing.T) {
	assert.NotPanics(t, func() {
		hours, cycles := extractPowerMetrics([]*smart.SmartAttribute{nil, {Name: "PowerOnHours", RawValue: 5}})
		assert.Equal(t, uint64(5), hours)
		assert.Equal(t, uint64(0), cycles)
	})
}

// fixedDataClient answers every GetData request with the same payload.
type fixedDataClient struct {
	gws.BuiltinEventHandler
	data *system.CombinedData
}

func (c *fixedDataClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	var req common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Bytes(), &req); err != nil || req.Action != common.GetData {
		return
	}
	// nil pointers in the payload encode as CBOR nulls, as a buggy agent would send
	response, _ := cbor.Marshal(common.AgentResponse{Id: req.Id, SystemData: c.data})
	_ = conn.WriteMessage(gws.OpcodeBinary, response)
}

// A full poll with null containers and filesystems used to panic in
// createRecords (#2831). The nulls must be gone before sys.data is published.
func TestUpdateDropsNullEntriesFromAgent(t *testing.T) {
	sys, app := newTestSystemWithHub(t)

	agentData := &system.CombinedData{
		Containers: []*container.Stats{nil, {Id: "abc", Name: "web"}},
		Stats: system.Stats{
			ExtraFs: map[string]*system.FsStats{"bad": nil, "sdb": {DiskTotal: 100, DiskUsed: 40}},
		},
	}

	connections := make(chan *ws.WsConn, 1)
	upgrader := gws.NewUpgrader(&monitorSyncServer{}, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Error(err)
			return
		}
		wsConn := ws.NewWsConnection(conn, semver.MustParse("0.20.0"))
		conn.Session().Store("wsConn", wsConn)
		connections <- wsConn
		conn.ReadLoop()
	}))
	t.Cleanup(server.Close)

	conn, _, err := gws.NewClient(&fixedDataClient{data: agentData}, &gws.ClientOption{Addr: "ws" + strings.TrimPrefix(server.URL, "http")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.NetConn().Close() })
	go conn.ReadLoop()

	select {
	case sys.WsConn = <-connections:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket connection was not established")
	}

	require.NotPanics(t, func() { require.NoError(t, sys.update()) })

	require.Len(t, sys.data.Containers, 1)
	assert.Equal(t, "abc", sys.data.Containers[0].Id)
	assert.NotContains(t, sys.data.Stats.ExtraFs, "bad")
	assert.Contains(t, sys.data.Stats.ExtraFs, "sdb")

	count, err := app.CountRecords("container_stats", dbx.HashExp{"system": sys.Id})
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}
