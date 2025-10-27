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

	if deviceData.ModelName != "YADRO WUH721414AL4204" {
		t.Fatalf("unexpected model name: %s", deviceData.ModelName)
	}
	if deviceData.FirmwareVersion != "C240" {
		t.Fatalf("unexpected firmware version: %s", deviceData.FirmwareVersion)
	}
	if deviceData.DiskName != "/dev/sde" {
		t.Fatalf("unexpected disk name: %s", deviceData.DiskName)
	}
	if deviceData.DiskType != "scsi" {
		t.Fatalf("unexpected disk type: %s", deviceData.DiskType)
	}
	if deviceData.Temperature != 34 {
		t.Fatalf("unexpected temperature: %d", deviceData.Temperature)
	}
	if deviceData.SmartStatus != "PASSED" {
		t.Fatalf("unexpected SMART status: %s", deviceData.SmartStatus)
	}
	if deviceData.Capacity != 14000519643136 {
		t.Fatalf("unexpected capacity: %d", deviceData.Capacity)
	}

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

func TestSmartctlArgs(t *testing.T) {
	sm := &SmartManager{}

	sataDevice := &DeviceInfo{Name: "/dev/sda", Type: "sat"}
	assert.Equal(t,
		[]string{"-d", "sat", "-aj", "-n", "standby", "/dev/sda"},
		sm.smartctlArgs(sataDevice, true),
	)

	assert.Equal(t,
		[]string{"-d", "sat", "-aj", "/dev/sda"},
		sm.smartctlArgs(sataDevice, false),
	)

	assert.Equal(t,
		[]string{"-aj", "-n", "standby"},
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
			"/dev/sdb": {},
		},
	}

	scanJSON := []byte(`{
        "devices": [
            {"name": "/dev/sda", "type": "sat", "info_name": "/dev/sda [SAT]", "protocol": "ATA"},
            {"name": "/dev/nvme0", "type": "nvme", "info_name": "/dev/nvme0", "protocol": "NVMe"}
        ]
    }`)

	hasData := sm.parseScan(scanJSON)
	assert.True(t, hasData)

	require.Len(t, sm.SmartDevices, 2)
	assert.Equal(t, "/dev/sda", sm.SmartDevices[0].Name)
	assert.Equal(t, "sat", sm.SmartDevices[0].Type)
	assert.Equal(t, "/dev/nvme0", sm.SmartDevices[1].Name)
	assert.Equal(t, "nvme", sm.SmartDevices[1].Type)

	_, exists := sm.SmartDataMap["/dev/sdb"]
	assert.False(t, exists, "stale smart data entry should be removed")
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
