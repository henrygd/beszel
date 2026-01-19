//go:build testing
// +build testing

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSmartForScsi(t *testing.T) {
	fixturePath := filepath.Join("test-data", "smart", "scsi.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed reading fixture: %v", err)
	}

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	hasData, exitStatus := sm.parseSmartForScsi(data)
	if !hasData {
		t.Fatalf("expected SCSI data to parse successfully")
	}
	if exitStatus != 0 {
		t.Fatalf("expected exit status 0, got %d", exitStatus)
	}

	deviceData, ok := sm.SmartDataMap["9YHSDH9B"]
	if !ok {
		t.Fatalf("expected smart data entry for serial 9YHSDH9B")
	}

	assert.Equal(t, deviceData.ModelName, "YADRO WUH721414AL4204")
	assert.Equal(t, deviceData.SerialNumber, "9YHSDH9B")
	assert.Equal(t, deviceData.FirmwareVersion, "C240")
	assert.Equal(t, deviceData.DiskName, "/dev/sde")
	assert.Equal(t, deviceData.DiskType, "scsi")
	assert.EqualValues(t, deviceData.Temperature, 34)
	assert.Equal(t, deviceData.SmartStatus, "PASSED")
	assert.EqualValues(t, deviceData.Capacity, 14000519643136)

	if len(deviceData.Attributes) == 0 {
		t.Fatalf("expected attributes to be populated")
	}

	assertAttrValue(t, deviceData.Attributes, "PowerOnHours", 458)
	assertAttrValue(t, deviceData.Attributes, "PowerOnMinutes", 25)
	assertAttrValue(t, deviceData.Attributes, "GrownDefectList", 0)
	assertAttrValue(t, deviceData.Attributes, "StartStopCycles", 2)
	assertAttrValue(t, deviceData.Attributes, "LoadUnloadCycles", 418)
	assertAttrValue(t, deviceData.Attributes, "ReadGigabytesProcessed", 3641)
	assertAttrValue(t, deviceData.Attributes, "WriteGigabytesProcessed", 2124590)
	assertAttrValue(t, deviceData.Attributes, "VerifyGigabytesProcessed", 0)
}

