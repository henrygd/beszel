package systems

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

// FetchSmartDataFromAgent fetches SMART data from the agent
func (sys *System) FetchSmartDataFromAgent() (map[string]smart.SmartData, error) {
	// fetch via websocket
	if sys.WsConn != nil && sys.WsConn.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return sys.WsConn.RequestSmartData(ctx)
	}
	// fetch via SSH
	var result map[string]smart.SmartData
	err := sys.runSSHOperation(5*time.Second, 1, func(session *ssh.Session) (bool, error) {
		stdout, err := session.StdoutPipe()
		if err != nil {
			return false, err
		}
		stdin, stdinErr := session.StdinPipe()
		if stdinErr != nil {
			return false, stdinErr
		}
		if err := session.Shell(); err != nil {
			return false, err
		}
		req := common.HubRequest[any]{Action: common.GetSmartData}
		_ = cbor.NewEncoder(stdin).Encode(req)
		_ = stdin.Close()
		var resp common.AgentResponse
		if err := cbor.NewDecoder(stdout).Decode(&resp); err != nil {
			return false, err
		}
		result = resp.SmartData
		return false, nil
	})
	return result, err
}

// FetchAndSaveSmartDevices fetches SMART data from the agent and saves it to the database
func (sys *System) FetchAndSaveSmartDevices() error {
	smartData, err := sys.FetchSmartDataFromAgent()
	if err != nil || len(smartData) == 0 {
		return err
	}
	return sys.saveSmartDevices(smartData)
}

// saveSmartDevices saves SMART device data to the smart_devices collection
func (sys *System) saveSmartDevices(smartData map[string]smart.SmartData) error {
	if len(smartData) == 0 {
		return nil
	}

	hub := sys.manager.hub
	collection, err := hub.FindCachedCollectionByNameOrId("smart_devices")
	if err != nil {
		return err
	}

	for deviceKey, device := range smartData {
		if err := sys.upsertSmartDeviceRecord(collection, deviceKey, device); err != nil {
			return err
		}
	}

	return nil
}

func (sys *System) upsertSmartDeviceRecord(collection *core.Collection, deviceKey string, device smart.SmartData) error {
	hub := sys.manager.hub
	recordID := makeStableHashId(sys.Id, deviceKey)

	record, err := hub.FindRecordById(collection, recordID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("id", recordID)
	}

	name := device.DiskName
	if name == "" {
		name = deviceKey
	}

	powerOnHours, powerCycles := extractPowerMetrics(device.Attributes)
	record.Set("system", sys.Id)
	record.Set("name", name)
	record.Set("model", device.ModelName)
	record.Set("state", device.SmartStatus)
	record.Set("capacity", device.Capacity)
	record.Set("temp", device.Temperature)
	record.Set("firmware", device.FirmwareVersion)
	record.Set("serial", device.SerialNumber)
	record.Set("type", device.DiskType)
	record.Set("hours", powerOnHours)
	record.Set("cycles", powerCycles)
	record.Set("attributes", device.Attributes)

	return hub.SaveNoValidate(record)
}

// extractPowerMetrics extracts power on hours and power cycles from SMART attributes
func extractPowerMetrics(attributes []*smart.SmartAttribute) (powerOnHours, powerCycles uint64) {
	for _, attr := range attributes {
		nameLower := strings.ToLower(attr.Name)
		if powerOnHours == 0 && (strings.Contains(nameLower, "poweronhours") || strings.Contains(nameLower, "power_on_hours")) {
			powerOnHours = attr.RawValue
		}
		if powerCycles == 0 && ((strings.Contains(nameLower, "power") && strings.Contains(nameLower, "cycle")) || strings.Contains(nameLower, "startstopcycles")) {
			powerCycles = attr.RawValue
		}
		if powerOnHours > 0 && powerCycles > 0 {
			break
		}
	}
	return
}
