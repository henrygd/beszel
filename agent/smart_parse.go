package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/smart"
)

var errNoHealthData = errors.New("no SMART health data in smartctl output")

// parseSmartctl turns `smartctl -a --json` output into SmartData and returns
// the protocol it was parsed as ("nvme", "scsi" or "ata"). dtype is the -d
// value used for the query; when set it is reported as the disk type.
//
// The protocol comes from smartctl's device.protocol, falling back to the
// sections present. Output that identifies a disk but holds no health data for
// its protocol is rejected, so the next smartctl attempt can run instead of a
// record of zeros being stored (#2295, #1504).
func (sm *SmartManager) parseSmartctl(output []byte, dtype string) (*smart.SmartData, string, error) {
	data, err := decodeSmartctlOutput(output)
	if err != nil {
		return nil, "", err
	}

	protocol := smartProtocol(data)
	switch protocol {
	case "nvme":
		log := data.NVMeSmartHealthInformationLog
		if log == nil {
			return nil, protocol, errNoHealthData
		}
		smartData := newSmartData(data, dtype, data.ModelName, data.FirmwareVersion, log.Temperature)
		if smartData.Capacity == 0 {
			smartData.Capacity = data.NVMeTotalCapacity
		}
		if smartData.Capacity == 0 && (runtime.GOOS == "darwin" || sm.darwinNvmeProvider != nil) {
			smartData.Capacity = sm.lookupDarwinNvmeCapacity(data.SerialNumber)
		}
		smartData.Attributes = nvmeAttributes(log, smartData.Temperature)
		return smartData, protocol, nil

	case "scsi":
		if data.SmartStatus.Passed == nil && data.ScsiErrorCounterLog == nil && data.Temperature.Current == 0 {
			return nil, protocol, errNoHealthData
		}
		smartData := newSmartData(data, dtype, data.ScsiModelName, data.ScsiRevision, data.Temperature.Current)
		smartData.Attributes = scsiAttributes(data)
		return smartData, protocol, nil

	default: // ata
		var table []smart.AtaSmartAttribute
		if data.AtaSmartAttributes != nil {
			table = data.AtaSmartAttributes.Table
		}
		if len(table) == 0 && data.SmartStatus.Passed == nil && data.Temperature.Current == 0 && !hasJSONValue(data.AtaDeviceStatistics) {
			return nil, protocol, errNoHealthData
		}
		smartData := newSmartData(data, dtype, data.ModelName, data.FirmwareVersion, data.Temperature.Current)
		if smartData.Temperature == 0 {
			var ataDeviceStats smart.AtaDeviceStatistics
			if temp := findAtaDeviceStatisticsValue(data, &ataDeviceStats, 5, "Current Temperature", 0, 255); temp != nil {
				smartData.Temperature = uint8(*temp)
			}
		}
		smartData.Attributes = ataAttributes(table)
		return smartData, protocol, nil
	}
}

// smartProtocol returns "nvme", "scsi" or "ata" for a smartctl response.
func smartProtocol(data *smart.SmartctlOutput) string {
	switch strings.ToUpper(data.Device.Protocol) {
	case "NVME":
		return "nvme"
	case "SCSI":
		return "scsi"
	case "ATA":
		return "ata"
	}
	switch {
	case data.NVMeSmartHealthInformationLog != nil, strings.EqualFold(data.Device.Type, "nvme"):
		return "nvme"
	case data.AtaSmartAttributes != nil:
		return "ata"
	case data.ScsiErrorCounterLog != nil, data.ScsiModelName != "":
		return "scsi"
	default:
		return "ata"
	}
}

// hasJSONValue reports whether a raw JSON value is present and not null.
func hasJSONValue(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

// isVirtualDevice checks if a device is a virtual disk that should be filtered out
// (e.g., Kubernetes PVCs, QEMU, VirtualBox, etc.)
func isVirtualDevice(data *smart.SmartctlOutput) bool {
	for _, field := range []string{data.ScsiVendor, data.ScsiProduct, data.ModelName, data.ScsiModelName} {
		field = strings.ToUpper(field)
		switch {
		case strings.Contains(field, "IET"), // iSCSI Enterprise Target
			strings.Contains(field, "VIRTUAL"),
			strings.Contains(field, "QEMU"),
			strings.Contains(field, "VBOX"),
			strings.Contains(field, "VMWARE"),
			strings.Contains(field, "MSFT"): // Microsoft Hyper-V
			return true
		}
	}
	return false
}

// decodeSmartctlOutput unmarshals smartctl JSON output and rejects devices
// without a serial number and virtual devices.
func decodeSmartctlOutput(output []byte) (*smart.SmartctlOutput, error) {
	var data smart.SmartctlOutput
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("invalid smartctl JSON: %w", err)
	}
	if data.SerialNumber == "" {
		return nil, errors.New("no serial number")
	}
	if isVirtualDevice(&data) {
		return nil, fmt.Errorf("virtual device %q", data.ModelName)
	}
	return &data, nil
}