func TestParseSmartForSata(t *testing.T) {
	fixturePath := filepath.Join("test-data", "smart", "sda.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	hasData, exitStatus := sm.parseSmartForSata(data)
	require.True(t, hasData)
	assert.Equal(t, 64, exitStatus)

	deviceData, ok := sm.SmartDataMap["9C40918040082"]
	require.True(t, ok, "expected smart data entry for serial 9C40918040082")

	assert.Equal(t, "P3-2TB", deviceData.ModelName)
	assert.Equal(t, "X0104A0", deviceData.FirmwareVersion)
	assert.Equal(t, "/dev/sda", deviceData.DiskName)
	assert.Equal(t, "sat", deviceData.DiskType)
	assert.Equal(t, uint8(31), deviceData.Temperature)
	assert.Equal(t, "PASSED", deviceData.SmartStatus)
	assert.Equal(t, uint64(2048408248320), deviceData.Capacity)
	if assert.NotEmpty(t, deviceData.Attributes) {
		assertAttrValue(t, deviceData.Attributes, "Temperature_Celsius", 31)
	}
}

func TestParseSmartForSataParentheticalRawValue(t *testing.T) {
	jsonPayload := []byte(`{
		"smartctl": {"exit_status": 0},
		"device": {"name": "/dev/sdz", "type": "sat"},
		"model_name": "Example",
		"serial_number": "PARENTHESES123",
		"firmware_version": "1.0",
		"user_capacity": {"bytes": 1024},
		"smart_status": {"passed": true},
		"temperature": {"current": 25},
		"ata_smart_attributes": {
			"table": [
				{
					"id": 9,
					"name": "Power_On_Hours",
					"value": 93,
					"worst": 55,
					"thresh": 0,
					"when_failed": "",
                    "raw": {
                        "value": 57891864217128,
                        "string": "39925 (212 206 0)"
                    }
				}
			]
		}
	}`)

	sm := &SmartManager{SmartDataMap: make(map[string]*smart.SmartData)}

	hasData, exitStatus := sm.parseSmartForSata(jsonPayload)
	require.True(t, hasData)
	assert.Equal(t, 0, exitStatus)

	data, ok := sm.SmartDataMap["PARENTHESES123"]
	require.True(t, ok)
	require.Len(t, data.Attributes, 1)

	attr := data.Attributes[0]
	assert.Equal(t, uint64(39925), attr.RawValue)
	assert.Equal(t, "39925 (212 206 0)", attr.RawString)
}

func TestParseSmartForNvme(t *testing.T) {
	fixturePath := filepath.Join("test-data", "smart", "nvme0.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	hasData, exitStatus := sm.parseSmartForNvme(data)
	require.True(t, hasData)
	assert.Equal(t, 0, exitStatus)

	deviceData, ok := sm.SmartDataMap["2024031600129"]
	require.True(t, ok, "expected smart data entry for serial 2024031600129")

	assert.Equal(t, "PELADN 512GB", deviceData.ModelName)
	assert.Equal(t, "VC2S038E", deviceData.FirmwareVersion)
	assert.Equal(t, "/dev/nvme0", deviceData.DiskName)
	assert.Equal(t, "nvme", deviceData.DiskType)
	assert.Equal(t, uint8(61), deviceData.Temperature)
	assert.Equal(t, "PASSED", deviceData.SmartStatus)
	assert.Equal(t, uint64(512110190592), deviceData.Capacity)
	if assert.NotEmpty(t, deviceData.Attributes) {
		assertAttrValue(t, deviceData.Attributes, "PercentageUsed", 0)
		assertAttrValue(t, deviceData.Attributes, "DataUnitsWritten", 16040567)
	}
}

func TestHasDataForDevice(t *testing.T) {
	sm := &SmartManager{
		SmartDataMap: map[string]*smart.SmartData{
			"serial-1": {DiskName: "/dev/sda"},
			"serial-2": nil,
		},
	}

	assert.True(t, sm.hasDataForDevice("/dev/sda"))
	assert.False(t, sm.hasDataForDevice("/dev/sdb"))
}

func TestDevicesSnapshotReturnsCopy(t *testing.T) {
	originalDevice := &DeviceInfo{Name: "/dev/sda"}
	sm := &SmartManager{
		SmartDevices: []*DeviceInfo{
			originalDevice,
			{Name: "/dev/sdb"},
		},
	}

	snapshot := sm.devicesSnapshot()
	require.Len(t, snapshot, 2)

	sm.SmartDevices[0] = &DeviceInfo{Name: "/dev/sdz"}
	assert.Equal(t, "/dev/sda", snapshot[0].Name)

	snapshot[1] = &DeviceInfo{Name: "/dev/nvme0"}
	assert.Equal(t, "/dev/sdb", sm.SmartDevices[1].Name)

	sm.SmartDevices = append(sm.SmartDevices, &DeviceInfo{Name: "/dev/nvme1"})
	assert.Len(t, snapshot, 2)
}

func TestScanDevicesWithEnvOverrideAndSeparator(t *testing.T) {
	t.Setenv("SMART_DEVICES_SEPARATOR", "|")
	t.Setenv("SMART_DEVICES", "/dev/sda:jmb39x-q,0|/dev/nvme0:nvme")

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	err := sm.ScanDevices(true)
	require.NoError(t, err)

	require.Len(t, sm.SmartDevices, 2)
	assert.Equal(t, "/dev/sda", sm.SmartDevices[0].Name)
	assert.Equal(t, "jmb39x-q,0", sm.SmartDevices[0].Type)
	assert.Equal(t, "/dev/nvme0", sm.SmartDevices[1].Name)
	assert.Equal(t, "nvme", sm.SmartDevices[1].Type)
}

func TestScanDevicesWithEnvOverride(t *testing.T) {
	t.Setenv("SMART_DEVICES", "/dev/sda:sat, /dev/nvme0:nvme")

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	err := sm.ScanDevices(true)
	require.NoError(t, err)

	require.Len(t, sm.SmartDevices, 2)
	assert.Equal(t, "/dev/sda", sm.SmartDevices[0].Name)
	assert.Equal(t, "sat", sm.SmartDevices[0].Type)
	assert.Equal(t, "/dev/nvme0", sm.SmartDevices[1].Name)
	assert.Equal(t, "nvme", sm.SmartDevices[1].Type)
}

func TestScanDevicesWithEnvOverrideInvalid(t *testing.T) {
	t.Setenv("SMART_DEVICES", ":sat")

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	err := sm.ScanDevices(true)
	require.Error(t, err)
}

func TestScanDevicesWithEnvOverrideEmpty(t *testing.T) {
	t.Setenv("SMART_DEVICES", "   ")

	sm := &SmartManager{
		SmartDataMap: make(map[string]*smart.SmartData),
	}

	err := sm.ScanDevices(true)
	assert.ErrorIs(t, err, errNoValidSmartData)
	assert.Empty(t, sm.SmartDevices)
}

func TestSmartctlArgsWithoutType(t *testing.T) {
	device := &DeviceInfo{Name: "/dev/sda"}

	sm := &SmartManager{}

	args := sm.smartctlArgs(device, true)
	assert.Equal(t, []string{"-a", "--json=c", "-n", "standby", "/dev/sda"}, args)
}

func TestSmartctlArgs(t *testing.T) {
	sm := &SmartManager{}

	sataDevice := &DeviceInfo{Name: "/dev/sda", Type: "sat"}
	assert.Equal(t,
		[]string{"-d", "sat", "-a", "--json=c", "-n", "standby", "/dev/sda"},
		sm.smartctlArgs(sataDevice, true),
	)

	assert.Equal(t,
		[]string{"-d", "sat", "-a", "--json=c", "/dev/sda"},
		sm.smartctlArgs(sataDevice, false),
	)

	assert.Equal(t,
		[]string{"-a", "--json=c", "-n", "standby"},
		sm.smartctlArgs(nil, true),
	)
}

func TestResolveRefreshError(t *testing.T) {
	scanErr := errors.New("scan failed")
	collectErr := errors.New("collect failed")

	tests := []struct {
		name        string
		devices     []*DeviceInfo
		data        map[string]*smart.SmartData
		scanErr     error
		collectErr  error
		expectedErr error
		expectNoErr bool
	}{
		{
			name:        "no devices returns scan error",
			devices:     nil,
			data:        make(map[string]*smart.SmartData),
			scanErr:     scanErr,
			expectedErr: scanErr,
		},
		{
			name:        "has data ignores errors",
			devices:     []*DeviceInfo{{Name: "/dev/sda"}},
			data:        map[string]*smart.SmartData{"serial": {}},
			scanErr:     scanErr,
			collectErr:  collectErr,
			expectNoErr: true,
		},
		{
			name:        "collect error preferred",
			devices:     []*DeviceInfo{{Name: "/dev/sda"}},
			data:        make(map[string]*smart.SmartData),
			collectErr:  collectErr,
			expectedErr: collectErr,
		},
		{
			name:        "scan error returned when no data",
			devices:     []*DeviceInfo{{Name: "/dev/sda"}},
			data:        make(map[string]*smart.SmartData),
			scanErr:     scanErr,
			expectedErr: scanErr,
		},
		{
			name:        "no errors returns sentinel",
			devices:     []*DeviceInfo{{Name: "/dev/sda"}},
			data:        make(map[string]*smart.SmartData),
			expectedErr: errNoValidSmartData,
		},
		{
			name:        "no devices collect error",
			devices:     nil,
			data:        make(map[string]*smart.SmartData),
			collectErr:  collectErr,
			expectedErr: collectErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &SmartManager{
				SmartDevices: tt.devices,
				SmartDataMap: tt.data,
			}

			err := sm.resolveRefreshError(tt.scanErr, tt.collectErr)
			if tt.expectNoErr {
				assert.NoError(t, err)
				return
			}

			if tt.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tt.expectedErr, err)
			}
		})
	}
}

