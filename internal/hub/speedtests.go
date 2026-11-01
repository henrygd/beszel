package hub

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
)

// generateSpeedtestID creates a stable hash ID for a speedtest based on its system,
// server and interface. The default interface is left out of the hash.
func generateSpeedtestID(systemID string, config speedtest.Config) string {
	parts := []string{systemID, "speedtest", strconv.FormatUint(uint64(config.ServerID), 10)}
	if config.Interface != "" {
		parts = append(parts, config.Interface)
	}
	return systems.MakeStableHashId(parts...)
}

// errDuplicateSpeedtest is returned when a system already has a speedtest for the chosen server and interface.
const errDuplicateSpeedtest = "This system already has a speedtest for this server and interface."

// speedtestExists reports whether a speedtest record with the given ID exists.
func speedtestExists(app core.App, id string) bool {
	_, err := app.FindRecordById("speedtests", id)
	return err == nil
}

// bindSpeedtestsEvents keeps speedtest records and agent speedtest state in sync.
func bindSpeedtestsEvents(hub *Hub) {
	// on create, normalize the interface and make sure the id is set to a stable hash
	hub.OnRecordCreate("speedtests").BindFunc(func(e *core.RecordEvent) error {
		config := speedtestConfigFromRecord(e.Record)
		e.Record.Set("interface", config.Interface)
		e.Record.Set("id", generateSpeedtestID(e.Record.GetString("system"), config))
		return e.Next()
	})

	// reject API creates that duplicate an existing speedtest with a clear message
	hub.OnRecordCreateRequest("speedtests").BindFunc(func(e *core.RecordRequestEvent) error {
		ID := generateSpeedtestID(e.Record.GetString("system"), speedtestConfigFromRecord(e.Record))
		if speedtestExists(e.App, ID) {
			return e.BadRequestError(errDuplicateSpeedtest, nil)
		}
		return e.Next()
	})

	// sync speedtest to agent on creation and start a first run
	hub.OnRecordAfterCreateSuccess("speedtests").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if !e.Record.GetBool("enabled") {
			return nil
		}
		// Paused systems may be absent from the manager; their speedtests sync when they reconnect.
		system, err := hub.sm.GetSystem(e.Record.GetString("system"))
		if err == nil && system.GetStatus() == "up" {
			go func() {
				if err := hub.upsertSpeedtest(e.Record, true); err != nil {
					hub.Logger().Warn("failed to sync new speedtest", "system", system.Id, "speedtest", e.Record.Id, "err", err)
				}
			}()
		}
		return nil
	})

	// On API update requests, if the server or interface changed, replace the record
	// so its ID stays a stable hash. Otherwise, update the speedtest on the agent.
	hub.OnRecordUpdateRequest("speedtests").BindFunc(func(e *core.RecordRequestEvent) error {
		systemID := e.Record.GetString("system")
		config := speedtestConfigFromRecord(e.Record)
		e.Record.Set("interface", config.Interface)
		ID := generateSpeedtestID(systemID, config)
		if ID != e.Record.Id {
			if speedtestExists(e.App, ID) {
				return e.BadRequestError(errDuplicateSpeedtest, nil)
			}
			if err := e.App.Save(copySpeedtestToNewRecord(e.Record, ID)); err != nil {
				return err
			}
			return e.App.Delete(e.Record)
		}
		if err := e.Next(); err != nil {
			return err
		}
		var err error
		if e.Record.GetBool("enabled") {
			runNow := !e.Record.Original().GetBool("enabled")
			err = hub.upsertSpeedtest(e.Record, runNow)
		} else {
			err = hub.deleteSpeedtest(e.Record)
		}
		if err != nil {
			hub.Logger().Warn("failed to sync updated speedtest", "system", systemID, "speedtest", e.Record.Id, "err", err)
		}
		return nil
	})

	// sync speedtest to agent on delete
	hub.OnRecordAfterDeleteSuccess("speedtests").BindFunc(func(e *core.RecordEvent) error {
		if err := hub.deleteSpeedtest(e.Record); err != nil {
			hub.Logger().Warn("failed to delete speedtest on agent", "system", e.Record.GetString("system"), "speedtest", e.Record.Id, "err", err)
		}
		return e.Next()
	})
}

// speedtestConfigFromRecord builds a speedtest config from a speedtests record.
func speedtestConfigFromRecord(record *core.Record) speedtest.Config {
	return speedtest.Config{
		ID:        record.Id,
		ServerID:  uint32(max(record.GetInt("server_id"), 0)),
		Interval:  uint32(max(record.GetInt("interval"), speedtest.MinInterval)),
		Interface: strings.TrimSpace(record.GetString("interface")),
	}
}

// copySpeedtestToNewRecord creates a new record with the old record's settings. It is
// used when the server or interface changes, since those are part of the record ID.
func copySpeedtestToNewRecord(oldRecord *core.Record, newID string) *core.Record {
	newRecord := core.NewRecord(oldRecord.Collection())
	newRecord.Id = newID
	for _, field := range []string{"system", "server_id", "server_name", "server_location", "interface", "interval", "enabled"} {
		newRecord.Set(field, oldRecord.Get(field))
	}
	return newRecord
}

// upsertSpeedtest creates or updates the record's speedtest on its system's agent.
func (h *Hub) upsertSpeedtest(record *core.Record, runNow bool) error {
	system, err := h.sm.GetSystem(record.GetString("system"))
	if err != nil {
		return err
	}
	return system.UpsertSpeedtest(speedtestConfigFromRecord(record), runNow)
}

// runSpeedtest handles POST /api/beszel/speedtest/run requests. It starts a run
// in the background; the result arrives with the agent's next stats.
func (h *Hub) runSpeedtest(e *core.RequestEvent) error {
	id := e.Request.URL.Query().Get("id")
	if id == "" {
		return e.BadRequestError("Invalid id parameter", nil)
	}
	record, err := e.App.FindRecordById("speedtests", id)
	if err != nil {
		return e.NotFoundError("", nil)
	}
	system, err := h.sm.GetSystem(record.GetString("system"))
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}
	// Paused speedtests have no task on the agent, and running one would schedule it.
	if !record.GetBool("enabled") {
		return e.BadRequestError("Speedtest is paused.", nil)
	}
	if system.GetStatus() != "up" {
		return e.BadRequestError("System is not connected.", nil)
	}
	if err := h.upsertSpeedtest(record, true); err != nil {
		return e.InternalServerError("", err)
	}
	return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// deleteSpeedtest removes the record's speedtest from its system's agent.
func (h *Hub) deleteSpeedtest(record *core.Record) error {
	system, err := h.sm.GetSystem(record.GetString("system"))
	if err != nil {
		return err
	}
	return system.DeleteSpeedtest(record.Id)
}
