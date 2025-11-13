//go:generate -command fetchsmartctl go run ./tools/fetchsmartctl
//go:generate fetchsmartctl -out ./smartmontools/smartctl.exe -url https://static.beszel.dev/bin/smartctl/smartctl-nc.exe -sha 3912249c3b329249aa512ce796fd1b64d7cbd8378b68ad2756b39163d9c30b47

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/smart"

	"golang.org/x/exp/slog"
)

// SmartManager manages data collection for SMART devices
type SmartManager struct {
	sync.Mutex
	SmartDataMap    map[string]*smart.SmartData
	SmartDevices    []*DeviceInfo
	refreshMutex    sync.Mutex
	lastScanTime    time.Time
	binPath         string
	excludedDevices map[string]struct{}
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
	// typeVerified reports whether we have already parsed SMART data for this device
	// with the stored parserType. When true we can skip re-running the detection logic.
	typeVerified bool
	// parserType holds the parser type (nvme, sat, scsi) that last succeeded.
	parserType string
}

var errNoValidSmartData = fmt.Errorf("no valid SMART data found") // Error for missing data

// Refresh updates SMART data for all known devices
func (sm *SmartManager) Refresh(forceScan bool) error {
	sm.refreshMutex.Lock()
	defer sm.refreshMutex.Unlock()

	scanErr := sm.ScanDevices(false)
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
			slog.Debug("smartctl collect failed", "device", deviceInfo.Name, "err", err)
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
func (sm *SmartManager) ScanDevices(force bool) error {
	if !force && time.Since(sm.lastScanTime) < 30*time.Minute {
		return nil
	}
	sm.lastScanTime = time.Now()
	currentDevices := sm.devicesSnapshot()

	var configuredDevices []*DeviceInfo
	if configuredRaw, ok := GetEnv("SMART_DEVICES"); ok {
		slog.Info("SMART_DEVICES", "value", configuredRaw)
		config := strings.TrimSpace(configuredRaw)
		if config == "" {
			return errNoValidSmartData
		}

		parsedDevices, err := sm.parseConfiguredDevices(config)
		if err != nil {
			return err
		}
		configuredDevices = parsedDevices
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, sm.binPath, "--scan", "-j")
	output, err := cmd.Output()

	var (
		scanErr        error
		scannedDevices []*DeviceInfo
		hasValidScan   bool
	)

	if err != nil {
		scanErr = err
	} else {
		scannedDevices, hasValidScan = sm.parseScan(output)
		if !hasValidScan {
			scanErr = errNoValidSmartData
		}
	}

	finalDevices := mergeDeviceLists(currentDevices, scannedDevices, configuredDevices)
	finalDevices = sm.filterExcludedDevices(finalDevices)
	sm.updateSmartDevices(finalDevices)

	if len(finalDevices) == 0 {
		if scanErr != nil {
			slog.Debug("smartctl scan failed", "err", scanErr)
			return scanErr
		}
		return errNoValidSmartData
	}

	return nil
}

func (sm *SmartManager) parseConfiguredDevices(config string) ([]*DeviceInfo, error) {
	entries := strings.Split(config, ",")
	devices := make([]*DeviceInfo, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, ":", 2)

		name := strings.TrimSpace(parts[0])
		if name == "" {
			return nil, fmt.Errorf("invalid SMART_DEVICES entry %q", entry)
		}

		devType := ""
		if len(parts) == 2 {
			devType = strings.ToLower(strings.TrimSpace(parts[1]))
		}

		devices = append(devices, &DeviceInfo{
			Name: name,
			Type: devType,
		})
	}

	if len(devices) == 0 {
		return nil, errNoValidSmartData
	}

	return devices, nil
}

func (sm *SmartManager) refreshExcludedDevices() {
	rawValue, _ := GetEnv("EXCLUDE_SMART")
	sm.excludedDevices = make(map[string]struct{})

	for entry := range strings.SplitSeq(rawValue, ",") {
		device := strings.TrimSpace(entry)
		if device == "" {
			continue
		}
		sm.excludedDevices[device] = struct{}{}
	}
}

