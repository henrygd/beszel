package smart

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Common types
type VersionInfo [2]int

type SmartctlInfo struct {
	Version      VersionInfo `json:"version"`
	SvnRevision  string      `json:"svn_revision"`
	PlatformInfo string      `json:"platform_info"`
	BuildInfo    string      `json:"build_info"`
	Argv         []string    `json:"argv"`
	ExitStatus   int         `json:"exit_status"`
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

// type LocalTime struct {
// 	TimeT   int64  `json:"time_t"`
// 	Asctime string `json:"asctime"`
// }

// type WwnInfo struct {
// 	Naa int `json:"naa"`
// 	Oui int `json:"oui"`
// 	ID  int `json:"id"`
// }

// type FormFactorInfo struct {
// 	AtaValue int    `json:"ata_value"`
// 	Name     string `json:"name"`
// }

// type TrimInfo struct {
// 	Supported bool `json:"supported"`
// }

// type AtaVersionInfo struct {
// 	String     string `json:"string"`
// 	MajorValue int    `json:"major_value"`
// 	MinorValue int    `json:"minor_value"`
// }

// type VersionStringInfo struct {
// 	String string `json:"string"`
// 	Value  int    `json:"value"`
// }

// type SpeedInfo struct {
// 	SataValue      int    `json:"sata_value"`
// 	String         string `json:"string"`
// 	UnitsPerSecond int    `json:"units_per_second"`
// 	BitsPerUnit    int    `json:"bits_per_unit"`
// }

// type InterfaceSpeedInfo struct {
// 	Max     SpeedInfo `json:"max"`
// 	Current SpeedInfo `json:"current"`
// }

type SmartStatusInfo struct {
	Passed bool `json:"passed"`
}

type StatusInfo struct {
	Value  int    `json:"value"`
	String string `json:"string"`
	Passed bool   `json:"passed"`
}

type PollingMinutes struct {
	Short    int `json:"short"`
	Extended int `json:"extended"`
}

type CapabilitiesInfo struct {
	Values                        []int `json:"values"`
	ExecOfflineImmediateSupported bool  `json:"exec_offline_immediate_supported"`
	OfflineIsAbortedUponNewCmd    bool  `json:"offline_is_aborted_upon_new_cmd"`
	OfflineSurfaceScanSupported   bool  `json:"offline_surface_scan_supported"`
	SelfTestsSupported            bool  `json:"self_tests_supported"`
	ConveyanceSelfTestSupported   bool  `json:"conveyance_self_test_supported"`
	SelectiveSelfTestSupported    bool  `json:"selective_self_test_supported"`
	AttributeAutosaveEnabled      bool  `json:"attribute_autosave_enabled"`
	ErrorLoggingSupported         bool  `json:"error_logging_supported"`
	GpLoggingSupported            bool  `json:"gp_logging_supported"`
}

// type AtaSmartData struct {
// 	OfflineDataCollection OfflineDataCollectionInfo `json:"offline_data_collection"`
// 	SelfTest              SelfTestInfo              `json:"self_test"`
// 	Capabilities          CapabilitiesInfo          `json:"capabilities"`
// }

// type OfflineDataCollectionInfo struct {
// 	Status            StatusInfo `json:"status"`
// 	CompletionSeconds int        `json:"completion_seconds"`
// }

// type SelfTestInfo struct {
// 	Status         StatusInfo     `json:"status"`
// 	PollingMinutes PollingMinutes `json:"polling_minutes"`
// }

// type AtaSctCapabilities struct {
// 	Value                         int  `json:"value"`
// 	ErrorRecoveryControlSupported bool `json:"error_recovery_control_supported"`
// 	FeatureControlSupported       bool `json:"feature_control_supported"`
// 	DataTableSupported            bool `json:"data_table_supported"`
// }

type SummaryInfo struct {
	Revision int `json:"revision"`
	Count    int `json:"count"`
}

type AtaSmartAttributes struct {
	// Revision int                 `json:"revision"`
	Table []AtaSmartAttribute `json:"table"`
}

type AtaSmartAttribute struct {
	ID         uint16 `json:"id"`
	Name       string `json:"name"`
	Value      uint16 `json:"value"`
	Worst      uint16 `json:"worst"`
	Thresh     uint16 `json:"thresh"`
	WhenFailed string `json:"when_failed"`
	// Flags      AttributeFlags `json:"flags"`
	Raw RawValue `json:"raw"`
}

// type AttributeFlags struct {
// 	Value         int    `json:"value"`
// 	String        string `json:"string"`
// 	Prefailure    bool   `json:"prefailure"`
// 	UpdatedOnline bool   `json:"updated_online"`
// 	Performance   bool   `json:"performance"`
// 	ErrorRate     bool   `json:"error_rate"`
// 	EventCount    bool   `json:"event_count"`
// 	AutoKeep      bool   `json:"auto_keep"`
// }

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

// type PowerOnTimeInfo struct {
// 	Hours uint32 `json:"hours"`
// }

type TemperatureInfo struct {
	Current uint8 `json:"current"`
}

type TemperatureInfoScsi struct {
	Current   uint8 `json:"current"`
	DriveTrip uint8 `json:"drive_trip"`
}

// type SelectiveSelfTestTable struct {
// 	LbaMin int        `json:"lba_min"`
// 	LbaMax int        `json:"lba_max"`
// 	Status StatusInfo `json:"status"`
// }

// type SelectiveSelfTestFlags struct {
// 	Value                int  `json:"value"`
// 	RemainderScanEnabled bool `json:"remainder_scan_enabled"`
// }

// type AtaSmartSelectiveSelfTestLog struct {
// 	Revision                 int                      `json:"revision"`
// 	Table                    []SelectiveSelfTestTable `json:"table"`
// 	Flags                    SelectiveSelfTestFlags   `json:"flags"`
// 	PowerUpScanResumeMinutes int                      `json:"power_up_scan_resume_minutes"`
// }

// BaseSmartInfo contains common fields shared between SATA and NVMe drives
// type BaseSmartInfo struct {
// 	Device           DeviceInfo   `json:"device"`
// 	ModelName        string       `json:"model_name"`
// 	SerialNumber     string       `json:"serial_number"`
// 	FirmwareVersion  string       `json:"firmware_version"`
// 	UserCapacity     UserCapacity `json:"user_capacity"`
// 	LogicalBlockSize int          `json:"logical_block_size"`
// 	LocalTime        LocalTime    `json:"local_time"`
// }

type SmartctlInfoLegacy struct {
	Version      VersionInfo `json:"version"`
	SvnRevision  string      `json:"svn_revision"`
	PlatformInfo string      `json:"platform_info"`
	BuildInfo    string      `json:"build_info"`
	Argv         []string    `json:"argv"`
	ExitStatus   int         `json:"exit_status"`
}

type SmartInfoForSata struct {
	// JSONFormatVersion VersionInfo        `json:"json_format_version"`
	Smartctl SmartctlInfoLegacy `json:"smartctl"`
	Device   DeviceInfo         `json:"device"`
	// ModelFamily  string             `json:"model_family"`
	ModelName    string `json:"model_name"`
	SerialNumber string `json:"serial_number"`
	// Wwn               WwnInfo            `json:"wwn"`
	FirmwareVersion string       `json:"firmware_version"`
	UserCapacity    UserCapacity `json:"user_capacity"`
	ScsiVendor      string       `json:"scsi_vendor"`
	ScsiProduct     string       `json:"scsi_product"`
	// LogicalBlockSize  int                `json:"logical_block_size"`
	// PhysicalBlockSize int                `json:"physical_block_size"`
	// RotationRate      int                `json:"rotation_rate"`
	// FormFactor        FormFactorInfo     `json:"form_factor"`
	// Trim                         TrimInfo                     `json:"trim"`
	// InSmartctlDatabase           bool                         `json:"in_smartctl_database"`
	// AtaVersion                   AtaVersionInfo               `json:"ata_version"`
	// SataVersion                  VersionStringInfo            `json:"sata_version"`
	// InterfaceSpeed               InterfaceSpeedInfo           `json:"interface_speed"`
	// LocalTime                    LocalTime                    `json:"local_time"`
	SmartStatus SmartStatusInfo `json:"smart_status"`
	// AtaSmartData                 AtaSmartData                 `json:"ata_smart_data"`
	// AtaSctCapabilities           AtaSctCapabilities           `json:"ata_sct_capabilities"`
	AtaSmartAttributes AtaSmartAttributes `json:"ata_smart_attributes"`
	// PowerOnTime                  PowerOnTimeInfo              `json:"power_on_time"`
	// PowerCycleCount              uint16                       `json:"power_cycle_count"`
	Temperature TemperatureInfo `json:"temperature"`
	// AtaSmartErrorLog             AtaSmartErrorLog             `json:"ata_smart_error_log"`
	// AtaSmartSelfTestLog          AtaSmartSelfTestLog          `json:"ata_smart_self_test_log"`
	// AtaSmartSelectiveSelfTestLog AtaSmartSelectiveSelfTestLog `json:"ata_smart_selective_self_test_log"`
}

type ScsiErrorCounter struct {
	ErrorsCorrectedByECCFast         uint64 `json:"errors_corrected_by_eccfast"`
	ErrorsCorrectedByECCDelayed      uint64 `json:"errors_corrected_by_eccdelayed"`
	ErrorsCorrectedByRereadsRewrites uint64 `json:"errors_corrected_by_rereads_rewrites"`
	TotalErrorsCorrected             uint64 `json:"total_errors_corrected"`
	CorrectionAlgorithmInvocations   uint64 `json:"correction_algorithm_invocations"`
	GigabytesProcessed               string `json:"gigabytes_processed"`
	TotalUncorrectedErrors           uint64 `json:"total_uncorrected_errors"`
}

type ScsiErrorCounterLog struct {
	Read   ScsiErrorCounter `json:"read"`
	Write  ScsiErrorCounter `json:"write"`
	Verify ScsiErrorCounter `json:"verify"`
}

type ScsiStartStopCycleCounter struct {
	YearOfManufacture                          string `json:"year_of_manufacture"`
	WeekOfManufacture                          string `json:"week_of_manufacture"`
	SpecifiedCycleCountOverDeviceLifetime      uint64 `json:"specified_cycle_count_over_device_lifetime"`
	AccumulatedStartStopCycles                 uint64 `json:"accumulated_start_stop_cycles"`
	SpecifiedLoadUnloadCountOverDeviceLifetime uint64 `json:"specified_load_unload_count_over_device_lifetime"`
	AccumulatedLoadUnloadCycles                uint64 `json:"accumulated_load_unload_cycles"`
}

type PowerOnTimeScsi struct {
	Hours   uint64 `json:"hours"`
	Minutes uint64 `json:"minutes"`
}

type SmartInfoForScsi struct {
	Smartctl                  SmartctlInfoLegacy        `json:"smartctl"`
	Device                    DeviceInfo                `json:"device"`
	ScsiVendor                string                    `json:"scsi_vendor"`
	ScsiProduct               string                    `json:"scsi_product"`
	ScsiModelName             string                    `json:"scsi_model_name"`
	ScsiRevision              string                    `json:"scsi_revision"`
	ScsiVersion               string                    `json:"scsi_version"`
	SerialNumber              string                    `json:"serial_number"`
	UserCapacity              UserCapacity              `json:"user_capacity"`
	Temperature               TemperatureInfoScsi       `json:"temperature"`
	SmartStatus               SmartStatusInfo           `json:"smart_status"`
	PowerOnTime               PowerOnTimeScsi           `json:"power_on_time"`
	ScsiStartStopCycleCounter ScsiStartStopCycleCounter `json:"scsi_start_stop_cycle_counter"`
	ScsiGrownDefectList       uint64                    `json:"scsi_grown_defect_list"`
	ScsiErrorCounterLog       ScsiErrorCounterLog       `json:"scsi_error_counter_log"`
}

// type AtaSmartErrorLog struct {
// 	Summary SummaryInfo `json:"summary"`
// }

// type AtaSmartSelfTestLog struct {
// 	Standard SummaryInfo `json:"standard"`
// }

type SmartctlInfoNvme struct {
	Version      VersionInfo `json:"version"`
	SVNRevision  string      `json:"svn_revision"`
	PlatformInfo string      `json:"platform_info"`
	BuildInfo    string      `json:"build_info"`
	Argv         []string    `json:"argv"`
	ExitStatus   int         `json:"exit_status"`
}

// type NVMePCIVendor struct {
// 	ID          int `json:"id"`
// 	SubsystemID int `json:"subsystem_id"`
// }

// type SizeCapacityInfo struct {
// 	Blocks uint64 `json:"blocks"`
// 	Bytes  uint64 `json:"bytes"`
// }

// type EUI64Info struct {
// 	OUI   int `json:"oui"`
// 	ExtID int `json:"ext_id"`
// }

// type NVMeNamespace struct {
// 	ID               uint32           `json:"id"`
// 	Size             SizeCapacityInfo `json:"size"`
// 	Capacity         SizeCapacityInfo `json:"capacity"`
// 	Utilization      SizeCapacityInfo `json:"utilization"`
// 	FormattedLBASize uint32           `json:"formatted_lba_size"`
// 	EUI64            EUI64Info        `json:"eui64"`
// }

type SmartStatusInfoNvme struct {
	Passed bool            `json:"passed"`
	NVMe   SmartStatusNVMe `json:"nvme"`
}

type SmartStatusNVMe struct {
	Value int `json:"value"`
}

type NVMeSmartHealthInformationLog struct {
	CriticalWarning         uint    `json:"critical_warning"`
	Temperature             uint8   `json:"temperature"`
	AvailableSpare          uint    `json:"available_spare"`
	AvailableSpareThreshold uint    `json:"available_spare_threshold"`
	PercentageUsed          uint8   `json:"percentage_used"`
	DataUnitsRead           uint64  `json:"data_units_read"`
	DataUnitsWritten        uint64  `json:"data_units_written"`
	HostReads               uint    `json:"host_reads"`
	HostWrites              uint    `json:"host_writes"`
	ControllerBusyTime      uint    `json:"controller_busy_time"`
	PowerCycles             uint16  `json:"power_cycles"`
	PowerOnHours            uint32  `json:"power_on_hours"`
	UnsafeShutdowns         uint16  `json:"unsafe_shutdowns"`
	MediaErrors             uint    `json:"media_errors"`
	NumErrLogEntries        uint    `json:"num_err_log_entries"`
	WarningTempTime         uint    `json:"warning_temp_time"`
	CriticalCompTime        uint    `json:"critical_comp_time"`
	TemperatureSensors      []uint8 `json:"temperature_sensors"`
}

type SmartInfoForNvme struct {
	// JSONFormatVersion             VersionInfo                   `json:"json_format_version"`
	Smartctl        SmartctlInfoNvme `json:"smartctl"`
	Device          DeviceInfo       `json:"device"`
	ModelName       string           `json:"model_name"`
	SerialNumber    string           `json:"serial_number"`
	FirmwareVersion string           `json:"firmware_version"`
	// NVMePCIVendor                 NVMePCIVendor                 `json:"nvme_pci_vendor"`
	// NVMeIEEEOUIIdentifier         uint32                        `json:"nvme_ieee_oui_identifier"`
	// NVMeTotalCapacity             uint64                        `json:"nvme_total_capacity"`
	// NVMeUnallocatedCapacity       uint64                        `json:"nvme_unallocated_capacity"`
	// NVMeControllerID              uint16                        `json:"nvme_controller_id"`
	// NVMeVersion                   VersionStringInfo             `json:"nvme_version"`
	// NVMeNumberOfNamespaces        uint8                         `json:"nvme_number_of_namespaces"`
	// NVMeNamespaces                []NVMeNamespace               `json:"nvme_namespaces"`
	UserCapacity UserCapacity `json:"user_capacity"`
	// LogicalBlockSize              int                           `json:"logical_block_size"`
	// LocalTime                     LocalTime                     `json:"local_time"`
	SmartStatus                   SmartStatusInfoNvme           `json:"smart_status"`
	NVMeSmartHealthInformationLog NVMeSmartHealthInformationLog `json:"nvme_smart_health_information_log"`
	Temperature                   TemperatureInfoNvme           `json:"temperature"`
	PowerCycleCount               uint16                        `json:"power_cycle_count"`
	PowerOnTime                   PowerOnTimeInfoNvme           `json:"power_on_time"`
}

type TemperatureInfoNvme struct {
	Current int `json:"current"`
}

type PowerOnTimeInfoNvme struct {
	Hours int `json:"hours"`
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