func TestParseScan(t *testing.T) {
	sm := &SmartManager{
		SmartDataMap: map[string]*smart.SmartData{
			"serial-active": {DiskName: "/dev/sda"},
			"serial-stale":  {DiskName: "/dev/sdb"},
		},
	}

	scanJSON := []byte(`{
        "devices": [
            {"name": "/dev/sda", "type": "sat", "info_name": "/dev/sda [SAT]", "protocol": "ATA"},
            {"name": "/dev/nvme0", "type": "nvme", "info_name": "/dev/nvme0", "protocol": "NVMe"}
        ]
    }`)

	devices, hasData := sm.parseScan(scanJSON)
	assert.True(t, hasData)

	sm.updateSmartDevices(devices)

	require.Len(t, sm.SmartDevices, 2)
	assert.Equal(t, "/dev/sda", sm.SmartDevices[0].Name)
	assert.Equal(t, "sat", sm.SmartDevices[0].Type)
	assert.Equal(t, "/dev/nvme0", sm.SmartDevices[1].Name)
	assert.Equal(t, "nvme", sm.SmartDevices[1].Type)

	_, activeExists := sm.SmartDataMap["serial-active"]
	assert.True(t, activeExists, "active smart data should be preserved when device path remains")

	_, staleExists := sm.SmartDataMap["serial-stale"]
	assert.False(t, staleExists, "stale smart data entry should be removed when device path disappears")
}

