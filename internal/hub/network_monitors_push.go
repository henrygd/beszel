package hub

import (
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	protocolPush = "push"
	// maxProbeInterval is the longest interval in seconds for monitors that are
	// probed by an agent. Push monitors may use a longer heartbeat interval.
	maxProbeInterval = 3600
)

const intervalTooLongMessage = "Interval must be 3600 seconds or less for this protocol"

// monitorIntervalTooLong reports whether a probe monitor exceeds the probe interval limit.
func monitorIntervalTooLong(record *core.Record) bool {
	return record.GetString("protocol") != protocolPush && record.GetInt("interval") > maxProbeInterval
}

// newPushToken returns a random token that identifies a push monitor in its URL.
func newPushToken() string {
	return security.RandomString(32)
}

// handleMonitorPush records a heartbeat for the push monitor that owns the token in the URL.
// The system's next stats update evaluates whether heartbeats arrive on time.
func (h *Hub) handleMonitorPush(e *core.RequestEvent) error {
	token := e.Request.PathValue("token")
	if token == "" {
		return e.NotFoundError("", nil)
	}
	res, err := h.DB().NewQuery("UPDATE network_monitors SET lastPush = {:now} WHERE pushToken = {:token} AND protocol = 'push' AND enabled = true").
		Bind(dbx.Params{"now": time.Now().UnixMilli(), "token": token}).
		Execute()
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return e.NotFoundError("", nil)
	}
	return e.NoContent(http.StatusNoContent)
}
