//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHexID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0x1002", "1002"},
		{"C2", "c2"},
		{"  15BF  ", "15bf"},
		{"0x15bf", "15bf"},
		{"", ""},
	}
	for _, tt := range tests {
		subName := tt.in
		if subName == "" {
			subName = "empty_string"
		}
		t.Run(subName, func(t *testing.T) {
			got := normalizeHexID(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCacheKeyForAmdgpu(t *testing.T) {
	tests := []struct {
		deviceID   string
		revisionID string
		want       string
	}{
		{"1114", "c2", "1114:c2"},
		{"15bf", "", "15bf"},
		{"1506", "c1", "1506:c1"},
	}
	for _, tt := range tests {
		got := cacheKeyForAmdgpu(tt.deviceID, tt.revisionID)
		assert.Equal(t, tt.want, got)
	}
}

func TestReadSysfsFloat(t *testing.T) {
	dir := t.TempDir()

	validPath := filepath.Join(dir, "val")
	require.NoError(t, os.WriteFile(validPath, []byte("  42.5  \n"), 0o644))
	got, err := readSysfsFloat(validPath)
	require.NoError(t, err)
	assert.Equal(t, 42.5, got)

	// Integer and scientific
	sciPath := filepath.Join(dir, "sci")
	require.NoError(t, os.WriteFile(sciPath, []byte("1e2"), 0o644))
	got, err = readSysfsFloat(sciPath)
	require.NoError(t, err)
	assert.Equal(t, 100.0, got)

	// Missing file
	_, err = readSysfsFloat(filepath.Join(dir, "missing"))
	require.Error(t, err)

	// Invalid content
	badPath := filepath.Join(dir, "bad")
	require.NoError(t, os.WriteFile(badPath, []byte("not a number"), 0o644))
	_, err = readSysfsFloat(badPath)
	require.Error(t, err)
}

func TestIsAmdGpu(t *testing.T) {
	dir := t.TempDir()
	deviceDir := filepath.Join(dir, "device")
	require.NoError(t, os.MkdirAll(deviceDir, 0o755))

	// AMD vendor 0x1002 -> true
	require.NoError(t, os.WriteFile(filepath.Join(deviceDir, "vendor"), []byte("0x1002\n"), 0o644))
	assert.True(t, isAmdGpu(dir), "vendor 0x1002 should be AMD")

	// Non-AMD vendor -> false
	require.NoError(t, os.WriteFile(filepath.Join(deviceDir, "vendor"), []byte("0x10de\n"), 0o644))
	assert.False(t, isAmdGpu(dir), "vendor 0x10de should not be AMD")

	// Missing vendor file -> false
	require.NoError(t, os.Remove(filepath.Join(deviceDir, "vendor")))
	assert.False(t, isAmdGpu(dir), "missing vendor file should be false")
}

func TestAmdgpuNameCacheRoundTrip(t *testing.T) {
	// Cache a name and retrieve it (unique key to avoid affecting other tests)
	deviceID, revisionID := "cachedev99", "00"
	cacheAmdgpuName(deviceID, revisionID, "AMD Test GPU 99 Graphics", true)

	name, found, done := getCachedAmdgpuName(deviceID, revisionID)
	assert.True(t, found)
	assert.True(t, done)
	assert.Equal(t, "AMD Test GPU 99", name)

	// Device-only key also stored
	name2, found2, _ := getCachedAmdgpuName(deviceID, "")
	assert.True(t, found2)
	assert.Equal(t, "AMD Test GPU 99", name2)

	// Cache a miss
	cacheMissingAmdgpuName("missedev99", "ab")
	_, found3, done3 := getCachedAmdgpuName("missedev99", "ab")
	assert.False(t, found3)
	assert.True(t, done3, "done should be true so caller skips file lookup")
}

func TestUpdateAmdGpuDataWithFakeSysfs(t *testing.T) {
	dir := t.TempDir()
	cardPath := filepath.Join(dir, "card0")
	devicePath := filepath.Join(cardPath, "device")
	hwmonPath := filepath.Join(devicePath, "hwmon", "hwmon0")
	require.NoError(t, os.MkdirAll(hwmonPath, 0o755))

	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(devicePath, name), []byte(content), 0o644))
	}
	write("vendor", "0x1002")
	write("device", "0x1506")
	write("revision", "0xc1")
	write("gpu_busy_percent", "25")
	write("mem_info_vram_used", "1073741824")
	write("mem_info_vram_total", "2147483648")
	require.NoError(t, os.WriteFile(filepath.Join(hwmonPath, "temp1_input"), []byte("45000"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hwmonPath, "power1_input"), []byte("20000000"), 0o644))

	// Pre-cache name so getAmdGpuName returns a known value (it uses system amdgpu.ids path)
	cacheAmdgpuName("1506", "c1", "AMD Radeon 610M Graphics", true)

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	ok := gm.updateAmdGpuData(cardPath)
	require.True(t, ok)

	gpu, ok := gm.GpuDataMap["card0"]
	require.True(t, ok)
	assert.Equal(t, "AMD Radeon 610M", gpu.Name)
	assert.Equal(t, 25.0, gpu.Usage)
	assert.Equal(t, bytesToMegabytes(1073741824), gpu.MemoryUsed)
	assert.Equal(t, bytesToMegabytes(2147483648), gpu.MemoryTotal)
	assert.Equal(t, 45.0, gpu.Temperature)
	assert.Equal(t, 20.0, gpu.Power)
	assert.Equal(t, 1.0, gpu.Count)
}

func TestLookupAmdgpuNameInFile(t *testing.T) {
	idsPath := filepath.Join("test-data", "amdgpu.ids")

	tests := []struct {
		name       string
		deviceID   string
		revisionID string
		wantName   string
		wantExact  bool
		wantFound  bool
	}{
		{
			name:       "exact device and revision match",
			deviceID:   "1114",
			revisionID: "c2",
			wantName:   "AMD Radeon 860M Graphics",
			wantExact:  true,
			wantFound:  true,
		},
		{
			name:       "exact match 15BF revision 01 returns 760M",
			deviceID:   "15bf",
			revisionID: "01",
			wantName:   "AMD Radeon 760M Graphics",
			wantExact:  true,
			wantFound:  true,
		},
		{
			name:       "exact match 15BF revision 00 returns 780M",
			deviceID:   "15bf",
			revisionID: "00",
			wantName:   "AMD Radeon 780M Graphics",
			wantExact:  true,
			wantFound:  true,
		},
		{
			name:       "device-only match returns first entry for device",
			deviceID:   "1506",
			revisionID: "",
			wantName:   "AMD Radeon 610M",
			wantExact:  false,
			wantFound:  true,
		},
		{
			name:       "unknown device not found",
			deviceID:   "dead",
			revisionID: "00",
			wantName:   "",
			wantExact:  false,
			wantFound:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotExact, gotFound := lookupAmdgpuNameInFile(tt.deviceID, tt.revisionID, idsPath)
			assert.Equal(t, tt.wantName, gotName, "name")
			assert.Equal(t, tt.wantExact, gotExact, "exact")
			assert.Equal(t, tt.wantFound, gotFound, "found")
		})
	}
}

func TestGetAmdGpuNameFromIdsFile(t *testing.T) {
	// Test that getAmdGpuName resolves a name when we can't inject the ids path.
	// We only verify behavior when product_name is missing and device/revision
	// would be read from sysfs; the actual lookup uses /usr/share/libdrm/amdgpu.ids.
	// So this test focuses on normalizeAmdgpuName and that lookupAmdgpuNameInFile
	// returns the expected name for our test-data file.
	idsPath := filepath.Join("test-data", "amdgpu.ids")
	name, exact, found := lookupAmdgpuNameInFile("1435", "ae", idsPath)
	require.True(t, found)
	require.True(t, exact)
	assert.Equal(t, "AMD Custom GPU 0932", name)
	assert.Equal(t, "AMD Custom GPU 0932", normalizeAmdgpuName(name))

	// " Graphics" suffix is trimmed by normalizeAmdgpuName
	name2 := "AMD Radeon 860M Graphics"
	assert.Equal(t, "AMD Radeon 860M", normalizeAmdgpuName(name2))
}
