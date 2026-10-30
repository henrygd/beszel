//go:build testing

package agent

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readSmartFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("test-data", "smart", name))
	require.NoError(t, err)
	return data
}

// parseSmart runs parseSmartctl and requires it to succeed.
func parseSmart(t *testing.T, output []byte, dtype string) *smart.SmartData {
	t.Helper()
	sm := &SmartManager{}
	data, _, err := sm.parseSmartctl(output, dtype)
	require.NoError(t, err)
	return data
}

func TestParseSmartctlScsi(t *testing.T) {
	deviceData := parseSmart(t, readSmartFixture(t, "scsi.json"), "")

	assert.Equal(t, "YADRO WUH721414AL4204", deviceData.ModelName)
	assert.Equal(t, "9YHSDH9B", deviceData.SerialNumber)
	assert.Equal(t, "C240", deviceData.FirmwareVersion)
	assert.Equal(t, "/dev/sde", deviceData.DiskName)
	assert.Equal(t, "scsi", deviceData.DiskType)
	assert.EqualValues(t, 34, deviceData.Temperature)
	assert.Equal(t, "PASSED", deviceData.SmartStatus)
	assert.EqualValues(t, 14000519643136, deviceData.Capacity)

	assertAttrValue(t, deviceData.Attributes, "PowerOnHours", 458)
	assertAttrValue(t, deviceData.Attributes, "PowerOnMinutes", 25)
	assertAttrValue(t, deviceData.Attributes, "GrownDefectList", 0)
	assertAttrValue(t, deviceData.Attributes, "StartStopCycles", 2)
	assertAttrValue(t, deviceData.Attributes, "LoadUnloadCycles", 418)
	assertAttrValue(t, deviceData.Attributes, "StartStopSpecified", 50000)
	assertAttrValue(t, deviceData.Attributes, "LoadUnloadSpecified", 600000)
	assertAttrValue(t, deviceData.Attributes, "ReadCorrectionAlgorithmInvocations", 346)
	assertAttrValue(t, deviceData.Attributes, "WriteCorrectionAlgorithmInvocations", 4052)
	assertAttrValue(t, deviceData.Attributes, "VerifyCorrectionAlgorithmInvocations", 223)
	assertAttrValue(t, deviceData.Attributes, "ReadGigabytesProcessed", 3641)
	assertAttrValue(t, deviceData.Attributes, "WriteGigabytesProcessed", 2124590)
	assertAttrValue(t, deviceData.Attributes, "VerifyGigabytesProcessed", 0)
}

func TestParseSmartctlSata(t *testing.T) {
	sm := &SmartManager{}
	deviceData, protocol, err := sm.parseSmartctl(readSmartFixture(t, "sda.json"), "")
	require.NoError(t, err)
	assert.Equal(t, "ata", protocol)

	assert.Equal(t, "9C40918040082", deviceData.SerialNumber)
	assert.Equal(t, "P3-2TB", deviceData.ModelName)
	assert.Equal(t, "X0104A0", deviceData.FirmwareVersion)
	assert.Equal(t, "/dev/sda", deviceData.DiskName)
	assert.Equal(t, "sat", deviceData.DiskType)
	assert.Equal(t, uint8(31), deviceData.Temperature)
	assert.Equal(t, "PASSED", deviceData.SmartStatus)
	assert.Equal(t, uint64(2048408248320), deviceData.Capacity)
	assertAttrValue(t, deviceData.Attributes, "Temperature_Celsius", 31)
}

func TestParseSmartctlNvme(t *testing.T) {
	deviceData := parseSmart(t, readSmartFixture(t, "nvme0.json"), "")

	assert.Equal(t, "2024031600129", deviceData.SerialNumber)
	assert.Equal(t, "PELADN 512GB", deviceData.ModelName)
	assert.Equal(t, "VC2S038E", deviceData.FirmwareVersion)
	assert.Equal(t, "/dev/nvme0", deviceData.DiskName)
	assert.Equal(t, "nvme", deviceData.DiskType)
	assert.Equal(t, uint8(61), deviceData.Temperature)
	assert.Equal(t, "PASSED", deviceData.SmartStatus)
	assert.Equal(t, uint64(512110190592), deviceData.Capacity)
	assertAttrValue(t, deviceData.Attributes, "PercentageUsed", 0)
	assertAttrValue(t, deviceData.Attributes, "DataUnitsWritten", 16040567)
}

// The -d type used for the query is reported as the disk type, so RAID and
// bridge types stay visible in the hub.
func TestParseSmartctlReportsQueryType(t *testing.T) {
	assert.Equal(t, "sat+megaraid", parseSmart(t, readSmartFixture(t, "sda.json"), "sat+megaraid").DiskType)
	assert.Equal(t, "ata", parseSmart(t, readSmartFixture(t, "sda.json"), "ata").DiskType)
}

func TestParseSmartctlStatus(t *testing.T) {
	for _, test := range []struct {
		name, status string
		want         string
	}{
		{"passed", `,"smart_status":{"passed":true}`, "PASSED"},
		{"failed", `,"smart_status":{"passed":false}`, "FAILED"},
		// A missing status must not read as a failure, even with a temperature.
		{"missing", "", "UNKNOWN"},
		{"null", `,"smart_status":{"passed":null}`, "UNKNOWN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := fmt.Sprintf(`{
				"device": {"name": "/dev/sda", "type": "sat", "protocol": "ATA"},
				"serial_number": "STATUS",
				"temperature": {"current": 30},
				"ata_smart_attributes": {"table": [{"id": 197, "raw": {"value": 1, "string": "1"}}]}
				%s
			}`, test.status)
			assert.Equal(t, test.want, parseSmart(t, []byte(output), "").SmartStatus)
		})
	}
}

// Output that identifies a disk but holds no health data for its protocol is
// rejected so the next attempt runs, instead of storing zeros (#2295, #1504).
func TestParseSmartctlRequiresHealthData(t *testing.T) {
	for _, test := range []struct {
		name, protocol, health string
		valid                  bool
	}{
		{"scsi failed status", "SCSI", `,"smart_status":{"passed":false}`, true},
		{"scsi passed status", "SCSI", `,"smart_status":{"passed":true}`, true},
		{"scsi error log", "SCSI", `,"scsi_error_counter_log":{"read":{}}`, true},
		{"scsi null status", "SCSI", `,"smart_status":null`, false},
		{"scsi nothing", "SCSI", "", false},
		{"nvme zero log", "NVMe", `,"nvme_smart_health_information_log":{"temperature":0}`, true},
		{"nvme null log", "NVMe", `,"nvme_smart_health_information_log":null`, false},
		{"nvme nothing", "NVMe", "", false},
		{"ata attributes", "ATA", `,"ata_smart_attributes":{"table":[{"id":5}]}`, true},
		{"ata status only", "ATA", `,"smart_status":{"passed":true}`, true},
		{"ata devstat only", "ATA", `,"ata_device_statistics":{"pages":[]}`, true},
		{"ata empty table", "ATA", `,"ata_smart_attributes":{"table":[]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := fmt.Sprintf(`{"device":{"name":"/dev/sda","protocol":%q},"serial_number":"HEALTH"%s}`, test.protocol, test.health)
			_, _, err := (&SmartManager{}).parseSmartctl([]byte(output), "")
			if test.valid {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, errNoHealthData)
			}
		})
	}

	_, _, err := (&SmartManager{}).parseSmartctl(readSmartFixture(t, "scsi_identity.json"), "")
	assert.ErrorIs(t, err, errNoHealthData)

	// An NVMe controller can return identity fields but fail the health log.
	_, _, err = (&SmartManager{}).parseSmartctl([]byte(`{
		"smartctl": {"exit_status": 2},
		"device": {"name": "/dev/nvme0", "type": "nvme"},
		"model_name": "Netac NVMe SSD 500GB",
		"serial_number": "IDENTITY-ONLY",
		"user_capacity": {"bytes": 500107862016}
	}`), "")
	assert.ErrorIs(t, err, errNoHealthData)

	// Real smartctl output when the agent lacks CAP_SYS_ADMIN for NVMe.
	_, _, err = (&SmartManager{}).parseSmartctl(readSmartFixture(t, "nvme_permission_denied.json"), "nvme")
	assert.ErrorIs(t, err, errNoHealthData)
}