func (sm *SmartManager) isExcludedDevice(deviceName string) bool {
	_, exists := sm.excludedDevices[deviceName]
	return exists
}

func (sm *SmartManager) filterExcludedDevices(devices []*DeviceInfo) []*DeviceInfo {
	if devices == nil {
		return []*DeviceInfo{}
	}

	excluded := sm.excludedDevices
	if len(excluded) == 0 {
		return devices
	}

	filtered := make([]*DeviceInfo, 0, len(devices))
	for _, device := range devices {
		if device == nil || device.Name == "" {
			continue
		}
		if _, skip := excluded[device.Name]; skip {
			continue
		}
		filtered = append(filtered, device)
	}
	return filtered
}

// detectSmartOutputType inspects sections that are unique to each smartctl
// JSON schema (NVMe, ATA/SATA, SCSI) to determine which parser should be used
// when the reported device type is ambiguous or missing.
func detectSmartOutputType(output []byte) string {
	var hints struct {
		AtaSmartAttributes            json.RawMessage `json:"ata_smart_attributes"`
		NVMeSmartHealthInformationLog json.RawMessage `json:"nvme_smart_health_information_log"`
		ScsiErrorCounterLog           json.RawMessage `json:"scsi_error_counter_log"`
	}

	if err := json.Unmarshal(output, &hints); err != nil {
		return ""
	}

	switch {
	case hasJSONValue(hints.NVMeSmartHealthInformationLog):
		return "nvme"
	case hasJSONValue(hints.AtaSmartAttributes):
		return "sat"
	case hasJSONValue(hints.ScsiErrorCounterLog):
		return "scsi"
	default:
		return "sat"
	}
}

