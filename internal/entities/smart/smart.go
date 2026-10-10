package smart

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SmartctlOutput holds the fields we read from `smartctl -a --json` for any
// protocol. Sections that do not apply to a device are left nil or empty.
//
// Counters use wide integer types on purpose: a value that overflows its Go type
// makes json.Unmarshal fail and drops the whole device (see #2484, #2582).
type SmartctlOutput struct {
	Smartctl struct {
		ExitStatus int `json:"exit_status"`
	} `json:"smartctl"`
	Device          DeviceInfo   `json:"device"`
	ModelName       string       `json:"model_name"`
	SerialNumber    string       `json:"serial_number"`
	FirmwareVersion string       `json:"firmware_version"`
	UserCapacity    UserCapacity `json:"user_capacity"`
	SmartStatus     struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int64 `json:"current"`
	} `json:"temperature"`

	// ATA
	AtaSmartAttributes  *AtaSmartAttributes `json:"ata_smart_attributes"`
	AtaDeviceStatistics json.RawMessage     `json:"ata_device_statistics"` // decoded lazily, only when needed

	// NVMe
	NVMeTotalCapacity             uint64                         `json:"nvme_total_capacity"`
	NVMeSmartHealthInformationLog *NVMeSmartHealthInformationLog `json:"nvme_smart_health_information_log"`

	// SCSI (scsi_vendor/scsi_product are also reported for SATA disks behind SAT)
	ScsiVendor                string                    `json:"scsi_vendor"`
	ScsiProduct               string                    `json:"scsi_product"`
	ScsiModelName             string                    `json:"scsi_model_name"`
	ScsiRevision              string                    `json:"scsi_revision"`
	PowerOnTime               PowerOnTimeScsi           `json:"power_on_time"`
	ScsiStartStopCycleCounter ScsiStartStopCycleCounter `json:"scsi_start_stop_cycle_counter"`
	ScsiGrownDefectList       uint64                    `json:"scsi_grown_defect_list"`
	ScsiErrorCounterLog       *ScsiErrorCounterLog      `json:"scsi_error_counter_log"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	InfoName string `json:"info_name"`
	Type     string `json:"type"`
	Protocol string `json:"protocol"`
}

type UserCapacity struct {
	Blocks uint64 `json:"blocks"`
	Bytes  uint64 `json:"bytes"`
}

type AtaSmartAttributes struct {
	Table []AtaSmartAttribute `json:"table"`
}

type AtaSmartAttribute struct {
	ID         uint16   `json:"id"`
	Name       string   `json:"name"`
	Value      uint16   `json:"value"`
	Worst      uint16   `json:"worst"`
	Thresh     uint16   `json:"thresh"`
	WhenFailed string   `json:"when_failed"`
	Raw        RawValue `json:"raw"`
}

type AtaDeviceStatistics struct {
	Pages []AtaDeviceStatisticsPage `json:"pages"`
}

type AtaDeviceStatisticsPage struct {
	Number uint8                      `json:"number"`
	Table  []AtaDeviceStatisticsEntry `json:"table"`
}

type AtaDeviceStatisticsEntry struct {
	Name  string `json:"name"`
	Value *int64 `json:"value,omitempty"`
}

type NVMeSmartHealthInformationLog struct {
	CriticalWarning         uint64 `json:"critical_warning"`
	Temperature             int64  `json:"temperature"`
	AvailableSpare          uint64 `json:"available_spare"`
	AvailableSpareThreshold uint64 `json:"available_spare_threshold"`
	PercentageUsed          uint64 `json:"percentage_used"`
	DataUnitsRead           uint64 `json:"data_units_read"`
	DataUnitsWritten        uint64 `json:"data_units_written"`
	HostReads               uint64 `json:"host_reads"`
	HostWrites              uint64 `json:"host_writes"`
	ControllerBusyTime      uint64 `json:"controller_busy_time"`
	PowerCycles             uint64 `json:"power_cycles"`
	PowerOnHours            uint64 `json:"power_on_hours"`
	UnsafeShutdowns         uint64 `json:"unsafe_shutdowns"`
	MediaErrors             uint64 `json:"media_errors"`
	NumErrLogEntries        uint64 `json:"num_err_log_entries"`
	WarningTempTime         uint64 `json:"warning_temp_time"`
	CriticalCompTime        uint64 `json:"critical_comp_time"`
}

type ScsiErrorCounter struct {
	TotalErrorsCorrected           uint64 `json:"total_errors_corrected"`
	CorrectionAlgorithmInvocations uint64 `json:"correction_algorithm_invocations"`
	GigabytesProcessed             string `json:"gigabytes_processed"`
	TotalUncorrectedErrors         uint64 `json:"total_uncorrected_errors"`
}

type ScsiErrorCounterLog struct {
	Read   ScsiErrorCounter `json:"read"`
	Write  ScsiErrorCounter `json:"write"`
	Verify ScsiErrorCounter `json:"verify"`
}

type ScsiStartStopCycleCounter struct {
	SpecifiedCycleCountOverDeviceLifetime      uint64 `json:"specified_cycle_count_over_device_lifetime"`
	AccumulatedStartStopCycles                 uint64 `json:"accumulated_start_stop_cycles"`
	SpecifiedLoadUnloadCountOverDeviceLifetime uint64 `json:"specified_load_unload_count_over_device_lifetime"`
	AccumulatedLoadUnloadCycles                uint64 `json:"accumulated_load_unload_cycles"`
}

type PowerOnTimeScsi struct {
	Hours   uint64 `json:"hours"`
	Minutes uint64 `json:"minutes"`
}

type RawValue struct {
	Value  SmartRawValue `json:"value"`
	String string        `json:"string"`
}

func (r *RawValue) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Value  json.RawMessage `json:"value"`
		String string          `json:"string"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	if len(tmp.Value) > 0 {
		if err := r.Value.UnmarshalJSON(tmp.Value); err != nil {
			return err
		}
	} else {
		r.Value = 0
	}

	r.String = tmp.String

	if parsed, ok := ParseSmartRawValueString(tmp.String); ok {
		r.Value = SmartRawValue(parsed)
	}

	return nil
}