func TestParseSmartctlRejectsInvalidOutput(t *testing.T) {
	for name, output := range map[string]string{
		"not json":  "not json",
		"no serial": `{"device":{"name":"/dev/sda"},"smart_status":{"passed":true}}`,
		"virtual":   `{"serial_number":"V1","model_name":"QEMU HARDDISK","smart_status":{"passed":true}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := (&SmartManager{}).parseSmartctl([]byte(output), "")
			assert.Error(t, err)
		})
	}
}

// Without device.protocol (older smartctl, hand-written JSON) the protocol is
// inferred from the sections present.
func TestSmartProtocolInference(t *testing.T) {
	for _, test := range []struct {
		data smart.SmartctlOutput
		want string
	}{
		{smart.SmartctlOutput{Device: smart.DeviceInfo{Protocol: "NVMe"}}, "nvme"},
		{smart.SmartctlOutput{Device: smart.DeviceInfo{Type: "nvme"}}, "nvme"},
		{smart.SmartctlOutput{NVMeSmartHealthInformationLog: &smart.NVMeSmartHealthInformationLog{}}, "nvme"},
		{smart.SmartctlOutput{AtaSmartAttributes: &smart.AtaSmartAttributes{}}, "ata"},
		{smart.SmartctlOutput{ScsiErrorCounterLog: &smart.ScsiErrorCounterLog{}}, "scsi"},
		{smart.SmartctlOutput{ScsiModelName: "SEAGATE"}, "scsi"},
		{smart.SmartctlOutput{}, "ata"},
	} {
		assert.Equal(t, test.want, smartProtocol(&test.data), "%+v", test.data)
	}
}

func TestParseSmartctlDeviceStatisticsTemperature(t *testing.T) {
	for _, test := range []struct {
		name    string
		current string
		want    uint8
	}{
		{"fallback when temperature missing", "", 22},
		// Negative entries (e.g. minimum operating temperature) must not fail the parse.
		{"reported temperature wins", `"temperature": {"current": 38},`, 38},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := fmt.Sprintf(`{
				"device": {"name": "/dev/sdb", "type": "sat"},
				"serial_number": "DEVSTAT123",
				"smart_status": {"passed": true},
				%s
				"ata_smart_attributes": {"table": []},
				"ata_device_statistics": {"pages": [{"number": 5, "table": [
					{"name": "Current Temperature", "value": 22},
					{"name": "Specified Minimum Operating Temperature", "value": -20}
				]}]}
			}`, test.current)
			assert.Equal(t, test.want, parseSmart(t, []byte(output), "").Temperature)
		})
	}
}

func TestParseSmartctlParentheticalRawValue(t *testing.T) {
	deviceData := parseSmart(t, []byte(`{
		"device": {"name": "/dev/sdz", "type": "sat"},
		"serial_number": "PARENTHESES123",
		"smart_status": {"passed": true},
		"ata_smart_attributes": {"table": [{
			"id": 9, "name": "Power_On_Hours", "value": 93, "worst": 55, "thresh": 0,
			"raw": {"value": 57891864217128, "string": "39925 (212 206 0)"}
		}]}
	}`), "")
	require.Len(t, deviceData.Attributes, 1)
	assert.Equal(t, uint64(39925), deviceData.Attributes[0].RawValue)
	assert.Equal(t, "39925 (212 206 0)", deviceData.Attributes[0].RawString)
}

// Counters larger than their old narrow Go types used to fail json.Unmarshal and
// drop the whole device (#2484, #2582).
func TestParseSmartctlNvmeLargeCounters(t *testing.T) {
	deviceData := parseSmart(t, []byte(`{
		"device": {"name": "/dev/nvme0", "type": "nvme"},
		"serial_number": "NVME-LARGE",
		"smart_status": {"passed": true},
		"nvme_smart_health_information_log": {
			"temperature": 40,
			"percentage_used": 300,
			"power_cycles": 70000,
			"power_on_hours": 5000000000,
			"unsafe_shutdowns": 159754
		}
	}`), "")
	assertAttrValue(t, deviceData.Attributes, "UnsafeShutdowns", 159754)
	assertAttrValue(t, deviceData.Attributes, "PowerCycles", 70000)
	assertAttrValue(t, deviceData.Attributes, "PowerOnHours", 5000000000)
	assertAttrValue(t, deviceData.Attributes, "PercentageUsed", 300)
}

// NVMe health counters are 128-bit, so smartctl can print values past uint64.
// They are capped instead of failing json.Unmarshal and dropping the device (#2731).
func TestParseSmartctlNvmeCounterBeyondUint64(t *testing.T) {
	deviceData := parseSmart(t, []byte(`{
		"device": {"name": "/dev/nvme0", "type": "nvme"},
		"serial_number": "NVME-OVERFLOW",
		"smart_status": {"passed": true},
		"nvme_smart_health_information_log": {
			"temperature": 35,
			"data_units_written": 340282366920938463463374607431768211455,
			"power_on_hours": 1234,
			"media_errors": 557325022338560884631142400
		}
	}`), "")
	// An overflowed counter is reported as invalid, not as a made-up number.
	for _, name := range []string{"MediaErrors", "DataUnitsWritten"} {
		attr := findAttr(deviceData.Attributes, name)
		require.NotNil(t, attr, name)
		assert.Zero(t, attr.RawValue, name)
		assert.Equal(t, smart.NVMeCounterInvalid, attr.RawString, name)
	}
	assertAttrValue(t, deviceData.Attributes, "PowerOnHours", 1234)
	assert.Empty(t, findAttr(deviceData.Attributes, "PowerOnHours").RawString)
	assert.Equal(t, uint8(35), deviceData.Temperature)
}

// Every 128-bit counter is reported as invalid when it overflows.
func TestParseSmartctlNvmeEveryCounterOverflow(t *testing.T) {
	fields := map[string]string{
		"data_units_read":      "DataUnitsRead",
		"data_units_written":   "DataUnitsWritten",
		"host_reads":           "HostReads",
		"host_writes":          "HostWrites",
		"controller_busy_time": "ControllerBusyTime",
		"power_cycles":         "PowerCycles",
		"power_on_hours":       "PowerOnHours",
		"unsafe_shutdowns":     "UnsafeShutdowns",
		"media_errors":         "MediaErrors",
		"num_err_log_entries":  "NumErrLogEntries",
	}
	for field, name := range fields {
		deviceData := parseSmart(t, []byte(`{
			"device": {"name": "/dev/nvme0", "type": "nvme"},
			"serial_number": "NVME-`+name+`",
			"nvme_smart_health_information_log": {"temperature": 35, "`+field+`": 18446744073709551616}
		}`), "")
		attr := findAttr(deviceData.Attributes, name)
		require.NotNil(t, attr, name)
		assert.Zero(t, attr.RawValue, name)
		assert.Equal(t, smart.NVMeCounterInvalid, attr.RawString, name)
	}
}

// Exponent-form counters and an overflowed TNVMCAP still parse the device;
// an overflowed capacity is left unknown (0) rather than made up.
func TestParseSmartctlNvmeExponentAndCapacity(t *testing.T) {
	deviceData := parseSmart(t, []byte(`{
		"device": {"name": "/dev/nvme0", "type": "nvme"},
		"serial_number": "NVME-EXP",
		"smart_status": {"passed": true},
		"nvme_total_capacity": 5.573250223385609e+26,
		"nvme_smart_health_information_log": {
			"temperature": 35,
			"power_on_hours": 5.573250223385609e+26,
			"power_cycles": 1.2e2,
			"media_errors": null
		}
	}`), "")
	assert.Zero(t, deviceData.Capacity)
	hours := findAttr(deviceData.Attributes, "PowerOnHours")
	require.NotNil(t, hours)
	assert.Zero(t, hours.RawValue)
	assert.Equal(t, smart.NVMeCounterInvalid, hours.RawString)
	assertAttrValue(t, deviceData.Attributes, "PowerCycles", 120)
	assertAttrValue(t, deviceData.Attributes, "MediaErrors", 0)

	deviceData = parseSmart(t, []byte(`{
		"device": {"name": "/dev/nvme0", "type": "nvme"},
		"serial_number": "NVME-CAP",
		"nvme_total_capacity": 1000204886016,
		"nvme_smart_health_information_log": {"temperature": 35}
	}`), "")
	assert.Equal(t, uint64(1000204886016), deviceData.Capacity)
}

// Out-of-range temperatures are clamped instead of failing the whole parse.
func TestParseSmartctlTemperatureOutOfRange(t *testing.T) {
	sata := parseSmart(t, []byte(`{
		"device": {"name": "/dev/sda", "type": "sat"},
		"serial_number": "SATA-NEG",
		"smart_status": {"passed": true},
		"temperature": {"current": -5},
		"ata_smart_attributes": {"table": []}
	}`), "")
	assert.Equal(t, uint8(0), sata.Temperature)

	nvme := parseSmart(t, []byte(`{
		"device": {"name": "/dev/nvme0", "type": "nvme"},
		"serial_number": "NVME-HOT",
		"smart_status": {"passed": true},
		"nvme_smart_health_information_log": {"temperature": 300}
	}`), "")
	assert.Equal(t, uint8(255), nvme.Temperature)
	assertAttrValue(t, nvme.Attributes, "Temperature", 255)
}

func TestParseSmartctlScsiWithoutErrorCounterLog(t *testing.T) {
	deviceData := parseSmart(t, []byte(`{
		"device": {"name": "/dev/sdb", "type": "scsi"},
		"scsi_model_name": "SEAGATE ST4000NM0023",
		"serial_number": "SCSI-NOLOG",
		"smart_status": {"passed": true},
		"temperature": {"current": 30},
		"power_on_time": {"hours": 1234}
	}`), "")
	assertAttrValue(t, deviceData.Attributes, "PowerOnHours", 1234)
	assertAttrValue(t, deviceData.Attributes, "ReadTotalUncorrectedErrors", 0)
	assert.Nil(t, findAttr(deviceData.Attributes, "ReadGigabytesProcessed"))
}

func TestParseSmartctlNvmeAppleSSD(t *testing.T) {
	// Apple SSDs don't report user_capacity via smartctl; capacity should be fetched
	// from system_profiler via the darwinNvmeProvider fallback.
	providerCalls := 0
	sm := &SmartManager{darwinNvmeProvider: func() ([]byte, error) {
		providerCalls++
		return []byte(`{"SPNVMeDataType": [{"_items": [{"device_serial": "0ba0147940253c15", "size_in_bytes": 251000193024}]}]}`), nil
	}}

	output := readSmartFixture(t, "apple_nvme.json")
	deviceData, _, err := sm.parseSmartctl(output, "")
	require.NoError(t, err)
	assert.Equal(t, "APPLE SSD AP0256Q", deviceData.ModelName)
	assert.Equal(t, uint64(251000193024), deviceData.Capacity)
	assert.Equal(t, uint8(42), deviceData.Temperature)
	assert.Equal(t, "PASSED", deviceData.SmartStatus)

	_, _, err = sm.parseSmartctl(output, "")
	require.NoError(t, err)
	assert.Equal(t, 1, providerCalls, "system_profiler should be called once")
}

func TestParseConfiguredDevices(t *testing.T) {
	devices, err := parseConfiguredDevices("/dev/sda:SAT, /dev/nvme0:nvme,/dev/sdb")
	require.NoError(t, err)
	assert.Equal(t, []smartDevice{
		{name: "/dev/sda", dtype: "sat", explicit: true},
		{name: "/dev/nvme0", dtype: "nvme", explicit: true},
		{name: "/dev/sdb"},
	}, devices)

	t.Setenv("SMART_DEVICES_SEPARATOR", "|")
	devices, err = parseConfiguredDevices("/dev/sda:jmb39x-q,0|/dev/bus/0:megaraid,1")
	require.NoError(t, err)
	assert.Equal(t, []smartDevice{
		{name: "/dev/sda", dtype: "jmb39x-q,0", explicit: true},
		{name: "/dev/bus/0", dtype: "megaraid,1", explicit: true},
	}, devices)

	_, err = parseConfiguredDevices(":sat")
	assert.Error(t, err)
	_, err = parseConfiguredDevices(" | ")
	assert.ErrorIs(t, err, errNoValidSmartData)
}

func TestMergeDevices(t *testing.T) {
	for _, test := range []struct {
		name       string
		scanned    []smartDevice
		configured []smartDevice
		want       []smartDevice
	}{
		{
			name:       "configured type replaces scanned type",
			scanned:    []smartDevice{{name: "/dev/sda", dtype: "scsi"}, {name: "/dev/nvme0", dtype: "nvme"}},
			configured: []smartDevice{{name: "/dev/sda", dtype: "sat", explicit: true}},
			want:       []smartDevice{{name: "/dev/sda", dtype: "sat", explicit: true}, {name: "/dev/nvme0", dtype: "nvme"}},
		},
		{
			name:       "configured path without type keeps scanned type",
			scanned:    []smartDevice{{name: "/dev/sda", dtype: "scsi"}},
			configured: []smartDevice{{name: "/dev/sda"}},
			want:       []smartDevice{{name: "/dev/sda", dtype: "scsi"}},
		},
		{
			name:       "same path and type marks scanned device explicit",
			scanned:    []smartDevice{{name: "/dev/sda", dtype: "scsi"}},
			configured: []smartDevice{{name: "/dev/sda", dtype: "scsi", explicit: true}},
			want:       []smartDevice{{name: "/dev/sda", dtype: "scsi", explicit: true}},
		},
		{
			name:       "unscanned devices are added",
			scanned:    []smartDevice{{name: "/dev/sda", dtype: "sat"}},
			configured: []smartDevice{{name: "/dev/sdb", dtype: "sat", explicit: true}},
			want:       []smartDevice{{name: "/dev/sda", dtype: "sat"}, {name: "/dev/sdb", dtype: "sat", explicit: true}},
		},
		{
			name: "RAID members sharing a path are added",
			scanned: []smartDevice{
				{name: "/dev/bus/0", dtype: "megaraid,0"},
				{name: "/dev/bus/0", dtype: "megaraid,1"},
			},
			configured: []smartDevice{{name: "/dev/bus/0", dtype: "megaraid,2", explicit: true}},
			want: []smartDevice{
				{name: "/dev/bus/0", dtype: "megaraid,0"},
				{name: "/dev/bus/0", dtype: "megaraid,1"},
				{name: "/dev/bus/0", dtype: "megaraid,2", explicit: true},
			},
		},
		{
			name:    "several configured types for a path scanned once",
			scanned: []smartDevice{{name: "/dev/sda", dtype: "scsi"}},
			configured: []smartDevice{
				{name: "/dev/sda", dtype: "cciss,0", explicit: true},
				{name: "/dev/sda", dtype: "cciss,1", explicit: true},
			},
			want: []smartDevice{
				{name: "/dev/sda", dtype: "cciss,0", explicit: true},
				{name: "/dev/sda", dtype: "cciss,1", explicit: true},
			},
		},
		{
			name:       "duplicates are dropped",
			scanned:    []smartDevice{{name: "/dev/sda", dtype: "sat"}, {name: "/dev/sda", dtype: "sat"}},
			configured: []smartDevice{{name: "/dev/sdb"}, {name: "/dev/sdb"}},
			want:       []smartDevice{{name: "/dev/sda", dtype: "sat"}, {name: "/dev/sdb"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, mergeDevices(test.scanned, test.configured))
		})
	}
}

func TestAttemptsFor(t *testing.T) {
	sm := &SmartManager{excludedDevices: map[string]struct{}{"/dev/nvme1n1": {}}}
	for _, test := range []struct {
		device smartDevice
		want   []smartAttempt
	}{
		{smartDevice{name: "/dev/sda", dtype: "sat"}, []smartAttempt{{"/dev/sda", "sat"}}},
		{smartDevice{name: "/dev/sda"}, []smartAttempt{{"/dev/sda", ""}}},
		{smartDevice{name: "/dev/sda", dtype: "ata"}, []smartAttempt{{"/dev/sda", ""}}},
		{smartDevice{name: "/dev/sda", dtype: "scsi"}, []smartAttempt{{"/dev/sda", ""}, {"/dev/sda", "sat"}}},
		{smartDevice{name: "/dev/sda", dtype: "scsi", explicit: true}, []smartAttempt{{"/dev/sda", "scsi"}}},
		{smartDevice{name: "/dev/sda", dtype: "ata", explicit: true}, []smartAttempt{{"/dev/sda", "ata"}}},
		{smartDevice{name: "/dev/bus/0", dtype: "megaraid,1"}, []smartAttempt{{"/dev/bus/0", "megaraid,1"}}},
		{smartDevice{name: "/dev/nvme0", dtype: "nvme"}, []smartAttempt{{"/dev/nvme0", "nvme"}, {"/dev/nvme0n1", "nvme"}}},
		{smartDevice{name: "/dev/nvme0", dtype: "nvme", explicit: true}, []smartAttempt{{"/dev/nvme0", "nvme"}, {"/dev/nvme0n1", "nvme"}}},
		{smartDevice{name: "/dev/nvme0"}, []smartAttempt{{"/dev/nvme0", ""}, {"/dev/nvme0n1", ""}}},
		{smartDevice{name: "/dev/nvme0", dtype: "sntasmedia"}, []smartAttempt{{"/dev/nvme0", "sntasmedia"}}},
		{smartDevice{name: "/dev/nvme0n1", dtype: "nvme"}, []smartAttempt{{"/dev/nvme0n1", "nvme"}}},
		{smartDevice{name: "/dev/nvme1", dtype: "nvme"}, []smartAttempt{{"/dev/nvme1", "nvme"}}},
	} {
		assert.Equal(t, test.want, sm.attemptsFor(test.device), "%+v", test.device)
	}
}

func TestSmartctlArgs(t *testing.T) {
	for _, test := range []struct {
		attempt smartAttempt
		ata     bool
		standby bool
		want    string
	}{
		{smartAttempt{"/dev/sda", ""}, false, true, "-a --json=c -n standby /dev/sda"},
		{smartAttempt{"/dev/sda", ""}, true, true, "-a --json=c -l devstat -n standby /dev/sda"},
		{smartAttempt{"/dev/sda", "sat"}, false, true, "-d sat -a --json=c -l devstat -n standby /dev/sda"},
		{smartAttempt{"/dev/sda", "ata"}, false, true, "-d ata -a --json=c -l devstat -n standby /dev/sda"},
		{smartAttempt{"/dev/nvme0", "nvme"}, false, true, "-d nvme -a --json=c -n standby /dev/nvme0"},
		{smartAttempt{"/dev/bus/0", "megaraid,1"}, true, true, "-d megaraid,1 -a --json=c -l devstat -n standby /dev/bus/0"},
		{smartAttempt{"/dev/sda", "sat"}, false, false, "-d sat -a --json=c -l devstat /dev/sda"},
		{smartAttempt{"/dev/nvme0", "nvme"}, false, false, "-d nvme -a --json=c /dev/nvme0"},
	} {
		assert.Equal(t, test.want, strings.Join(smartctlArgs(test.attempt, test.ata, test.standby), " "))
	}
}

func TestIsStandby(t *testing.T) {
	assert.True(t, isStandby([]byte(standbyOutput), 2))
	assert.True(t, isStandby([]byte(`{"smartctl":{"messages":[{"string":"Device is in SLEEP mode, exit(2)"}]}}`), 2))
	assert.False(t, isStandby([]byte(standbyOutput), 0), "needs exit bit 1")
	assert.False(t, isStandby([]byte(`{"smartctl":{"messages":[{"string":"Permission denied"}]}}`), 2))
	// smartctl reports SLEEP (power_mode.ata_value -1) when it lacks permission
	// to check the power mode; a really sleeping disk looks the same.
	assert.True(t, isStandby(readSmartFixture(t, "sat_sleep_permission.json"), 2))
}

const standbyOutput = `{"smartctl":{"exit_status":2,"messages":[{"string":"Device is in STANDBY mode, exit(2)","severity":"information"}]},"device":{"name":"/dev/sda","type":"sat"}}`

// fakeSmartctl answers smartctl calls from a table keyed by "<-d type> <path>"
// (an empty type for auto-detection, e.g. " /dev/sda") and records each call.
// Calls without -n standby are answered from wake first, if it has the key.
type fakeSmartctl struct {
	scan      string
	scanExit  int
	scanErr   error
	responses map[string]fakeResponse
	wake      map[string]fakeResponse
	calls     []string
}

type fakeResponse struct {
	output string
	exit   int
}

func (f *fakeSmartctl) run(_ time.Duration, args ...string) ([]byte, int, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if call == "--scan -j" {
		return []byte(f.scan), f.scanExit, f.scanErr
	}
	dtype := ""
	if i := slices.Index(args, "-d"); i >= 0 {
		dtype = args[i+1]
	}
	key := dtype + " " + args[len(args)-1]
	response, ok := f.wake[key]
	if !ok || slices.Contains(args, "-n") {
		response, ok = f.responses[key]
	}
	if !ok {
		return []byte(`{"smartctl":{"exit_status":2,"messages":[{"string":"No such device"}]}}`), 2, nil
	}
	return []byte(response.output), response.exit, nil
}

// deviceCalls returns the recorded calls other than the scan.
func (f *fakeSmartctl) deviceCalls() []string {
	return slices.DeleteFunc(slices.Clone(f.calls), func(call string) bool { return call == "--scan -j" })
}

func newFakeSmartManager(t *testing.T, fake *fakeSmartctl) *SmartManager {
	t.Helper()
	original := scanSysfsDevices
	scanSysfsDevices = func() []smartDevice { return nil }
	t.Cleanup(func() { scanSysfsDevices = original })
	return &SmartManager{data: map[string]*smart.SmartData{}, run: fake.run}
}

// nvmeAt returns the nvme0.json fixture as if smartctl had queried path.
func nvmeAt(t *testing.T, path string) fakeResponse {
	return fakeResponse{output: strings.ReplaceAll(string(readSmartFixture(t, "nvme0.json")), `"/dev/nvme0"`, `"`+path+`"`)}
}

func scanOf(devices ...string) string {
	entries := make([]string, 0, len(devices))
	for _, device := range devices {
		name, dtype, _ := strings.Cut(device, ":")
		entries = append(entries, fmt.Sprintf(`{"name":%q,"type":%q}`, name, dtype))
	}
	return `{"devices":[` + strings.Join(entries, ",") + `]}`
}

// A SATA disk that the scan reports as scsi returns identity-only output when
// auto-detected; the sat attempt recovers its data and is tried first next time (#2295).
func TestRefreshScsiFallsBackToSat(t *testing.T) {
	fake := &fakeSmartctl{
		scan: scanOf("/dev/sda:scsi"),
		responses: map[string]fakeResponse{
			" /dev/sda":    {output: string(readSmartFixture(t, "scsi_identity.json"))},
			"sat /dev/sda": {output: string(readSmartFixture(t, "sat.json"))},
		},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	data := sm.GetCurrentData()["Z4K2A01FS"]
	assert.Equal(t, "TOSHIBA MG03ACA200", data.ModelName)
	assert.Equal(t, "sat", data.DiskType)
	assert.Equal(t, uint8(38), data.Temperature)
	assert.Equal(t, "PASSED", data.SmartStatus)
	assert.Equal(t, []string{
		"-a --json=c -n standby /dev/sda",
		"-d sat -a --json=c -l devstat -n standby /dev/sda",
	}, fake.deviceCalls())

	fake.calls = nil
	_, err = sm.Refresh(false)
	require.NoError(t, err)
	assert.Equal(t, []string{"-d sat -a --json=c -l devstat -n standby /dev/sda"}, fake.deviceCalls())
}

// A disk scanned as ata is auto-detected but requests devstat from the first
// query, including a wake query; after that the protocol of the reading decides.
func TestRefreshScannedAtaRequestsDevstat(t *testing.T) {
	for _, test := range []struct {
		fixture, next string
		asleep        bool
	}{
		{"sda.json", "-a --json=c -l devstat -n standby /dev/sda", false},
		{"scsi.json", "-a --json=c -n standby /dev/sda", false},
		{"sda.json", "-a --json=c -l devstat -n standby /dev/sda", true},
		{"scsi.json", "-a --json=c -n standby /dev/sda", true},
	} {
		t.Run(fmt.Sprintf("%s/asleep=%t", test.fixture, test.asleep), func(t *testing.T) {
			reading := fakeResponse{output: string(readSmartFixture(t, test.fixture))}
			fake := &fakeSmartctl{
				scan:      scanOf("/dev/sda:ata"),
				responses: map[string]fakeResponse{" /dev/sda": reading},
			}
			wantCalls := []string{"-a --json=c -l devstat -n standby /dev/sda"}
			if test.asleep {
				fake.responses[" /dev/sda"] = fakeResponse{output: standbyOutput, exit: 2}
				fake.wake = map[string]fakeResponse{" /dev/sda": reading}
				wantCalls = append(wantCalls, "-a --json=c -l devstat /dev/sda")
			}
			sm := newFakeSmartManager(t, fake)

			_, err := sm.Refresh(false)
			require.NoError(t, err)
			assert.Equal(t, wantCalls, fake.deviceCalls())

			fake.calls = nil
			fake.responses[" /dev/sda"] = reading
			_, err = sm.Refresh(false)
			require.NoError(t, err)
			assert.Equal(t, []string{test.next}, fake.deviceCalls())
		})
	}
}

// An explicit SMART_DEVICES type is a deliberate override and is not retried.
func TestRefreshExplicitTypeIsNotRetried(t *testing.T) {
	t.Setenv("SMART_DEVICES", "/dev/sda:scsi")
	fake := &fakeSmartctl{
		scan: scanOf(),
		responses: map[string]fakeResponse{
			"scsi /dev/sda": {output: string(readSmartFixture(t, "scsi_identity.json"))},
			"sat /dev/sda":  {output: string(readSmartFixture(t, "sat.json"))},
		},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	assert.ErrorIs(t, err, errNoHealthData)
	assert.False(t, complete)
	assert.Empty(t, sm.GetCurrentData())
	assert.Equal(t, []string{"-d scsi -a --json=c -n standby /dev/sda"}, fake.deviceCalls())
}

// NVMe controllers that reject SMART queries fall back to their namespace,
// which is then queried directly on later refreshes (#1504, #2597).
func TestRefreshNvmeNamespaceFallback(t *testing.T) {
	fake := &fakeSmartctl{
		scan:      scanOf("/dev/nvme0:nvme"),
		responses: map[string]fakeResponse{"nvme /dev/nvme0n1": nvmeAt(t, "/dev/nvme0n1")},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, "/dev/nvme0n1", sm.GetCurrentData()["2024031600129"].DiskName)
	assert.Equal(t, []string{
		"-d nvme -a --json=c -n standby /dev/nvme0",
		"-d nvme -a --json=c -n standby /dev/nvme0n1",
	}, fake.deviceCalls())

	// The next refresh, after the scan cache expired, goes straight to the namespace.
	fake.calls = nil
	sm.lastScan = time.Now().Add(-smartScanInterval - time.Minute)
	complete, err = sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, []string{"-d nvme -a --json=c -n standby /dev/nvme0n1"}, fake.deviceCalls())

	// A scan that reports both aliases does not duplicate the drive.
	fake.scan = scanOf("/dev/nvme0:nvme", "/dev/nvme0n1:nvme")
	complete, err = sm.Refresh(true)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Len(t, sm.GetCurrentData(), 1)
}

func TestRefreshNvmeNamespaceFailure(t *testing.T) {
	denied := fakeResponse{output: string(readSmartFixture(t, "nvme_permission_denied.json")), exit: 4}
	fake := &fakeSmartctl{
		scan:      scanOf("/dev/nvme0:nvme"),
		responses: map[string]fakeResponse{"nvme /dev/nvme0": denied, "nvme /dev/nvme0n1": denied},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	assert.ErrorIs(t, err, errNoHealthData)
	assert.False(t, complete)
	assert.Empty(t, sm.GetCurrentData())
	assert.Len(t, fake.deviceCalls(), 2)
}

func TestRefreshHonorsExclusions(t *testing.T) {
	for _, excluded := range []string{"/dev/nvme0", "/dev/nvme0n1"} {
		t.Run(excluded, func(t *testing.T) {
			fake := &fakeSmartctl{
				scan:      scanOf("/dev/nvme0:nvme"),
				responses: map[string]fakeResponse{"nvme /dev/nvme0n1": nvmeAt(t, "/dev/nvme0n1")},
			}
			sm := newFakeSmartManager(t, fake)
			sm.excludedDevices = map[string]struct{}{excluded: {}}

			_, err := sm.Refresh(false)
			assert.Error(t, err)
			assert.Empty(t, sm.GetCurrentData())
			assert.NotContains(t, fake.deviceCalls(), "-d nvme -a --json=c -n standby /dev/nvme0n1")
		})
	}
}

// A sleeping disk that was collected before is never woken: its last data is reused.
func TestRefreshStandby(t *testing.T) {
	fake := &fakeSmartctl{
		scan:      scanOf("/dev/sda:sat"),
		responses: map[string]fakeResponse{"sat /dev/sda": {output: string(readSmartFixture(t, "sda.json")), exit: 64}},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)

	fake.responses["sat /dev/sda"] = fakeResponse{output: standbyOutput, exit: 2}
	complete, err = sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete, "reusing the last data counts as collected")
	assert.Equal(t, "P3-2TB", sm.GetCurrentData()["9C40918040082"].ModelName)

	for _, call := range fake.deviceCalls() {
		assert.Contains(t, call, "-n standby")
	}
}

// A temporary read failure must not forget the last successful reading used
// when the disk subsequently enters standby, even if it was already woken once.
func TestRefreshStandbyAfterCollectionFailure(t *testing.T) {
	for _, initiallyAsleep := range []bool{false, true} {
		t.Run(fmt.Sprintf("initiallyAsleep=%t", initiallyAsleep), func(t *testing.T) {
			reading := fakeResponse{output: string(readSmartFixture(t, "sda.json"))}
			standby := fakeResponse{output: standbyOutput, exit: 2}
			fake := &fakeSmartctl{
				scan:      scanOf("/dev/sda:sat"),
				responses: map[string]fakeResponse{"sat /dev/sda": reading},
				wake:      map[string]fakeResponse{"sat /dev/sda": reading},
			}
			if initiallyAsleep {
				fake.responses["sat /dev/sda"] = standby
			}
			sm := newFakeSmartManager(t, fake)
			_, err := sm.Refresh(false)
			require.NoError(t, err)
			previous := sm.GetCurrentData()

			fake.responses["sat /dev/sda"] = fakeResponse{output: `{"smartctl":{"exit_status":4}}`, exit: 4}
			for range 2 {
				complete, err := sm.Refresh(false)
				assert.Error(t, err)
				assert.False(t, complete)
				assert.Empty(t, sm.GetCurrentData(), "failed reads must not be reported as fresh data")
			}

			fake.responses["sat /dev/sda"] = standby
			fake.calls = nil
			complete, err := sm.Refresh(false)
			require.NoError(t, err)
			assert.True(t, complete)
			assert.Equal(t, previous, sm.GetCurrentData())
			assert.Equal(t, []string{"-d sat -a --json=c -l devstat -n standby /dev/sda"}, fake.deviceCalls())
		})
	}
}

// A sleeping disk with no earlier data is woken once so it shows up.
func TestRefreshWakesUncollectedStandbyDisk(t *testing.T) {
	fake := &fakeSmartctl{
		scan:      scanOf("/dev/sda:sat"),
		responses: map[string]fakeResponse{"sat /dev/sda": {output: standbyOutput, exit: 2}},
		wake:      map[string]fakeResponse{"sat /dev/sda": {output: string(readSmartFixture(t, "sda.json")), exit: 64}},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, "P3-2TB", sm.GetCurrentData()["9C40918040082"].ModelName)
	assert.Equal(t, []string{
		"-d sat -a --json=c -l devstat -n standby /dev/sda",
		"-d sat -a --json=c -l devstat /dev/sda",
	}, fake.deviceCalls())

	// Collected now, so later standby reuses its data instead of waking it.
	fake.calls = nil
	complete, err = sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, []string{"-d sat -a --json=c -l devstat -n standby /dev/sda"}, fake.deviceCalls())
}

// A disk whose wake query fails is not woken again on later refreshes, and is
// skipped with the refresh reported incomplete so the hub keeps its record.
func TestRefreshWakeFailureNotRetried(t *testing.T) {
	fake := &fakeSmartctl{
		scan:      scanOf("/dev/sda:sat"),
		responses: map[string]fakeResponse{"sat /dev/sda": {output: string(readSmartFixture(t, "sat_sleep_permission.json")), exit: 2}},
		wake:      map[string]fakeResponse{"sat /dev/sda": {output: `{"smartctl":{"exit_status":2,"messages":[{"string":"Permission denied"}]}}`, exit: 2}},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	assert.False(t, complete)
	assert.ErrorContains(t, err, "exit status 2", "the wake query's own error is reported, not standby")
	assert.Empty(t, sm.GetCurrentData())
	assert.Equal(t, []string{
		"-d sat -a --json=c -l devstat -n standby /dev/sda",
		"-d sat -a --json=c -l devstat /dev/sda",
	}, fake.deviceCalls())

	for range 2 {
		fake.calls = nil
		complete, err = sm.Refresh(false)
		assert.False(t, complete, "a skipped sleeping disk makes the refresh incomplete")
		assert.ErrorIs(t, err, errNoValidSmartData)
		assert.Equal(t, []string{"-d sat -a --json=c -l devstat -n standby /dev/sda"}, fake.deviceCalls())
	}
}

// A failed wake query moves on to the next attempt, which runs with -n standby
// against the now-awake disk (#2295).
func TestRefreshWakeFallsBackToSat(t *testing.T) {
	fake := &fakeSmartctl{
		scan: scanOf("/dev/sda:scsi"),
		responses: map[string]fakeResponse{
			" /dev/sda":    {output: standbyOutput, exit: 2},
			"sat /dev/sda": {output: string(readSmartFixture(t, "sat.json"))},
		},
		wake: map[string]fakeResponse{" /dev/sda": {output: string(readSmartFixture(t, "scsi_identity.json"))}},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, "sat", sm.GetCurrentData()["Z4K2A01FS"].DiskType)
	assert.Equal(t, []string{
		"-a --json=c -n standby /dev/sda",
		"-a --json=c /dev/sda",
		"-d sat -a --json=c -l devstat -n standby /dev/sda",
	}, fake.deviceCalls())
}

// Data is rebuilt on each refresh, so a removed disk disappears.
func TestRefreshPrunesRemovedDevices(t *testing.T) {
	fake := &fakeSmartctl{
		scan: scanOf("/dev/sda:sat", "/dev/nvme0:nvme"),
		responses: map[string]fakeResponse{
			"sat /dev/sda":    {output: string(readSmartFixture(t, "sda.json"))},
			"nvme /dev/nvme0": nvmeAt(t, "/dev/nvme0"),
		},
	}
	sm := newFakeSmartManager(t, fake)

	_, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.Len(t, sm.GetCurrentData(), 2)

	// A failed read retains standby history, but a subsequent scan that removes
	// the disk must still discard that history.
	delete(fake.responses, "sat /dev/sda")
	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.False(t, complete)

	fake.scan = scanOf("/dev/nvme0:nvme")
	complete, err = sm.Refresh(true)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, []string{"2024031600129"}, slices.Collect(maps.Keys(sm.GetCurrentData())))
	assert.NotContains(t, sm.last, "/dev/sda|sat")
}

// One failing disk makes the refresh incomplete without hiding the others.
func TestRefreshPartialFailure(t *testing.T) {
	fake := &fakeSmartctl{
		scan:      scanOf("/dev/sda:sat", "/dev/sdb:sat"),
		responses: map[string]fakeResponse{"sat /dev/sda": {output: string(readSmartFixture(t, "sda.json"))}},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.False(t, complete)
	assert.Len(t, sm.GetCurrentData(), 1)
}

// The same disk seen through two paths is reported once, by its first path (#1907).
func TestRefreshDuplicateSerial(t *testing.T) {
	sda := readSmartFixture(t, "sda.json")
	fake := &fakeSmartctl{
		scan: scanOf("/dev/sda:sat", "/dev/sdb:sat"),
		responses: map[string]fakeResponse{
			"sat /dev/sda": {output: string(sda)},
			"sat /dev/sdb": {output: strings.ReplaceAll(string(sda), `"/dev/sda"`, `"/dev/sdb"`)},
		},
	}
	sm := newFakeSmartManager(t, fake)

	for range 2 {
		complete, err := sm.Refresh(false)
		require.NoError(t, err)
		assert.True(t, complete)
		require.Len(t, sm.GetCurrentData(), 1)
		assert.Equal(t, "/dev/sda", sm.GetCurrentData()["9C40918040082"].DiskName)
	}
}

func TestRefreshScanFailureKeepsPreviousScan(t *testing.T) {
	for _, test := range []struct {
		name, output string
		exit         int
		err          error
	}{
		{name: "timeout", err: errors.New("scan timed out")},
		{name: "invalid JSON", output: "invalid JSON"},
		{name: "nonzero exit without devices", output: `{"smartctl":{"exit_status":1}}`, exit: 1},
		{name: "nonzero exit with partial devices", output: scanOf("/dev/sdb:sat"), exit: 1},
		{name: "empty device list", output: scanOf()},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeSmartctl{
				scan:      scanOf("/dev/sda:sat"),
				responses: map[string]fakeResponse{"sat /dev/sda": {output: string(readSmartFixture(t, "sda.json"))}},
			}
			sm := newFakeSmartManager(t, fake)
			_, err := sm.Refresh(false)
			require.NoError(t, err)

			fake.scan, fake.scanExit, fake.scanErr = test.output, test.exit, test.err
			for _, forceScan := range []bool{true, false} {
				fake.calls = nil
				complete, err := sm.Refresh(forceScan)
				require.NoError(t, err)
				assert.False(t, complete, "a failed scan must not let the hub prune devices")
				assert.Contains(t, sm.GetCurrentData(), "9C40918040082")
				assert.Error(t, sm.scanErr)
				if test.exit != 0 {
					assert.ErrorContains(t, sm.scanErr, "exit status 1")
				}
				assert.Equal(t, []string{"-d sat -a --json=c -l devstat -n standby /dev/sda"}, fake.deviceCalls())
				assert.Equal(t, forceScan, slices.Contains(fake.calls, "--scan -j"))
			}

			fake.scan, fake.scanExit, fake.scanErr = scanOf("/dev/sda:sat"), 0, nil
			complete, err := sm.Refresh(true)
			require.NoError(t, err)
			assert.True(t, complete)
			assert.NoError(t, sm.scanErr)
		})
	}
}

func TestRefreshWithSmartDevices(t *testing.T) {
	t.Setenv("SMART_DEVICES", "/dev/sda:sat,/dev/nvme0:nvme")
	fake := &fakeSmartctl{
		scan: scanOf("/dev/sda:scsi"),
		responses: map[string]fakeResponse{
			"sat /dev/sda":    {output: string(readSmartFixture(t, "sda.json"))},
			"nvme /dev/nvme0": nvmeAt(t, "/dev/nvme0"),
		},
	}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Len(t, sm.GetCurrentData(), 2)
	assert.Equal(t, []string{
		"-d sat -a --json=c -l devstat -n standby /dev/sda",
		"-d nvme -a --json=c -n standby /dev/nvme0",
	}, fake.deviceCalls())
}

func TestRefreshSmartDevicesEmptyDisablesSmart(t *testing.T) {
	t.Setenv("SMART_DEVICES", "   ")
	fake := &fakeSmartctl{scan: scanOf("/dev/sda:sat")}
	sm := newFakeSmartManager(t, fake)

	complete, err := sm.Refresh(false)
	assert.ErrorIs(t, err, errNoValidSmartData)
	assert.False(t, complete)
	assert.Empty(t, fake.calls)
}

func TestRefreshSmartDevicesInvalid(t *testing.T) {
	t.Setenv("SMART_DEVICES", ":sat")
	sm := newFakeSmartManager(t, &fakeSmartctl{scan: scanOf()})

	_, err := sm.Refresh(false)
	assert.Error(t, err)
}

func TestRefreshNoDevices(t *testing.T) {
	sm := newFakeSmartManager(t, &fakeSmartctl{scan: scanOf()})

	complete, err := sm.Refresh(false)
	assert.ErrorIs(t, err, errNoValidSmartData)
	assert.False(t, complete)
}

func assertAttrValue(t *testing.T, attributes []*smart.SmartAttribute, name string, expected uint64) {
	t.Helper()
	attr := findAttr(attributes, name)
	if attr == nil {
		t.Fatalf("expected attribute %s to be present", name)
	}
	if attr.RawValue != expected {
		t.Fatalf("unexpected attribute %s value: got %d, want %d", name, attr.RawValue, expected)
	}
}

func findAttr(attributes []*smart.SmartAttribute, name string) *smart.SmartAttribute {
	for _, attr := range attributes {
		if attr != nil && attr.Name == name {
			return attr
		}
	}
	return nil
}

func TestIsVirtualDevice(t *testing.T) {
	tests := []struct {
		name     string
		data     smart.SmartctlOutput
		expected bool
	}{
		{"regular sata", smart.SmartctlOutput{ScsiVendor: "SEAGATE", ScsiProduct: "ST1000DM003", ModelName: "ST1000DM003-1CH162"}, false},
		{"qemu sata", smart.SmartctlOutput{ScsiVendor: "QEMU", ScsiProduct: "QEMU HARDDISK", ModelName: "QEMU HARDDISK"}, true},
		{"virtualbox sata", smart.SmartctlOutput{ScsiVendor: "VBOX", ScsiProduct: "HARDDISK", ModelName: "VBOX HARDDISK"}, true},
		{"vmware sata", smart.SmartctlOutput{ScsiVendor: "VMWARE", ScsiProduct: "Virtual disk", ModelName: "VMWARE Virtual disk"}, true},
		{"virtual in model", smart.SmartctlOutput{ScsiVendor: "ATA", ScsiProduct: "VIRTUAL", ModelName: "VIRTUAL DISK"}, true},
		{"iet virtual", smart.SmartctlOutput{ScsiVendor: "IET", ScsiProduct: "VIRTUAL-DISK", ModelName: "VIRTUAL-DISK"}, true},
		{"hyper-v virtual", smart.SmartctlOutput{ScsiVendor: "MSFT", ScsiProduct: "VIRTUAL HD", ModelName: "VIRTUAL HD"}, true},
		{"regular nvme", smart.SmartctlOutput{ModelName: "Samsung SSD 970 EVO Plus 1TB"}, false},
		{"qemu nvme", smart.SmartctlOutput{ModelName: "QEMU NVMe Ctrl"}, true},
		{"virtual nvme", smart.SmartctlOutput{ModelName: "Virtual NVMe Device"}, true},
		{"regular scsi", smart.SmartctlOutput{ScsiVendor: "SEAGATE", ScsiProduct: "ST1000DM003", ScsiModelName: "ST1000DM003-1CH162"}, false},
		{"vmware scsi", smart.SmartctlOutput{ScsiVendor: "VMWARE", ScsiProduct: "Virtual disk", ScsiModelName: "VMWARE Virtual disk"}, true},
		{"virtual scsi model only", smart.SmartctlOutput{ScsiModelName: "QEMU HARDDISK"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isVirtualDevice(&tt.data))
		})
	}
}

func TestFindAtaDeviceStatisticsValue(t *testing.T) {
	val42 := int64(42)
	val100 := int64(100)
	valMinus20 := int64(-20)

	tests := []struct {
		name           string
		data           smart.SmartctlOutput
		ataDeviceStats smart.AtaDeviceStatistics
		entryNumber    uint8
		entryName      string
		minValue       int64
		maxValue       int64
		expectedValue  *int64
	}{
		{
			name: "value in ataDeviceStats",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 5,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "Current Temperature", Value: &val42},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      100,
			expectedValue: &val42,
		},
		{
			name: "value unmarshaled from data",
			data: smart.SmartctlOutput{
				AtaDeviceStatistics: []byte(`{"pages":[{"number":5,"table":[{"name":"Current Temperature","value":100}]}]}`),
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      255,
			expectedValue: &val100,
		},
		{
			name: "value out of range (too high)",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 5,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "Current Temperature", Value: &val100},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      50,
			expectedValue: nil,
		},
		{
			name: "value out of range (too low)",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 5,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "Min Temp", Value: &valMinus20},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Min Temp",
			minValue:      0,
			maxValue:      100,
			expectedValue: nil,
		},
		{
			name:          "no statistics available",
			data:          smart.SmartctlOutput{},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      255,
			expectedValue: nil,
		},
		{
			name: "wrong page number",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 1,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "Current Temperature", Value: &val42},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      100,
			expectedValue: nil,
		},
		{
			name: "wrong entry name",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 5,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "Other Stat", Value: &val42},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      100,
			expectedValue: nil,
		},
		{
			name: "case insensitive name match",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 5,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "CURRENT TEMPERATURE", Value: &val42},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      100,
			expectedValue: &val42,
		},
		{
			name: "entry value is nil",
			ataDeviceStats: smart.AtaDeviceStatistics{
				Pages: []smart.AtaDeviceStatisticsPage{
					{
						Number: 5,
						Table: []smart.AtaDeviceStatisticsEntry{
							{Name: "Current Temperature", Value: nil},
						},
					},
				},
			},
			entryNumber:   5,
			entryName:     "Current Temperature",
			minValue:      0,
			maxValue:      100,
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findAtaDeviceStatisticsValue(&tt.data, &tt.ataDeviceStats, tt.entryNumber, tt.entryName, tt.minValue, tt.maxValue)
			if tt.expectedValue == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expectedValue, *result)
			}
		})
	}
}

func TestRefreshExcludedDevices(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		expectedDevs map[string]struct{}
	}{
		{
			name:         "empty env",
			envValue:     "",
			expectedDevs: map[string]struct{}{},
		},
		{
			name:     "single device",
			envValue: "/dev/sda",
			expectedDevs: map[string]struct{}{
				"/dev/sda": {},
			},
		},
		{
			name:     "multiple devices",
			envValue: "/dev/sda,/dev/sdb,/dev/nvme0",
			expectedDevs: map[string]struct{}{
				"/dev/sda":   {},
				"/dev/sdb":   {},
				"/dev/nvme0": {},
			},
		},
		{
			name:     "devices with whitespace",
			envValue: " /dev/sda , /dev/sdb ,  /dev/nvme0  ",
			expectedDevs: map[string]struct{}{
				"/dev/sda":   {},
				"/dev/sdb":   {},
				"/dev/nvme0": {},
			},
		},
		{
			name:     "duplicate devices",
			envValue: "/dev/sda,/dev/sdb,/dev/sda",
			expectedDevs: map[string]struct{}{
				"/dev/sda": {},
				"/dev/sdb": {},
			},
		},
		{
			name:     "empty entries and whitespace",
			envValue: "/dev/sda,, /dev/sdb , , ",
			expectedDevs: map[string]struct{}{
				"/dev/sda": {},
				"/dev/sdb": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("EXCLUDE_SMART", tt.envValue)
			} else {
				// Ensure env var is not set for empty test
				t.Setenv("EXCLUDE_SMART", "")
			}

			sm := &SmartManager{}
			sm.refreshExcludedDevices()

			assert.Equal(t, tt.expectedDevs, sm.excludedDevices)
		})
	}
}

func TestIsExcludedDevice(t *testing.T) {
	sm := &SmartManager{
		excludedDevices: map[string]struct{}{
			"/dev/sda":   {},
			"/dev/nvme0": {},
		},
	}

	tests := []struct {
		name         string
		deviceName   string
		expectedBool bool
	}{
		{"excluded device sda", "/dev/sda", true},
		{"excluded device nvme0", "/dev/nvme0", true},
		{"non-excluded device sdb", "/dev/sdb", false},
		{"non-excluded device nvme1", "/dev/nvme1", false},
		{"empty device name", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sm.isExcludedDevice(tt.deviceName)
			assert.Equal(t, tt.expectedBool, result)
		})
	}
}

func TestIsNvmeControllerPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Controller paths (should return true)
		{"/dev/nvme0", true},
		{"/dev/nvme1", true},
		{"/dev/nvme10", true},
		{"nvme0", true},

		// Namespace paths (should return false)
		{"/dev/nvme0n1", false},
		{"/dev/nvme1n1", false},
		{"/dev/nvme0n1p1", false},
		{"nvme0n1", false},

		// Non-NVMe paths (should return false)
		{"/dev/sda", false},
		{"/dev/sda1", false},
		{"/dev/hda", false},
		{"", false},
		{"/dev/nvme", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isNvmeControllerPath(tt.path)
			assert.Equal(t, tt.expected, result, "path: %s", tt.path)
		})
	}
}

func TestLookupDarwinNvmeCapacityMultipleDisks(t *testing.T) {
	fakeProvider := func() ([]byte, error) {
		return []byte(`{
			"SPNVMeDataType": [
				{
					"_items": [
						{"device_serial": "serial-disk0", "size_in_bytes": 251000193024},
						{"device_serial": "serial-disk1", "size_in_bytes": 1000204886016}
					]
				},
				{
					"_items": [
						{"device_serial": "serial-disk2", "size_in_bytes": 512110190592}
					]
				}
			]
		}`), nil
	}

	sm := &SmartManager{darwinNvmeProvider: fakeProvider}
	assert.Equal(t, uint64(251000193024), sm.lookupDarwinNvmeCapacity("serial-disk0"))
	assert.Equal(t, uint64(1000204886016), sm.lookupDarwinNvmeCapacity("serial-disk1"))
	assert.Equal(t, uint64(512110190592), sm.lookupDarwinNvmeCapacity("serial-disk2"))
	assert.Equal(t, uint64(0), sm.lookupDarwinNvmeCapacity("unknown-serial"))
}

func TestLookupDarwinNvmeCapacityProviderError(t *testing.T) {
	fakeProvider := func() ([]byte, error) {
		return nil, errors.New("system_profiler not found")
	}

	sm := &SmartManager{darwinNvmeProvider: fakeProvider}
	assert.Equal(t, uint64(0), sm.lookupDarwinNvmeCapacity("any-serial"))
	// Cache should be initialized even on error so we don't retry (Once already fired)
	assert.NotNil(t, sm.darwinNvmeCapacity)
}