// newSmartData fills the fields shared by all smartctl protocols. deviceType is
// the exact type used to identify and query the device; when set, it takes
// precedence over the generic type reported by smartctl.
func newSmartData(data *smart.SmartctlOutput, deviceType, model, firmware string, temperature int64) *smart.SmartData {
	smartData := &smart.SmartData{
		ModelName:       model,
		SerialNumber:    data.SerialNumber,
		FirmwareVersion: firmware,
		Capacity:        data.UserCapacity.Bytes,
		Temperature:     uint8(min(max(temperature, 0), 255)),
		SmartStatus:     getSmartStatus(data.SmartStatus.Passed),
		DiskName:        data.Device.Name,
		DiskType:        data.Device.Type,
	}
	if deviceType != "" {
		smartData.DiskType = deviceType
	}
	return smartData
}

// getSmartStatus maps smartctl's health check result. A missing result is
// UNKNOWN rather than FAILED, so an unreadable status never raises an alert.
func getSmartStatus(passed *bool) string {
	switch {
	case passed == nil:
		return "UNKNOWN"
	case *passed:
		return "PASSED"
	default:
		return "FAILED"
	}
}

func ataAttributes(table []smart.AtaSmartAttribute) []*smart.SmartAttribute {
	attributes := make([]*smart.SmartAttribute, 0, len(table))
	for _, attr := range table {
		attributes = append(attributes, &smart.SmartAttribute{
			ID:         attr.ID,
			Name:       attr.Name,
			Value:      attr.Value,
			Worst:      attr.Worst,
			Threshold:  attr.Thresh,
			RawValue:   uint64(attr.Raw.Value), // already parsed from raw.string when possible
			RawString:  attr.Raw.String,
			WhenFailed: attr.WhenFailed,
		})
	}
	return attributes
}

// findAtaDeviceStatisticsEntry centralizes ATA devstat lookups so additional
// metrics can be pulled from the same structure in the future.
func findAtaDeviceStatisticsValue(data *smart.SmartctlOutput, ataDeviceStats *smart.AtaDeviceStatistics, entryNumber uint8, entryName string, minValue, maxValue int64) *int64 {
	if len(ataDeviceStats.Pages) == 0 {
		if len(data.AtaDeviceStatistics) == 0 {
			return nil
		}
		if err := json.Unmarshal(data.AtaDeviceStatistics, ataDeviceStats); err != nil {
			return nil
		}
	}
	for pageIdx := range ataDeviceStats.Pages {
		page := &ataDeviceStats.Pages[pageIdx]
		if page.Number != entryNumber {
			continue
		}
		for entryIdx := range page.Table {
			entry := &page.Table[entryIdx]
			if !strings.EqualFold(entry.Name, entryName) {
				continue
			}
			if entry.Value == nil || *entry.Value < minValue || *entry.Value > maxValue {
				return nil
			}
			return entry.Value
		}
	}
	return nil
}