func TestMergeDeviceListsPrefersConfigured(t *testing.T) {
	scanned := []*DeviceInfo{
		{Name: "/dev/sda", Type: "sat", InfoName: "scan-info", Protocol: "ATA"},
		{Name: "/dev/nvme0", Type: "nvme"},
	}

	configured := []*DeviceInfo{
		{Name: "/dev/sda", Type: "sat-override"},
		{Name: "/dev/sdb", Type: "sat"},
	}

	merged := mergeDeviceLists(nil, scanned, configured)
	require.Len(t, merged, 3)

	byName := make(map[string]*DeviceInfo, len(merged))
	for _, dev := range merged {
		byName[dev.Name] = dev
	}

	require.Contains(t, byName, "/dev/sda")
	assert.Equal(t, "sat-override", byName["/dev/sda"].Type, "configured type should override scanned type")
	assert.Equal(t, "scan-info", byName["/dev/sda"].InfoName, "scan metadata should be preserved when config does not provide it")

	require.Contains(t, byName, "/dev/nvme0")
	assert.Equal(t, "nvme", byName["/dev/nvme0"].Type)

	require.Contains(t, byName, "/dev/sdb")
	assert.Equal(t, "sat", byName["/dev/sdb"].Type)
}

func TestMergeDeviceListsPreservesVerification(t *testing.T) {
	existing := []*DeviceInfo{
		{Name: "/dev/sda", Type: "sat+megaraid", parserType: "sat", typeVerified: true},
	}

	scanned := []*DeviceInfo{
		{Name: "/dev/sda", Type: "nvme"},
	}

	merged := mergeDeviceLists(existing, scanned, nil)
	require.Len(t, merged, 1)

	device := merged[0]
	assert.True(t, device.typeVerified)
	assert.Equal(t, "sat", device.parserType)
	assert.Equal(t, "sat+megaraid", device.Type)
}

func TestMergeDeviceListsUpdatesTypeWhenUnverified(t *testing.T) {
	existing := []*DeviceInfo{
		{Name: "/dev/sda", Type: "sat", parserType: "sat", typeVerified: false},
	}

	scanned := []*DeviceInfo{
		{Name: "/dev/sda", Type: "nvme"},
	}

	merged := mergeDeviceLists(existing, scanned, nil)
	require.Len(t, merged, 1)

	device := merged[0]
	assert.False(t, device.typeVerified)
	assert.Equal(t, "nvme", device.Type)
	assert.Equal(t, "", device.parserType)
}

