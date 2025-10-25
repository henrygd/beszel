package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/smart"

	"golang.org/x/exp/slog"
)

// SmartManager manages data collection for SMART devices
type SmartManager struct {
	sync.Mutex
	SmartDataMap map[string]*smart.SmartData
	SmartDevices []*DeviceInfo
	refreshMutex sync.Mutex
}

type scanOutput struct {
	Devices []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		InfoName string `json:"info_name"`
		Protocol string `json:"protocol"`
	} `json:"devices"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	InfoName string `json:"info_name"`
	Protocol string `json:"protocol"`
}

var errNoValidSmartData = fmt.Errorf("no valid SMART data found") // Error for missing data

// Refresh updates SMART data for all known devices on demand.
func (sm *SmartManager) Refresh() error {
	sm.refreshMutex.Lock()
	defer sm.refreshMutex.Unlock()

	scanErr := sm.ScanDevices()
	if scanErr != nil {
		slog.Debug("smartctl scan failed", "err", scanErr)
	}

	devices := sm.devicesSnapshot()
	var collectErr error
	for _, deviceInfo := range devices {
		if deviceInfo == nil {
			continue
		}
		if err := sm.CollectSmart(deviceInfo); err != nil {
			slog.Debug("smartctl collect failed, skipping", "device", deviceInfo.Name, "err", err)
			collectErr = err
		}
	}

	return sm.resolveRefreshError(scanErr, collectErr)
}

// devicesSnapshot returns a copy of the current device slice to avoid iterating
// while holding the primary mutex for longer than necessary.
func (sm *SmartManager) devicesSnapshot() []*DeviceInfo {
	sm.Lock()
	defer sm.Unlock()

	devices := make([]*DeviceInfo, len(sm.SmartDevices))
	copy(devices, sm.SmartDevices)
	return devices
}

// hasSmartData reports whether any SMART data has been collected.
// func (sm *SmartManager) hasSmartData() bool {
// 	sm.Lock()
// 	defer sm.Unlock()

// 	return len(sm.SmartDataMap) > 0
// }

// resolveRefreshError determines the proper error to return after a refresh.
func (sm *SmartManager) resolveRefreshError(scanErr, collectErr error) error {
	sm.Lock()
	noDevices := len(sm.SmartDevices) == 0
	noData := len(sm.SmartDataMap) == 0
	sm.Unlock()

	if noDevices {
		if scanErr != nil {
			return scanErr
		}
	}

	if !noData {
		return nil
	}

	if collectErr != nil {
		return collectErr
	}
	if scanErr != nil {
		return scanErr
	}
	return errNoValidSmartData
}

// GetCurrentData returns the current SMART data
func (sm *SmartManager) GetCurrentData() map[string]smart.SmartData {
	sm.Lock()
	defer sm.Unlock()
	result := make(map[string]smart.SmartData, len(sm.SmartDataMap))
	for key, value := range sm.SmartDataMap {
		if value != nil {
			result[key] = *value
		}
	}
	return result
}

// ScanDevices scans for SMART devices
// Scan devices using `smartctl --scan -j`
// If scan fails, return error
// If scan succeeds, parse the output and update the SmartDevices slice
func (sm *SmartManager) ScanDevices() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "smartctl", "--scan", "-j")
	output, err := cmd.Output()

	if err != nil {
		return err
	}

	hasValidData := sm.parseScan(output)
	if !hasValidData {
		return errNoValidSmartData
	}
	return nil
}

// CollectSmart collects SMART data for a device
// Collect data using `smartctl --all -j /dev/sdX` or `smartctl --all -j /dev/nvmeX`
// Always attempts to parse output even if command fails, as some data may still be available
// If collect fails, return error
// If collect succeeds, parse the output and update the SmartDataMap
// Uses -n standby to avoid waking up sleeping disks, but bypasses standby mode
// for initial data collection when no cached data exists
func (sm *SmartManager) CollectSmart(deviceInfo *DeviceInfo) error {
	// Check if we have any existing data for this device
	hasExistingData := sm.hasDataForDevice(deviceInfo.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try with -n standby first if we have existing data
	cmd := exec.CommandContext(ctx, "smartctl", "-aj", "-n", "standby", deviceInfo.Name)
	output, err := cmd.CombinedOutput()

	// Check if device is in standby (exit status 2)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
		if hasExistingData {
			// Device is in standby and we have cached data, keep using cache
			slog.Debug("device in standby mode, using cached data", "device", deviceInfo.Name)
			return nil
		}
		// No cached data, need to collect initial data by bypassing standby
		slog.Debug("device in standby but no cached data, collecting initial data", "device", deviceInfo.Name)
		ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()
		cmd = exec.CommandContext(ctx2, "smartctl", "-aj", deviceInfo.Name)
		output, err = cmd.CombinedOutput()
	}

	hasValidData := false

	switch deviceInfo.Type {
	case "scsi", "sat", "ata":
		// parse SATA/SCSI/ATA devices
		hasValidData, _ = sm.parseSmartForSata(output)
	case "nvme":
		// parse nvme devices
		hasValidData, _ = sm.parseSmartForNvme(output)
	}

	if !hasValidData {
		if err != nil {
			return err
		}
		return errNoValidSmartData
	}
	return nil
}

// hasDataForDevice checks if we have cached SMART data for a specific device
func (sm *SmartManager) hasDataForDevice(deviceName string) bool {
	sm.Lock()
	defer sm.Unlock()

	// Check if any cached data has this device name
	for _, data := range sm.SmartDataMap {
		if data != nil && data.DiskName == deviceName {
			return true
		}
	}
	return false
}

// parseScan parses the output of smartctl --scan -j and updates the SmartDevices slice
func (sm *SmartManager) parseScan(output []byte) bool {
	sm.Lock()
	defer sm.Unlock()

	sm.SmartDevices = make([]*DeviceInfo, 0)
	scan := &scanOutput{}

	if err := json.Unmarshal(output, scan); err != nil {
		slog.Debug("Failed to parse smartctl scan JSON", "err", err)
		return false
	}

	if len(scan.Devices) == 0 {
		return false
	}

	scannedDeviceNameMap := make(map[string]bool, len(scan.Devices))

	for _, device := range scan.Devices {
		deviceInfo := &DeviceInfo{
			Name:     device.Name,
			Type:     device.Type,
			InfoName: device.InfoName,
			Protocol: device.Protocol,
		}
		sm.SmartDevices = append(sm.SmartDevices, deviceInfo)
		scannedDeviceNameMap[device.Name] = true
	}
	// remove devices that are not in the scan
	for key := range sm.SmartDataMap {
		if _, ok := scannedDeviceNameMap[key]; !ok {
			delete(sm.SmartDataMap, key)
		}
	}

	return true
}

// parseSmartForSata parses the output of smartctl --all -j for SATA/ATA devices and updates the SmartDataMap
// Returns hasValidData and exitStatus
func (sm *SmartManager) parseSmartForSata(output []byte) (bool, int) {
	var data smart.SmartInfoForSata

	if err := json.Unmarshal(output, &data); err != nil {
		return false, 0
	}

	if data.SerialNumber == "" {
		slog.Debug("device has no serial number, skipping", "device", data.Device.Name)
		return false, data.Smartctl.ExitStatus
	}

	sm.Lock()
	defer sm.Unlock()

	// get device name (e.g. /dev/sda)
	keyName := data.SerialNumber

	// if device does not exist in SmartDataMap, initialize it
	if _, ok := sm.SmartDataMap[keyName]; !ok {
		sm.SmartDataMap[keyName] = &smart.SmartData{}
	}

	// update SmartData
	smartData := sm.SmartDataMap[keyName]
	// smartData.ModelFamily = data.ModelFamily
	smartData.ModelName = data.ModelName
	smartData.SerialNumber = data.SerialNumber
	smartData.FirmwareVersion = data.FirmwareVersion
	smartData.Capacity = data.UserCapacity.Bytes
	smartData.Temperature = data.Temperature.Current
	smartData.SmartStatus = getSmartStatus(smartData.Temperature, data.SmartStatus.Passed)
	smartData.DiskName = data.Device.Name
	smartData.DiskType = data.Device.Type

	// update SmartAttributes
	smartData.Attributes = make([]*smart.SmartAttribute, 0, len(data.AtaSmartAttributes.Table))
	for _, attr := range data.AtaSmartAttributes.Table {
		smartAttr := &smart.SmartAttribute{
			ID:         attr.ID,
			Name:       attr.Name,
			Value:      attr.Value,
			Worst:      attr.Worst,
			Threshold:  attr.Thresh,
			RawValue:   attr.Raw.Value,
			RawString:  attr.Raw.String,
			WhenFailed: attr.WhenFailed,
		}
		smartData.Attributes = append(smartData.Attributes, smartAttr)
	}
	sm.SmartDataMap[keyName] = smartData

	return true, data.Smartctl.ExitStatus
}

func getSmartStatus(temperature uint8, passed bool) string {
	if passed {
		return "PASSED"
	} else if temperature > 0 {
		return "FAILED"
	} else {
		return "UNKNOWN"
	}
}

// parseSmartForNvme parses the output of smartctl --all -j /dev/nvmeX and updates the SmartDataMap
// Returns hasValidData and exitStatus
func (sm *SmartManager) parseSmartForNvme(output []byte) (bool, int) {
	data := &smart.SmartInfoForNvme{}

	if err := json.Unmarshal(output, &data); err != nil {
		return false, 0
	}

	if data.SerialNumber == "" {
		slog.Debug("device has no serial number, skipping", "device", data.Device.Name)
		return false, data.Smartctl.ExitStatus
	}

	sm.Lock()
	defer sm.Unlock()

	// get device name (e.g. /dev/nvme0)
	keyName := data.SerialNumber

	// if device does not exist in SmartDataMap, initialize it
	if _, ok := sm.SmartDataMap[keyName]; !ok {
		sm.SmartDataMap[keyName] = &smart.SmartData{}
	}

	// update SmartData
	smartData := sm.SmartDataMap[keyName]
	smartData.ModelName = data.ModelName
	smartData.SerialNumber = data.SerialNumber
	smartData.FirmwareVersion = data.FirmwareVersion
	smartData.Capacity = data.UserCapacity.Bytes
	smartData.Temperature = data.NVMeSmartHealthInformationLog.Temperature
	smartData.SmartStatus = getSmartStatus(smartData.Temperature, data.SmartStatus.Passed)
	smartData.DiskName = data.Device.Name
	smartData.DiskType = data.Device.Type

	// nvme attributes does not follow the same format as ata attributes,
	// so we manually map each field to SmartAttributes
	log := data.NVMeSmartHealthInformationLog
	smartData.Attributes = []*smart.SmartAttribute{
		{Name: "CriticalWarning", RawValue: uint64(log.CriticalWarning)},
		{Name: "Temperature", RawValue: uint64(log.Temperature)},
		{Name: "AvailableSpare", RawValue: uint64(log.AvailableSpare)},
		{Name: "AvailableSpareThreshold", RawValue: uint64(log.AvailableSpareThreshold)},
		{Name: "PercentageUsed", RawValue: uint64(log.PercentageUsed)},
		{Name: "DataUnitsRead", RawValue: log.DataUnitsRead},
		{Name: "DataUnitsWritten", RawValue: log.DataUnitsWritten},
		{Name: "HostReads", RawValue: uint64(log.HostReads)},
		{Name: "HostWrites", RawValue: uint64(log.HostWrites)},
		{Name: "ControllerBusyTime", RawValue: uint64(log.ControllerBusyTime)},
		{Name: "PowerCycles", RawValue: uint64(log.PowerCycles)},
		{Name: "PowerOnHours", RawValue: uint64(log.PowerOnHours)},
		{Name: "UnsafeShutdowns", RawValue: uint64(log.UnsafeShutdowns)},
		{Name: "MediaErrors", RawValue: uint64(log.MediaErrors)},
		{Name: "NumErrLogEntries", RawValue: uint64(log.NumErrLogEntries)},
		{Name: "WarningTempTime", RawValue: uint64(log.WarningTempTime)},
		{Name: "CriticalCompTime", RawValue: uint64(log.CriticalCompTime)},
	}

	sm.SmartDataMap[keyName] = smartData

	return true, data.Smartctl.ExitStatus
}

// detectSmartctl checks if smartctl is installed, returns an error if not
func (sm *SmartManager) detectSmartctl() error {
	if _, err := exec.LookPath("smartctl"); err == nil {
		return nil
	}
	return fmt.Errorf("smartctl not found")
}

// NewSmartManager creates and initializes a new SmartManager
func NewSmartManager() (*SmartManager, error) {
	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}
	if err := sm.detectSmartctl(); err != nil {
		return nil, err
	}

	return sm, nil
}
