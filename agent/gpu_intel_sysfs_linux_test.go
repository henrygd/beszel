//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntelSysfsTest(t *testing.T) (root, cardPath, hwmonPath string) {
	t.Helper()
	root = t.TempDir()
	oldRoot := drmSysfsRoot
	drmSysfsRoot = root
	t.Cleanup(func() {
		drmSysfsRoot = oldRoot
	})

	cardPath = filepath.Join(root, "card0")
	devicePath := filepath.Join(cardPath, "device")
	hwmonPath = filepath.Join(devicePath, "hwmon", "hwmon0")
	require.NoError(t, os.MkdirAll(hwmonPath, 0o755))
	return root, cardPath, hwmonPath
}

func writeIntelSysfsFile(t *testing.T, basePath, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(basePath, name), []byte(content), 0o644))
}

func setIntelSysfsTime(t *testing.T, now time.Time) {
	t.Helper()
	oldNow := intelSysfsNow
	intelSysfsNow = func() time.Time { return now }
	t.Cleanup(func() {
		intelSysfsNow = oldNow
	})
}

func TestIntelSysfsDetectsIntelCardWithEnergy(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "name", "xe\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")

	gm := &GPUManager{}
	assert.True(t, gm.hasIntelSysfs())

	cards, err := discoverIntelSysfsCards()
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, cardPath, cards[0].cardPath)
	assert.Equal(t, hwmonPath, cards[0].hwmonDir)
}

func TestIntelSysfsRejectsNonIntelCard(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x1002\n")
	writeIntelSysfsFile(t, hwmonPath, "name", "xe\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")

	gm := &GPUManager{}
	assert.False(t, gm.hasIntelSysfs())
}

func TestIntelSysfsRequiresEnergyInput(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "name", "xe\n")

	gm := &GPUManager{}
	assert.False(t, gm.hasIntelSysfs())
}

func TestIntelSysfsFirstSampleInitializesWithoutBogusPower(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "name", "xe\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")
	setIntelSysfsTime(t, time.Unix(100, 0))

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	ok := gm.updateIntelSysfsGpuData(cardPath, hwmonPath)
	require.True(t, ok)

	gpu := gm.GpuDataMap["card0"]
	require.NotNil(t, gpu)
	assert.Equal(t, "Intel GPU card0", gpu.Name)
	assert.Equal(t, 0.0, gpu.Power)
	assert.Equal(t, 1.0, gpu.Count)
}

func TestIntelSysfsSecondSampleComputesWatts(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	oldNow := intelSysfsNow
	intelSysfsNow = func() time.Time { return time.Unix(100, 0) }
	t.Cleanup(func() { intelSysfsNow = oldNow })
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "6000000\n")
	intelSysfsNow = func() time.Time { return time.Unix(102, 0) }
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	gpu := gm.GpuDataMap["card0"]
	require.NotNil(t, gpu)
	assert.Equal(t, 2.5, gpu.Power)
	assert.Equal(t, 2.0, gpu.Count)
}

func TestIntelSysfsSecondEnergyCounterMapsToPowerPkg(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")
	writeIntelSysfsFile(t, hwmonPath, "energy2_input", "2000000\n")

	oldNow := intelSysfsNow
	t.Cleanup(func() { intelSysfsNow = oldNow })
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	intelSysfsNow = func() time.Time { return time.Unix(100, 0) }
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "2000000\n")
	writeIntelSysfsFile(t, hwmonPath, "energy2_input", "8000000\n")
	intelSysfsNow = func() time.Time { return time.Unix(102, 0) }
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	gpu := gm.GpuDataMap["card0"]
	require.NotNil(t, gpu)
	assert.Equal(t, 0.5, gpu.Power)
	assert.Equal(t, 3.0, gpu.PowerPkg)
}

func TestIntelSysfsCounterResetSkipsOneSample(t *testing.T) {
	gm := &GPUManager{}
	power, ok := gm.calculateIntelSysfsPower("card0", 5000000, time.Unix(100, 0))
	assert.False(t, ok)
	assert.Equal(t, 0.0, power)

	power, ok = gm.calculateIntelSysfsPower("card0", 1000000, time.Unix(101, 0))
	assert.False(t, ok)
	assert.Equal(t, 0.0, power)

	power, ok = gm.calculateIntelSysfsPower("card0", 3000000, time.Unix(103, 0))
	assert.True(t, ok)
	assert.Equal(t, 1.0, power)
}

func TestIntelSysfsTempInputMapsToCelsius(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")
	writeIntelSysfsFile(t, hwmonPath, "temp1_input", "43500\n")
	setIntelSysfsTime(t, time.Unix(100, 0))

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	gpu := gm.GpuDataMap["card0"]
	require.NotNil(t, gpu)
	assert.Equal(t, 43.5, gpu.Temperature)
}

func TestIntelSysfsMissingOptionalFilesDoNotFail(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")
	setIntelSysfsTime(t, time.Unix(100, 0))

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	gpu := gm.GpuDataMap["card0"]
	require.NotNil(t, gpu)
	assert.Equal(t, 0.0, gpu.Usage)
	assert.Equal(t, 0.0, gpu.MemoryUsed)
	assert.Equal(t, 0.0, gpu.MemoryTotal)
	assert.Equal(t, 0.0, gpu.Temperature)
}

func TestIntelSysfsMapsOpportunisticMemoryAndUsage(t *testing.T) {
	_, cardPath, hwmonPath := setupIntelSysfsTest(t)
	devicePath := filepath.Join(cardPath, "device")
	writeIntelSysfsFile(t, devicePath, "vendor", "0x8086\n")
	writeIntelSysfsFile(t, devicePath, "gpu_busy_percent", "37\n")
	writeIntelSysfsFile(t, devicePath, "mem_info_lmem_used", "1073741824\n")
	writeIntelSysfsFile(t, devicePath, "mem_info_lmem_total", "2147483648\n")
	writeIntelSysfsFile(t, hwmonPath, "energy1_input", "1000000\n")
	setIntelSysfsTime(t, time.Unix(100, 0))

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	require.True(t, gm.updateIntelSysfsGpuData(cardPath, hwmonPath))

	gpu := gm.GpuDataMap["card0"]
	require.NotNil(t, gpu)
	assert.Equal(t, 37.0, gpu.Usage)
	assert.Equal(t, utils.BytesToMegabytes(1073741824), gpu.MemoryUsed)
	assert.Equal(t, utils.BytesToMegabytes(2147483648), gpu.MemoryTotal)
}