func TestParseSmartOutputMarksVerified(t *testing.T) {
	fixturePath := filepath.Join("test-data", "smart", "nvme0.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	sm := &SmartManager{SmartDataMap: make(map[string]*smart.SmartData)}
	device := &DeviceInfo{Name: "/dev/nvme0"}

	require.True(t, sm.parseSmartOutput(device, data))
	assert.Equal(t, "nvme", device.Type)
	assert.Equal(t, "nvme", device.parserType)
	assert.True(t, device.typeVerified)
}

func TestParseSmartOutputKeepsCustomType(t *testing.T) {
	fixturePath := filepath.Join("test-data", "smart", "sda.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	sm := &SmartManager{SmartDataMap: make(map[string]*smart.SmartData)}
	device := &DeviceInfo{Name: "/dev/sda", Type: "sat+megaraid"}

	require.True(t, sm.parseSmartOutput(device, data))
	assert.Equal(t, "sat+megaraid", device.Type)
	assert.Equal(t, "sat", device.parserType)
	assert.True(t, device.typeVerified)
}

func TestParseSmartOutputResetsVerificationOnFailure(t *testing.T) {
	sm := &SmartManager{SmartDataMap: make(map[string]*smart.SmartData)}
	device := &DeviceInfo{Name: "/dev/sda", Type: "sat", parserType: "sat", typeVerified: true}

	assert.False(t, sm.parseSmartOutput(device, []byte("not json")))
	assert.False(t, device.typeVerified)
	assert.Equal(t, "sat", device.parserType)
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
	sm := &SmartManager{}

	tests := []struct {
		name     string
		vendor   string
		product  string
		model    string
		expected bool
	}{
		{"regular drive", "SEAGATE", "ST1000DM003", "ST1000DM003-1CH162", false},
		{"qemu virtual", "QEMU", "QEMU HARDDISK", "QEMU HARDDISK", true},
		{"virtualbox virtual", "VBOX", "HARDDISK", "VBOX HARDDISK", true},
		{"vmware virtual", "VMWARE", "Virtual disk", "VMWARE Virtual disk", true},
		{"virtual in model", "ATA", "VIRTUAL", "VIRTUAL DISK", true},
		{"iet virtual", "IET", "VIRTUAL-DISK", "VIRTUAL-DISK", true},
		{"hyper-v virtual", "MSFT", "VIRTUAL HD", "VIRTUAL HD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &smart.SmartInfoForSata{
				ScsiVendor:  tt.vendor,
				ScsiProduct: tt.product,
				ModelName:   tt.model,
			}
			result := sm.isVirtualDevice(data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVirtualDeviceNvme(t *testing.T) {
	sm := &SmartManager{}

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"regular nvme", "Samsung SSD 970 EVO Plus 1TB", false},
		{"qemu virtual", "QEMU NVMe Ctrl", true},
		{"virtualbox virtual", "VBOX NVMe", true},
		{"vmware virtual", "VMWARE NVMe", true},
		{"virtual in model", "Virtual NVMe Device", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &smart.SmartInfoForNvme{
				ModelName: tt.model,
			}
			result := sm.isVirtualDeviceNvme(data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVirtualDeviceScsi(t *testing.T) {
	sm := &SmartManager{}

	tests := []struct {
		name     string
		vendor   string
		product  string
		model    string
		expected bool
	}{
		{"regular scsi", "SEAGATE", "ST1000DM003", "ST1000DM003-1CH162", false},
		{"qemu virtual", "QEMU", "QEMU HARDDISK", "QEMU HARDDISK", true},
		{"virtualbox virtual", "VBOX", "HARDDISK", "VBOX HARDDISK", true},
		{"vmware virtual", "VMWARE", "Virtual disk", "VMWARE Virtual disk", true},
		{"virtual in model", "ATA", "VIRTUAL", "VIRTUAL DISK", true},
		{"iet virtual", "IET", "VIRTUAL-DISK", "VIRTUAL-DISK", true},
		{"hyper-v virtual", "MSFT", "VIRTUAL HD", "VIRTUAL HD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &smart.SmartInfoForScsi{
				ScsiVendor:    tt.vendor,
				ScsiProduct:   tt.product,
				ScsiModelName: tt.model,
			}
			result := sm.isVirtualDeviceScsi(data)
			assert.Equal(t, tt.expected, result)
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
				os.Unsetenv("EXCLUDE_SMART")
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

func TestFilterExcludedDevices(t *testing.T) {
	tests := []struct {
		name           string
		excludedDevs   map[string]struct{}
		inputDevices   []*DeviceInfo
		expectedDevs   []*DeviceInfo
		expectedLength int
	}{
		{
			name:         "no exclusions",
			excludedDevs: map[string]struct{}{},
			inputDevices: []*DeviceInfo{
				{Name: "/dev/sda"},
				{Name: "/dev/sdb"},
				{Name: "/dev/nvme0"},
			},
			expectedDevs: []*DeviceInfo{
				{Name: "/dev/sda"},
				{Name: "/dev/sdb"},
				{Name: "/dev/nvme0"},
			},
			expectedLength: 3,
		},
		{
			name: "some devices excluded",
			excludedDevs: map[string]struct{}{
				"/dev/sda":   {},
				"/dev/nvme0": {},
			},
			inputDevices: []*DeviceInfo{
				{Name: "/dev/sda"},
				{Name: "/dev/sdb"},
				{Name: "/dev/nvme0"},
				{Name: "/dev/nvme1"},
			},
			expectedDevs: []*DeviceInfo{
				{Name: "/dev/sdb"},
				{Name: "/dev/nvme1"},
			},
			expectedLength: 2,
		},
		{
			name: "all devices excluded",
			excludedDevs: map[string]struct{}{
				"/dev/sda": {},
				"/dev/sdb": {},
			},
			inputDevices: []*DeviceInfo{
				{Name: "/dev/sda"},
				{Name: "/dev/sdb"},
			},
			expectedDevs:   []*DeviceInfo{},
			expectedLength: 0,
		},
		{
			name:           "nil devices",
			excludedDevs:   map[string]struct{}{},
			inputDevices:   nil,
			expectedDevs:   []*DeviceInfo{},
			expectedLength: 0,
		},
		{
			name: "filter nil and empty name devices",
			excludedDevs: map[string]struct{}{
				"/dev/sda": {},
			},
			inputDevices: []*DeviceInfo{
				{Name: "/dev/sda"},
				nil,
				{Name: ""},
				{Name: "/dev/sdb"},
			},
			expectedDevs: []*DeviceInfo{
				{Name: "/dev/sdb"},
			},
			expectedLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &SmartManager{
				excludedDevices: tt.excludedDevs,
			}

			result := sm.filterExcludedDevices(tt.inputDevices)

			assert.Len(t, result, tt.expectedLength)
			assert.Equal(t, tt.expectedDevs, result)
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