func scsiAttributes(data *smart.SmartctlOutput) []*smart.SmartAttribute {
	cycles := data.ScsiStartStopCycleCounter
	attributes := []*smart.SmartAttribute{
		{Name: "PowerOnHours", RawValue: data.PowerOnTime.Hours},
		{Name: "PowerOnMinutes", RawValue: data.PowerOnTime.Minutes},
		{Name: "GrownDefectList", RawValue: data.ScsiGrownDefectList},
		{Name: "StartStopCycles", RawValue: cycles.AccumulatedStartStopCycles},
		{Name: "LoadUnloadCycles", RawValue: cycles.AccumulatedLoadUnloadCycles},
		{Name: "StartStopSpecified", RawValue: cycles.SpecifiedCycleCountOverDeviceLifetime},
		{Name: "LoadUnloadSpecified", RawValue: cycles.SpecifiedLoadUnloadCountOverDeviceLifetime},
	}

	var errorLog smart.ScsiErrorCounterLog
	if data.ScsiErrorCounterLog != nil {
		errorLog = *data.ScsiErrorCounterLog
	}
	for _, counter := range []struct {
		prefix string
		stats  smart.ScsiErrorCounter
	}{{"Read", errorLog.Read}, {"Write", errorLog.Write}, {"Verify", errorLog.Verify}} {
		attributes = append(attributes,
			&smart.SmartAttribute{Name: counter.prefix + "TotalErrorsCorrected", RawValue: counter.stats.TotalErrorsCorrected},
			&smart.SmartAttribute{Name: counter.prefix + "TotalUncorrectedErrors", RawValue: counter.stats.TotalUncorrectedErrors},
			&smart.SmartAttribute{Name: counter.prefix + "CorrectionAlgorithmInvocations", RawValue: counter.stats.CorrectionAlgorithmInvocations},
		)
		if val := parseScsiGigabytesProcessed(counter.stats.GigabytesProcessed); val >= 0 {
			attributes = append(attributes, &smart.SmartAttribute{Name: counter.prefix + "GigabytesProcessed", RawValue: uint64(val)})
		}
	}
	return attributes
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

// nvmeAttributes maps the NVMe health log, which does not follow the ATA
// attribute format, to SmartAttributes.
func nvmeAttributes(log *smart.NVMeSmartHealthInformationLog, temperature uint8) []*smart.SmartAttribute {
	return []*smart.SmartAttribute{
		{Name: "CriticalWarning", RawValue: log.CriticalWarning},
		{Name: "Temperature", RawValue: uint64(temperature)},
		{Name: "AvailableSpare", RawValue: log.AvailableSpare},
		{Name: "AvailableSpareThreshold", RawValue: log.AvailableSpareThreshold},
		{Name: "PercentageUsed", RawValue: log.PercentageUsed},
		{Name: "DataUnitsRead", RawValue: log.DataUnitsRead},
		{Name: "DataUnitsWritten", RawValue: log.DataUnitsWritten},
		{Name: "HostReads", RawValue: log.HostReads},
		{Name: "HostWrites", RawValue: log.HostWrites},
		{Name: "ControllerBusyTime", RawValue: log.ControllerBusyTime},
		{Name: "PowerCycles", RawValue: log.PowerCycles},
		{Name: "PowerOnHours", RawValue: log.PowerOnHours},
		{Name: "UnsafeShutdowns", RawValue: log.UnsafeShutdowns},
		{Name: "MediaErrors", RawValue: log.MediaErrors},
		{Name: "NumErrLogEntries", RawValue: log.NumErrLogEntries},
		{Name: "WarningTempTime", RawValue: log.WarningTempTime},
		{Name: "CriticalCompTime", RawValue: log.CriticalCompTime},
	}
}

// lookupDarwinNvmeCapacity returns the capacity in bytes for a given NVMe serial number on Darwin.
// It uses system_profiler SPNVMeDataType to get capacity since Apple SSDs don't report user_capacity
// via smartctl. Results are cached after the first call via sync.Once.
func (sm *SmartManager) lookupDarwinNvmeCapacity(serial string) uint64 {
	sm.darwinNvmeOnce.Do(func() {
		sm.darwinNvmeCapacity = make(map[string]uint64)

		provider := sm.darwinNvmeProvider
		if provider == nil {
			provider = func() ([]byte, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return exec.CommandContext(ctx, "system_profiler", "SPNVMeDataType", "-json").Output()
			}
		}

		out, err := provider()
		if err != nil {
			slog.Debug("system_profiler NVMe lookup failed", "err", err)
			return
		}

		var result struct {
			SPNVMeDataType []struct {
				Items []struct {
					DeviceSerial string `json:"device_serial"`
					SizeInBytes  uint64 `json:"size_in_bytes"`
				} `json:"_items"`
			} `json:"SPNVMeDataType"`
		}
		if err := json.Unmarshal(out, &result); err != nil {
			slog.Debug("system_profiler NVMe parse failed", "err", err)
			return
		}

		for _, controller := range result.SPNVMeDataType {
			for _, item := range controller.Items {
				if item.DeviceSerial != "" && item.SizeInBytes > 0 {
					sm.darwinNvmeCapacity[item.DeviceSerial] = item.SizeInBytes
				}
			}
		}
	})
	return sm.darwinNvmeCapacity[serial]
}