// hasJSONValue reports whether a JSON payload contains a concrete value. The
// smartctl output often emits "null" for sections that do not apply, so we
// only treat non-null content as a hint.
func hasJSONValue(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func normalizeParserType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "nvme", "sntasmedia", "sntrealtek":
		return "nvme"
	case "sat", "ata":
		return "sat"
	case "scsi":
		return "scsi"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// parseSmartOutput attempts each SMART parser, optionally detecting the type when
// it is not provided, and updates the device info when a parser succeeds.
func (sm *SmartManager) parseSmartOutput(deviceInfo *DeviceInfo, output []byte) bool {
	parsers := []struct {
		Type  string
		Parse func([]byte) (bool, int)
	}{
		{Type: "nvme", Parse: sm.parseSmartForNvme},
		{Type: "sat", Parse: sm.parseSmartForSata},
		{Type: "scsi", Parse: sm.parseSmartForScsi},
	}

	deviceType := normalizeParserType(deviceInfo.parserType)
	if deviceType == "" {
		deviceType = normalizeParserType(deviceInfo.Type)
	}
	if deviceInfo.parserType == "" {
		switch deviceType {
		case "nvme", "sat", "scsi":
			deviceInfo.parserType = deviceType
		}
	}

	// Only run the type detection when we do not yet know which parser works
	// or the previous attempt failed.
	needsDetection := deviceType == "" || !deviceInfo.typeVerified
	if needsDetection {
		structureType := detectSmartOutputType(output)
		if deviceType != structureType {
			deviceType = structureType
			deviceInfo.parserType = structureType
			deviceInfo.typeVerified = false
		}
		if deviceInfo.Type == "" || strings.EqualFold(deviceInfo.Type, structureType) {
			deviceInfo.Type = structureType
		}
	}

	// Try the most likely parser first, but keep the remaining parsers in reserve
	// so an incorrect hint never leaves the device unparsed.
	selectedParsers := make([]struct {
		Type  string
		Parse func([]byte) (bool, int)
	}, 0, len(parsers))
	if deviceType != "" {
		for _, parser := range parsers {
			if parser.Type == deviceType {
				selectedParsers = append(selectedParsers, parser)
				break
			}
		}
	}
	for _, parser := range parsers {
		alreadySelected := false
		for _, selected := range selectedParsers {
			if selected.Type == parser.Type {
				alreadySelected = true
				break
			}
		}
		if alreadySelected {
			continue
		}
		selectedParsers = append(selectedParsers, parser)
	}

	// Try the selected parsers in order until we find one that succeeds.
	for _, parser := range selectedParsers {
		hasData, _ := parser.Parse(output)
		if hasData {
			deviceInfo.parserType = parser.Type
			if deviceInfo.Type == "" || strings.EqualFold(deviceInfo.Type, parser.Type) {
				deviceInfo.Type = parser.Type
			}
			// Remember that this parser is valid so future refreshes can bypass
			// detection entirely.
			deviceInfo.typeVerified = true
			return true
		}
		slog.Debug("parser failed", "device", deviceInfo.Name, "parser", parser.Type)
	}

	// Leave verification false so the next pass will attempt detection again.
	deviceInfo.typeVerified = false
	slog.Debug("parsing failed", "device", deviceInfo.Name)
	return false
}

// CollectSmart collects SMART data for a device
// Collect data using `smartctl -d <type> -aj /dev/<device>` when device type is known
// Always attempts to parse output even if command fails, as some data may still be available
// If collect fails, return error
// If collect succeeds, parse the output and update the SmartDataMap
// Uses -n standby to avoid waking up sleeping disks, but bypasses standby mode
// for initial data collection when no cached data exists
func (sm *SmartManager) CollectSmart(deviceInfo *DeviceInfo) error {
	if deviceInfo != nil && sm.isExcludedDevice(deviceInfo.Name) {
		return errNoValidSmartData
	}

	// slog.Info("collecting SMART data", "device", deviceInfo.Name, "type", deviceInfo.Type, "has_existing_data", sm.hasDataForDevice(deviceInfo.Name))

	// Check if we have any existing data for this device
	hasExistingData := sm.hasDataForDevice(deviceInfo.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try with -n standby first if we have existing data
	args := sm.smartctlArgs(deviceInfo, true)
	cmd := exec.CommandContext(ctx, sm.binPath, args...)
	output, err := cmd.CombinedOutput()

	// Check if device is in standby (exit status 2)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
		if hasExistingData {
			// Device is in standby and we have cached data, keep using cache
			return nil
		}
		// No cached data, need to collect initial data by bypassing standby
		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()
		args = sm.smartctlArgs(deviceInfo, false)
		cmd = exec.CommandContext(ctx2, sm.binPath, args...)
		output, err = cmd.CombinedOutput()
	}

	hasValidData := sm.parseSmartOutput(deviceInfo, output)

	if !hasValidData {
		if err != nil {
			slog.Info("smartctl failed", "device", deviceInfo.Name, "err", err)
			return err
		}
		slog.Info("no valid SMART data found", "device", deviceInfo.Name)
		return errNoValidSmartData
	}

	return nil
}

// smartctlArgs returns the arguments for the smartctl command
// based on the device type and whether to include standby mode
func (sm *SmartManager) smartctlArgs(deviceInfo *DeviceInfo, includeStandby bool) []string {
	args := make([]string, 0, 7)

	if deviceInfo != nil {
		deviceType := strings.ToLower(deviceInfo.Type)
		// types sometimes misidentified in scan; see github.com/henrygd/beszel/issues/1345
		if deviceType != "" && deviceType != "scsi" && deviceType != "ata" {
			args = append(args, "-d", deviceInfo.Type)
		}
	}

	args = append(args, "-a", "--json=c")

	if includeStandby {
		args = append(args, "-n", "standby")
	}

	if deviceInfo != nil {
		args = append(args, deviceInfo.Name)
	}

	return args
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

// parseScan parses the output of smartctl --scan -j and returns the discovered devices.
func (sm *SmartManager) parseScan(output []byte) ([]*DeviceInfo, bool) {
	scan := &scanOutput{}

	if err := json.Unmarshal(output, scan); err != nil {
		return nil, false
	}

	if len(scan.Devices) == 0 {
		slog.Debug("no devices found in smartctl scan")
		return nil, false
	}

	devices := make([]*DeviceInfo, 0, len(scan.Devices))
	for _, device := range scan.Devices {
		slog.Debug("smartctl scan", "name", device.Name, "type", device.Type, "protocol", device.Protocol)
		devices = append(devices, &DeviceInfo{
			Name:     device.Name,
			Type:     device.Type,
			InfoName: device.InfoName,
			Protocol: device.Protocol,
		})
	}

	return devices, true
}

// mergeDeviceLists combines scanned and configured SMART devices, preferring
// configured SMART_DEVICES when both sources reference the same device.
func mergeDeviceLists(existing, scanned, configured []*DeviceInfo) []*DeviceInfo {
	if len(scanned) == 0 && len(configured) == 0 {
		return existing
	}

	// preserveVerifiedType copies the verified type/parser metadata from an existing
	// device record so that subsequent scans/config updates never downgrade a
	// previously verified device.
	preserveVerifiedType := func(target, prev *DeviceInfo) {
		if prev == nil || !prev.typeVerified {
			return
		}
		target.Type = prev.Type
		target.typeVerified = true
		target.parserType = prev.parserType
	}

	existingIndex := make(map[string]*DeviceInfo, len(existing))
	for _, dev := range existing {
		if dev == nil || dev.Name == "" {
			continue
		}
		existingIndex[dev.Name] = dev
	}

	finalDevices := make([]*DeviceInfo, 0, len(scanned)+len(configured))
	deviceIndex := make(map[string]*DeviceInfo, len(scanned)+len(configured))

	// Start with the newly scanned devices so we always surface fresh metadata,
	// but ensure we retain any previously verified parser assignment.
	for _, dev := range scanned {
		if dev == nil || dev.Name == "" {
			continue
		}

		// Work on a copy so we can safely adjust metadata without mutating the
		// input slices that may be reused elsewhere.
		copyDev := *dev
		if prev := existingIndex[copyDev.Name]; prev != nil {
			preserveVerifiedType(&copyDev, prev)
		}

		finalDevices = append(finalDevices, &copyDev)
		deviceIndex[copyDev.Name] = finalDevices[len(finalDevices)-1]
	}

	// Merge configured devices on top so users can override scan results (except
	// for verified type information).
	for _, dev := range configured {
		if dev == nil || dev.Name == "" {
			continue
		}

		if existingDev, ok := deviceIndex[dev.Name]; ok {
			// Only update the type if it has not been verified yet; otherwise we
			// keep the existing verified metadata intact.
			if dev.Type != "" && !existingDev.typeVerified {
				newType := strings.TrimSpace(dev.Type)
				existingDev.Type = newType
				existingDev.typeVerified = false
				existingDev.parserType = normalizeParserType(newType)
			}
			if dev.InfoName != "" {
				existingDev.InfoName = dev.InfoName
			}
			if dev.Protocol != "" {
				existingDev.Protocol = dev.Protocol
			}
			continue
		}

		copyDev := *dev
		if prev := existingIndex[copyDev.Name]; prev != nil {
			preserveVerifiedType(&copyDev, prev)
		} else if copyDev.Type != "" {
			copyDev.parserType = normalizeParserType(copyDev.Type)
		}

		finalDevices = append(finalDevices, &copyDev)
		deviceIndex[copyDev.Name] = finalDevices[len(finalDevices)-1]
	}

	return finalDevices
}

// updateSmartDevices replaces the cached device list and prunes SMART data
// entries whose backing device no longer exists.
func (sm *SmartManager) updateSmartDevices(devices []*DeviceInfo) {
	sm.Lock()
	defer sm.Unlock()

	sm.SmartDevices = devices

	if len(sm.SmartDataMap) == 0 {
		return
	}

	validNames := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		if device == nil || device.Name == "" {
			continue
		}
		validNames[device.Name] = struct{}{}
	}

	for key, data := range sm.SmartDataMap {
		if data == nil {
			delete(sm.SmartDataMap, key)
			continue
		}

		if _, ok := validNames[data.DiskName]; ok {
			continue
		}

		delete(sm.SmartDataMap, key)
	}
}

// isVirtualDevice checks if a device is a virtual disk that should be filtered out
func (sm *SmartManager) isVirtualDevice(data *smart.SmartInfoForSata) bool {
	vendorUpper := strings.ToUpper(data.ScsiVendor)
	productUpper := strings.ToUpper(data.ScsiProduct)
	modelUpper := strings.ToUpper(data.ModelName)

	return sm.isVirtualDeviceFromStrings(vendorUpper, productUpper, modelUpper)
}

// isVirtualDeviceNvme checks if an NVMe device is a virtual disk that should be filtered out
func (sm *SmartManager) isVirtualDeviceNvme(data *smart.SmartInfoForNvme) bool {
	modelUpper := strings.ToUpper(data.ModelName)

	return sm.isVirtualDeviceFromStrings(modelUpper)
}

// isVirtualDeviceScsi checks if a SCSI device is a virtual disk that should be filtered out
func (sm *SmartManager) isVirtualDeviceScsi(data *smart.SmartInfoForScsi) bool {
	vendorUpper := strings.ToUpper(data.ScsiVendor)
	productUpper := strings.ToUpper(data.ScsiProduct)
	modelUpper := strings.ToUpper(data.ScsiModelName)

	return sm.isVirtualDeviceFromStrings(vendorUpper, productUpper, modelUpper)
}

// isVirtualDeviceFromStrings checks if any of the provided strings indicate a virtual device
func (sm *SmartManager) isVirtualDeviceFromStrings(fields ...string) bool {
	for _, field := range fields {
		fieldUpper := strings.ToUpper(field)
		switch {
		case strings.Contains(fieldUpper, "IET"), // iSCSI Enterprise Target
			strings.Contains(fieldUpper, "VIRTUAL"),
			strings.Contains(fieldUpper, "QEMU"),
			strings.Contains(fieldUpper, "VBOX"),
			strings.Contains(fieldUpper, "VMWARE"),
			strings.Contains(fieldUpper, "MSFT"): // Microsoft Hyper-V
			return true
		}
	}
	return false
}

// parseSmartForSata parses the output of smartctl --all -j for SATA/ATA devices and updates the SmartDataMap
// Returns hasValidData and exitStatus
func (sm *SmartManager) parseSmartForSata(output []byte) (bool, int) {
	var data smart.SmartInfoForSata

	if err := json.Unmarshal(output, &data); err != nil {
		return false, 0
	}

	if data.SerialNumber == "" {
		slog.Debug("no serial number", "device", data.Device.Name)
		return false, data.Smartctl.ExitStatus
	}

	// Skip virtual devices (e.g., Kubernetes PVCs, QEMU, VirtualBox, etc.)
	if sm.isVirtualDevice(&data) {
		slog.Debug("skipping smart", "device", data.Device.Name, "model", data.ModelName)
		return false, data.Smartctl.ExitStatus
	}

	sm.Lock()
	defer sm.Unlock()

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
		rawValue := uint64(attr.Raw.Value)
		if parsed, ok := smart.ParseSmartRawValueString(attr.Raw.String); ok {
			rawValue = parsed
		}
		smartAttr := &smart.SmartAttribute{
			ID:         attr.ID,
			Name:       attr.Name,
			Value:      attr.Value,
			Worst:      attr.Worst,
			Threshold:  attr.Thresh,
			RawValue:   rawValue,
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

func (sm *SmartManager) parseSmartForScsi(output []byte) (bool, int) {
	var data smart.SmartInfoForScsi

	if err := json.Unmarshal(output, &data); err != nil {
		return false, 0
	}

	if data.SerialNumber == "" {
		slog.Debug("no serial number", "device", data.Device.Name)
		return false, data.Smartctl.ExitStatus
	}

	// Skip virtual devices (e.g., Kubernetes PVCs, QEMU, VirtualBox, etc.)
	if sm.isVirtualDeviceScsi(&data) {
		slog.Debug("skipping smart", "device", data.Device.Name, "model", data.ScsiModelName)
		return false, data.Smartctl.ExitStatus
	}

	sm.Lock()
	defer sm.Unlock()

	keyName := data.SerialNumber
	if _, ok := sm.SmartDataMap[keyName]; !ok {
		sm.SmartDataMap[keyName] = &smart.SmartData{}
	}

	smartData := sm.SmartDataMap[keyName]
	smartData.ModelName = data.ScsiModelName
	smartData.SerialNumber = data.SerialNumber
	smartData.FirmwareVersion = data.ScsiRevision
	smartData.Capacity = data.UserCapacity.Bytes
	smartData.Temperature = data.Temperature.Current
	smartData.SmartStatus = getSmartStatus(smartData.Temperature, data.SmartStatus.Passed)
	smartData.DiskName = data.Device.Name
	smartData.DiskType = data.Device.Type

	attributes := make([]*smart.SmartAttribute, 0, 10)
	attributes = append(attributes, &smart.SmartAttribute{Name: "PowerOnHours", RawValue: data.PowerOnTime.Hours})
	attributes = append(attributes, &smart.SmartAttribute{Name: "PowerOnMinutes", RawValue: data.PowerOnTime.Minutes})
	attributes = append(attributes, &smart.SmartAttribute{Name: "GrownDefectList", RawValue: data.ScsiGrownDefectList})
	attributes = append(attributes, &smart.SmartAttribute{Name: "StartStopCycles", RawValue: data.ScsiStartStopCycleCounter.AccumulatedStartStopCycles})
	attributes = append(attributes, &smart.SmartAttribute{Name: "LoadUnloadCycles", RawValue: data.ScsiStartStopCycleCounter.AccumulatedLoadUnloadCycles})
	attributes = append(attributes, &smart.SmartAttribute{Name: "StartStopSpecified", RawValue: data.ScsiStartStopCycleCounter.SpecifiedCycleCountOverDeviceLifetime})
	attributes = append(attributes, &smart.SmartAttribute{Name: "LoadUnloadSpecified", RawValue: data.ScsiStartStopCycleCounter.SpecifiedLoadUnloadCountOverDeviceLifetime})

	readStats := data.ScsiErrorCounterLog.Read
	writeStats := data.ScsiErrorCounterLog.Write
	verifyStats := data.ScsiErrorCounterLog.Verify

	attributes = append(attributes, &smart.SmartAttribute{Name: "ReadTotalErrorsCorrected", RawValue: readStats.TotalErrorsCorrected})
	attributes = append(attributes, &smart.SmartAttribute{Name: "ReadTotalUncorrectedErrors", RawValue: readStats.TotalUncorrectedErrors})
	attributes = append(attributes, &smart.SmartAttribute{Name: "ReadCorrectionAlgorithmInvocations", RawValue: readStats.CorrectionAlgorithmInvocations})
	if val := parseScsiGigabytesProcessed(readStats.GigabytesProcessed); val >= 0 {
		attributes = append(attributes, &smart.SmartAttribute{Name: "ReadGigabytesProcessed", RawValue: uint64(val)})
	}
	attributes = append(attributes, &smart.SmartAttribute{Name: "WriteTotalErrorsCorrected", RawValue: writeStats.TotalErrorsCorrected})
	attributes = append(attributes, &smart.SmartAttribute{Name: "WriteTotalUncorrectedErrors", RawValue: writeStats.TotalUncorrectedErrors})
	attributes = append(attributes, &smart.SmartAttribute{Name: "WriteCorrectionAlgorithmInvocations", RawValue: writeStats.CorrectionAlgorithmInvocations})
	if val := parseScsiGigabytesProcessed(writeStats.GigabytesProcessed); val >= 0 {
		attributes = append(attributes, &smart.SmartAttribute{Name: "WriteGigabytesProcessed", RawValue: uint64(val)})
	}
	attributes = append(attributes, &smart.SmartAttribute{Name: "VerifyTotalErrorsCorrected", RawValue: verifyStats.TotalErrorsCorrected})
	attributes = append(attributes, &smart.SmartAttribute{Name: "VerifyTotalUncorrectedErrors", RawValue: verifyStats.TotalUncorrectedErrors})
	attributes = append(attributes, &smart.SmartAttribute{Name: "VerifyCorrectionAlgorithmInvocations", RawValue: verifyStats.CorrectionAlgorithmInvocations})
	if val := parseScsiGigabytesProcessed(verifyStats.GigabytesProcessed); val >= 0 {
		attributes = append(attributes, &smart.SmartAttribute{Name: "VerifyGigabytesProcessed", RawValue: uint64(val)})
	}

	smartData.Attributes = attributes
	sm.SmartDataMap[keyName] = smartData

	return true, data.Smartctl.ExitStatus
}

func parseScsiGigabytesProcessed(value string) int64 {
	if value == "" {
		return -1
	}
	normalized := strings.ReplaceAll(value, ",", "")
	parsed, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil {
		return -1
	}
	return parsed
}

// parseSmartForNvme parses the output of smartctl --all -j /dev/nvmeX and updates the SmartDataMap
// Returns hasValidData and exitStatus
func (sm *SmartManager) parseSmartForNvme(output []byte) (bool, int) {
	data := &smart.SmartInfoForNvme{}

	if err := json.Unmarshal(output, &data); err != nil {
		return false, 0
	}

	if data.SerialNumber == "" {
		slog.Debug("no serial number", "device", data.Device.Name)
		return false, data.Smartctl.ExitStatus
	}

	// Skip virtual devices (e.g., Kubernetes PVCs, QEMU, VirtualBox, etc.)
	if sm.isVirtualDeviceNvme(data) {
		slog.Debug("skipping smart", "device", data.Device.Name, "model", data.ModelName)
		return false, data.Smartctl.ExitStatus
	}

	sm.Lock()
	defer sm.Unlock()

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
func (sm *SmartManager) detectSmartctl() (string, error) {
	isWindows := runtime.GOOS == "windows"

	// Load embedded smartctl.exe for Windows amd64 builds.
	if isWindows && runtime.GOARCH == "amd64" {
		if path, err := ensureEmbeddedSmartctl(); err == nil {
			return path, nil
		}
	}

	if path, err := exec.LookPath("smartctl"); err == nil {
		return path, nil
	}
	locations := []string{}
	if isWindows {
		locations = append(locations,
			"C:\\Program Files\\smartmontools\\bin\\smartctl.exe",
		)
	} else {
		locations = append(locations, "/opt/homebrew/bin/smartctl")
	}
	for _, location := range locations {
		if _, err := os.Stat(location); err == nil {
			return location, nil
		}
	}
	return "", errors.New("smartctl not found")
}

// NewSmartManager creates and initializes a new SmartManager
func NewSmartManager() (*SmartManager, error) {
	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}
	sm.refreshExcludedDevices()
	path, err := sm.detectSmartctl()
	if err != nil {
		slog.Debug(err.Error())
		return nil, err
	}
	slog.Debug("smartctl", "path", path)
	sm.binPath = path
	return sm, nil
}