type SmartRawValue uint64

// handles when drives report strings like "0h+0m+0.000s" or "7344 (253d 8h)" for power on hours
func (v *SmartRawValue) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 || trimmed == "null" {
		*v = 0
		return nil
	}

	if trimmed[0] == '"' {
		valueStr, err := strconv.Unquote(trimmed)
		if err != nil {
			return err
		}
		parsed, ok := ParseSmartRawValueString(valueStr)
		if ok {
			*v = SmartRawValue(parsed)
			return nil
		}
		*v = 0
		return nil
	}

	if parsed, err := strconv.ParseUint(trimmed, 0, 64); err == nil {
		*v = SmartRawValue(parsed)
		return nil
	}

	if parsed, ok := ParseSmartRawValueString(trimmed); ok {
		*v = SmartRawValue(parsed)
		return nil
	}

	*v = 0
	return nil
}

// ParseSmartRawValueString attempts to extract a numeric value from the raw value
// strings emitted by smartctl, which sometimes include human-friendly annotations
// like "7344 (253d 8h)" or "0h+0m+0.000s". It returns the parsed value and a
// boolean indicating success.
func ParseSmartRawValueString(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if parsed, err := strconv.ParseUint(value, 0, 64); err == nil {
		return parsed, true
	}

	if idx := strings.IndexRune(value, 'h'); idx > 0 {
		hoursPart := strings.TrimSpace(value[:idx])
		if hoursPart != "" {
			if parsed, err := strconv.ParseFloat(hoursPart, 64); err == nil {
				return uint64(parsed), true
			}
		}
	}

	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			continue
		}
		end := i + 1
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		digits := value[i:end]
		if parsed, err := strconv.ParseUint(digits, 10, 64); err == nil {
			return parsed, true
		}
		i = end
	}

	return 0, false
}

type SmartData struct {
	// ModelFamily     string            `json:"mf,omitempty" cbor:"0,keyasint,omitempty"`
	ModelName       string            `json:"mn,omitempty" cbor:"1,keyasint,omitempty"`
	SerialNumber    string            `json:"sn,omitempty" cbor:"2,keyasint,omitempty"`
	FirmwareVersion string            `json:"fv,omitempty" cbor:"3,keyasint,omitempty"`
	Capacity        uint64            `json:"c,omitempty" cbor:"4,keyasint,omitempty"`
	SmartStatus     string            `json:"s,omitempty" cbor:"5,keyasint,omitempty"`
	DiskName        string            `json:"dn,omitempty" cbor:"6,keyasint,omitempty"`
	DiskType        string            `json:"dt,omitempty" cbor:"7,keyasint,omitempty"`
	Temperature     uint8             `json:"t,omitempty" cbor:"8,keyasint,omitempty"`
	Attributes      []*SmartAttribute `json:"a,omitempty" cbor:"9,keyasint,omitempty"`
}

// SmartDataResponse contains the collected data and whether every discovered
// device was collected. Older agents omit Complete, so hubs must not prune from it.
type SmartDataResponse struct {
	Data     map[string]SmartData `json:"data" cbor:"0,keyasint"`
	Complete bool                 `json:"complete" cbor:"1,keyasint,omitempty"` // Whether every discovered device was collected
}

type SmartAttribute struct {
	ID         uint16 `json:"id,omitempty" cbor:"0,keyasint,omitempty"`
	Name       string `json:"n" cbor:"1,keyasint"`
	Value      uint16 `json:"v,omitempty" cbor:"2,keyasint,omitempty"`
	Worst      uint16 `json:"w,omitempty" cbor:"3,keyasint,omitempty"`
	Threshold  uint16 `json:"t,omitempty" cbor:"4,keyasint,omitempty"`
	RawValue   uint64 `json:"rv" cbor:"5,keyasint"`
	RawString  string `json:"rs,omitempty" cbor:"6,keyasint,omitempty"`
	WhenFailed string `json:"wf,omitempty" cbor:"7,keyasint,omitempty"`
}
