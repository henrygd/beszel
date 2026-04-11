package hub

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// listNetworkProbes handles GET /api/beszel/network-probes
func (h *Hub) listNetworkProbes(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("system parameter required", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	records, err := e.App.FindRecordsByFilter(
		"network_probes",
		"system = {:system}",
		"-created",
		0, 0,
		dbx.Params{"system": systemID},
	)
	if err != nil {
		return e.InternalServerError("", err)
	}

	type probeRecord struct {
		Id       string `json:"id"`
		Name     string `json:"name"`
		Target   string `json:"target"`
		Protocol string `json:"protocol"`
		Port     int    `json:"port"`
		Interval int    `json:"interval"`
		Enabled  bool   `json:"enabled"`
	}

	result := make([]probeRecord, 0, len(records))
	for _, r := range records {
		result = append(result, probeRecord{
			Id:       r.Id,
			Name:     r.GetString("name"),
			Target:   r.GetString("target"),
			Protocol: r.GetString("protocol"),
			Port:     r.GetInt("port"),
			Interval: r.GetInt("interval"),
			Enabled:  r.GetBool("enabled"),
		})
	}

	return e.JSON(http.StatusOK, result)
}

// createNetworkProbe handles POST /api/beszel/network-probes
func (h *Hub) createNetworkProbe(e *core.RequestEvent) error {
	var req struct {
		System   string `json:"system"`
		Name     string `json:"name"`
		Target   string `json:"target"`
		Protocol string `json:"protocol"`
		Port     int    `json:"port"`
		Interval int    `json:"interval"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.BadRequestError("invalid request body", err)
	}
	if req.System == "" || req.Target == "" || req.Protocol == "" {
		return e.BadRequestError("system, target, and protocol are required", nil)
	}
	if req.Protocol != "icmp" && req.Protocol != "tcp" && req.Protocol != "http" {
		return e.BadRequestError("protocol must be icmp, tcp, or http", nil)
	}
	if req.Protocol == "http" && !strings.HasPrefix(req.Target, "http://") && !strings.HasPrefix(req.Target, "https://") {
		return e.BadRequestError("http probe target must start with http:// or https://", nil)
	}
	if req.Interval <= 0 {
		req.Interval = 10
	}

	system, err := h.sm.GetSystem(req.System)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	collection, err := e.App.FindCachedCollectionByNameOrId("network_probes")
	if err != nil {
		return e.InternalServerError("", err)
	}

	record := core.NewRecord(collection)
	record.Set("system", req.System)
	record.Set("name", req.Name)
	record.Set("target", req.Target)
	record.Set("protocol", req.Protocol)
	record.Set("port", req.Port)
	record.Set("interval", req.Interval)
	record.Set("enabled", true)

	if err := e.App.Save(record); err != nil {
		return e.InternalServerError("", err)
	}

	// Sync probes to agent
	h.syncProbesToAgent(req.System)

	return e.JSON(http.StatusOK, map[string]string{"id": record.Id})
}

// deleteNetworkProbe handles DELETE /api/beszel/network-probes
func (h *Hub) deleteNetworkProbe(e *core.RequestEvent) error {
	probeID := e.Request.URL.Query().Get("id")
	if probeID == "" {
		return e.BadRequestError("id parameter required", nil)
	}

	record, err := e.App.FindRecordById("network_probes", probeID)
	if err != nil {
		return e.NotFoundError("", nil)
	}

	systemID := record.GetString("system")
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	if err := e.App.Delete(record); err != nil {
		return e.InternalServerError("", err)
	}

	// Sync probes to agent
	h.syncProbesToAgent(systemID)

	return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// getNetworkProbeStats handles GET /api/beszel/network-probe-stats
func (h *Hub) getNetworkProbeStats(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	statsType := e.Request.URL.Query().Get("type")
	if systemID == "" {
		return e.BadRequestError("system parameter required", nil)
	}
	if statsType == "" {
		statsType = "1m"
	}

	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	records, err := e.App.FindRecordsByFilter(
		"network_probe_stats",
		"system = {:system} && type = {:type}",
		"created",
		0, 0,
		dbx.Params{"system": systemID, "type": statsType},
	)
	if err != nil {
		return e.InternalServerError("", err)
	}

	type statsRecord struct {
		Stats   json.RawMessage `json:"stats"`
		Created string          `json:"created"`
	}

	result := make([]statsRecord, 0, len(records))
	for _, r := range records {
		statsJSON, _ := json.Marshal(r.Get("stats"))
		result = append(result, statsRecord{
			Stats:   statsJSON,
			Created: r.GetDateTime("created").Time().UTC().Format("2006-01-02 15:04:05.000Z"),
		})
	}

	return e.JSON(http.StatusOK, result)
}

// syncProbesToAgent fetches enabled probes for a system and sends them to the agent.
func (h *Hub) syncProbesToAgent(systemID string) {
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return
	}

	configs := h.sm.GetProbeConfigsForSystem(systemID)

	go func() {
		if err := system.SyncNetworkProbes(configs); err != nil {
			h.Logger().Warn("failed to sync probes to agent", "system", systemID, "err", err)
		}
	}()
}
