//go:build testing

package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePdhEngineInstance(t *testing.T) {
	tests := []struct {
		name       string
		instance   string
		wantLuid   string
		wantEngine string
		wantType   string
		wantOk     bool
	}{
		{
			name:       "3D engine",
			instance:   "pid_12708_luid_0x00000000_0x0000AB6D_phys_0_eng_0_engtype_3D",
			wantLuid:   "0x00000000_0x0000ab6d",
			wantEngine: "0_eng_0",
			wantType:   "3D",
			wantOk:     true,
		},
		{
			name:       "engine type with spaces",
			instance:   "pid_3588_luid_0x00000000_0x0000ab6d_phys_0_eng_7_engtype_Video Codec Engine",
			wantLuid:   "0x00000000_0x0000ab6d",
			wantEngine: "0_eng_7",
			wantType:   "Video Codec Engine",
			wantOk:     true,
		},
		{name: "missing engine type", instance: "pid_1_luid_0x00000000_0x0000ab6d_phys_0_eng_0", wantOk: false},
		{name: "empty engine type", instance: "pid_1_luid_0x00000000_0x0000ab6d_phys_0_eng_0_engtype_", wantOk: false},
		{name: "missing luid", instance: "pid_1_phys_0_eng_0_engtype_3D", wantOk: false},
		{name: "empty", instance: "", wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			luid, engine, engType, ok := parsePdhEngineInstance(tt.instance)
			assert.Equal(t, tt.wantOk, ok)
			if !tt.wantOk {
				return
			}
			assert.Equal(t, tt.wantLuid, luid)
			assert.Equal(t, tt.wantEngine, engine)
			assert.Equal(t, tt.wantType, engType)
		})
	}
}

func TestParsePdhMemoryInstance(t *testing.T) {
	luid, ok := parsePdhMemoryInstance("luid_0x00000000_0x0000AB6D_phys_0")
	assert.True(t, ok)
	assert.Equal(t, "0x00000000_0x0000ab6d", luid)

	luid, ok = parsePdhMemoryInstance("luid_0x00000000_0x0000de56_phys_0_part_0")
	assert.True(t, ok)
	assert.Equal(t, "0x00000000_0x0000de56", luid)

	_, ok = parsePdhMemoryInstance("_Total")
	assert.False(t, ok)
}

func TestUpdatePdhGpuData(t *testing.T) {
	const gib = 1 << 30
	adapters := []pdhAdapter{
		{id: "0", luid: "0x00000000_0x0000ab6d", name: "AMD Radeon RX 9070 XT", memoryTotal: 16 * gib},
		{id: "1", luid: "0x00000000_0x00001234", name: "Intel(R) UHD Graphics", memoryTotal: gib / 8},
	}

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}

	first := pdhSample{
		engines: map[string]float64{
			// two processes share 3D engine 0 on the AMD card
			"pid_100_luid_0x00000000_0x0000AB6D_phys_0_eng_0_engtype_3D":                 30,
			"pid_200_luid_0x00000000_0x0000AB6D_phys_0_eng_0_engtype_3D":                 20,
			"pid_300_luid_0x00000000_0x0000AB6D_phys_0_eng_7_engtype_Video Codec Engine": 10,
			// two copy engines; the busier one sets the copy usage
			"pid_100_luid_0x00000000_0x0000AB6D_phys_0_eng_1_engtype_Copy": 5,
			"pid_100_luid_0x00000000_0x0000AB6D_phys_0_eng_2_engtype_Copy": 15,
			// the Basic Render Driver is not a listed adapter
			"pid_100_luid_0x00000000_0x0000de56_phys_0_eng_0_engtype_3D": 99,
			"not a gpu engine": 50,
		},
		memory: map[string]float64{
			"luid_0x00000000_0x0000AB6D_phys_0":        4 * gib,
			"luid_0x00000000_0x00001234_phys_0":        gib / 16,
			"luid_0x00000000_0x0000de56_phys_0_part_0": 8192,
		},
	}
	gm.updatePdhGpuData(adapters, first)

	second := pdhSample{
		engines: map[string]float64{
			// a sum over 100 means overlapping process samples, so cap it
			"pid_100_luid_0x00000000_0x0000AB6D_phys_0_eng_0_engtype_3D": 80,
			"pid_200_luid_0x00000000_0x0000AB6D_phys_0_eng_0_engtype_3D": 40,
			"pid_400_luid_0x00000000_0x00001234_phys_0_eng_0_engtype_3D": 12,
		},
		memory: map[string]float64{
			"luid_0x00000000_0x0000AB6D_phys_0": 6 * gib,
			"luid_0x00000000_0x00001234_phys_0": gib / 16,
		},
	}
	gm.updatePdhGpuData(adapters, second)

	data := gm.GetCurrentData(0)
	require.Len(t, data, 2)

	amd := data["0"]
	assert.Equal(t, "AMD Radeon RX 9070 XT", amd.Name)
	assert.Equal(t, map[string]float64{
		"3D":                 75, // (50 + 100) / 2
		"Copy":               7.5,
		"Video Codec Engine": 5,
	}, amd.Engines)
	assert.Equal(t, 75.0, amd.Usage)
	assert.Equal(t, 6144.0, amd.MemoryUsed) // MiB
	assert.Equal(t, 16384.0, amd.MemoryTotal)

	intel := data["1"]
	assert.Equal(t, "Intel(R) UHD Graphics", intel.Name)
	assert.Equal(t, map[string]float64{"3D": 6}, intel.Engines)
	assert.Equal(t, 6.0, intel.Usage)
}

func TestUpdatePdhGpuDataWithoutAdapters(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	gm.updatePdhGpuData(nil, pdhSample{})
	assert.Empty(t, gm.GpuDataMap)
}

func TestUpdatePdhGpuDataIdleAdapter(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	adapters := []pdhAdapter{{id: "0", luid: "0x00000000_0x0000ab6d", name: "GPU", memoryTotal: 1 << 30}}

	// no process uses the GPU and the memory counter has no instance for it
	gm.updatePdhGpuData(adapters, pdhSample{})

	data := gm.GetCurrentData(0)
	gpu := data["0"]
	assert.Equal(t, "GPU", gpu.Name)
	assert.Equal(t, 0.0, gpu.Usage)
	assert.Equal(t, 0.0, gpu.MemoryUsed)
	assert.NotZero(t, gpu.MemoryTotal)
}

func TestPdhCollectorPriority(t *testing.T) {
	gm := &GPUManager{}

	assert.Equal(t, []collectorSource{collectorSourcePDH},
		gm.resolveLegacyCollectorPriority(gpuCapabilities{hasPdh: true}))

	// pdh runs after nvidia-smi and leaves the NVIDIA cards to it
	assert.Equal(t, []collectorSource{collectorSourceNvidiaSMI, collectorSourcePDH},
		gm.resolveLegacyCollectorPriority(gpuCapabilities{hasPdh: true, hasNvidiaSmi: true}))

	assert.True(t, hasAnyGpuCollector(gpuCapabilities{hasPdh: true}))
	assert.Equal(t, []collectorSource{collectorSourcePDH}, parseCollectorPriority("PDH"))
}

func TestPdhUsageIgnoresVideoAndCopyEngines(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	adapters := []pdhAdapter{{id: "0", luid: "0x00000000_0x0000ab6d", name: "GPU"}}

	// screen capture keeps the video encoder busy while the desktop is idle
	gm.updatePdhGpuData(adapters, pdhSample{engines: map[string]float64{
		"pid_1_luid_0x00000000_0x0000ab6d_phys_0_eng_0_engtype_3D":                 4,
		"pid_2_luid_0x00000000_0x0000ab6d_phys_0_eng_7_engtype_Video Codec Engine": 30,
		"pid_2_luid_0x00000000_0x0000ab6d_phys_0_eng_1_engtype_Copy":               20,
	}})
	// compute work counts toward usage like 3D work
	gm.updatePdhGpuData(adapters, pdhSample{engines: map[string]float64{
		"pid_1_luid_0x00000000_0x0000ab6d_phys_0_eng_0_engtype_3D":        4,
		"pid_3_luid_0x00000000_0x0000ab6d_phys_0_eng_3_engtype_Compute 0": 50,
	}})

	gpu := gm.GetCurrentData(0)["0"]
	assert.Equal(t, 27.0, gpu.Usage) // (4 + 50) / 2
	assert.Equal(t, 15.0, gpu.Engines["Video Codec Engine"])
	assert.Equal(t, 25.0, gpu.Engines["Compute 0"])

	// the next period averages only its own samples
	gm.updatePdhGpuData(adapters, pdhSample{engines: map[string]float64{
		"pid_1_luid_0x00000000_0x0000ab6d_phys_0_eng_0_engtype_3D": 10,
	}})
	assert.Equal(t, 10.0, gm.GetCurrentData(0)["0"].Usage)
}

func TestPdhEngineCountsTowardUsage(t *testing.T) {
	for _, engType := range []string{"3D", "Compute 0", "Compute_1", "Graphics_1", "Cuda", "High Priority 3D", "High Priority Compute"} {
		assert.True(t, pdhEngineCountsTowardUsage(engType), engType)
	}
	for _, engType := range []string{"Copy", "Video Codec Engine", "VideoDecode", "Video JPEG 0", "Security 1", "Timer 0"} {
		assert.False(t, pdhEngineCountsTowardUsage(engType), engType)
	}
}

func TestAssignPdhAdapterIDs(t *testing.T) {
	adapters := []pdhAdapter{
		{vendorID: 0x1002, deviceID: 0x7550},
		{vendorID: 0x8086, deviceID: 0x9bc4},
		{vendorID: 0x1002, deviceID: 0x7550},
	}
	assignPdhAdapterIDs(adapters)
	assert.Equal(t, "p1002-7550", adapters[0].id)
	assert.Equal(t, "p8086-9bc4", adapters[1].id)
	assert.Equal(t, "p1002-7550-2", adapters[2].id)
}

func TestPdhSkipsNvidiaCardsWhenNvidiaCollectorRuns(t *testing.T) {
	adapters := []pdhAdapter{
		{id: "p10de-2504", vendorID: nvidiaVendorID, luid: "0x00000000_0x00000001", name: "NVIDIA GeForce RTX 3060"},
		{id: "p1002-7550", vendorID: 0x1002, luid: "0x00000000_0x00000002", name: "AMD Radeon RX 9070 XT"},
	}

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	gm.updatePdhGpuData(adapters, pdhSample{})
	assert.Len(t, gm.GpuDataMap, 2)

	gm = &GPUManager{GpuDataMap: make(map[string]*system.GPUData), nvidiaCollectorStarted: true}
	gm.updatePdhGpuData(adapters, pdhSample{})
	assert.Len(t, gm.GpuDataMap, 1)
	assert.Contains(t, gm.GpuDataMap, "p1002-7550")
}

func TestStartingNvidiaCollectorIsRecorded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("BESZEL_AGENT_GPU_COLLECTOR", "nvidia-smi")
	gpuCommandFixture(t, dir, "nvidia-smi", "0, NVIDIA GeForce RTX 3050 Ti Laptop GPU, 48, 12, 4096, 26.3, 12.73\n")

	gm, err := NewGPUManager()
	require.NoError(t, err)
	gm.Lock()
	defer gm.Unlock()
	assert.True(t, gm.nvidiaCollectorStarted)
}

func TestPdhAdapterListChanges(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	amd := pdhAdapter{id: "p1002-7550", luid: "0x00000000_0x0000ab6d", name: "AMD Radeon RX 9070 XT"}
	intel := pdhAdapter{id: "p8086-9bc4", luid: "0x00000000_0x00001234", name: "Intel(R) UHD Graphics"}
	busy := func(luid string, usage float64) pdhSample {
		return pdhSample{engines: map[string]float64{"pid_1_luid_" + luid + "_phys_0_eng_0_engtype_3D": usage}}
	}

	gm.updatePdhGpuData([]pdhAdapter{amd, intel}, busy(amd.luid, 40))
	require.Len(t, gm.GetCurrentData(0), 2)

	// a driver update removes the AMD card, and the Intel driver renames its adapter
	intel.name = "Intel(R) UHD Graphics 630"
	gm.updatePdhGpuData([]pdhAdapter{intel}, busy(intel.luid, 10))
	data := gm.GetCurrentData(0)
	require.Len(t, data, 1, "a removed adapter must not keep reporting its last value")
	assert.Equal(t, "Intel(R) UHD Graphics 630", data["p8086-9bc4"].Name)
	assert.Equal(t, 10.0, data["p8086-9bc4"].Usage)

	// the AMD card returns with a new LUID and starts from fresh averages
	amd.luid = "0x00000000_0x0000cd01"
	gm.updatePdhGpuData([]pdhAdapter{amd, intel}, busy(amd.luid, 30))
	data = gm.GetCurrentData(0)
	require.Len(t, data, 2)
	assert.Equal(t, 30.0, data["p1002-7550"].Usage)
	assert.Equal(t, 0.0, data["p8086-9bc4"].Usage)
}

func TestDropPdhGpus(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	adapters := []pdhAdapter{{id: "p1002-7550", luid: "0x00000000_0x0000ab6d", name: "GPU"}}
	gm.updatePdhGpuData(adapters, pdhSample{})
	gm.GpuDataMap["0"] = &system.GPUData{Name: "NVIDIA GPU", Count: 1}
	gm.GetCurrentData(0)

	// when the counters can't be read, pdh GPUs drop out instead of repeating stale values
	gm.dropPdhGpus()
	data := gm.GetCurrentData(0)
	assert.NotContains(t, data, "p1002-7550")
	assert.Contains(t, data, "0", "GPUs from other collectors stay")
}

func TestPdhUsageAcrossCacheKeys(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	adapters := []pdhAdapter{{id: "p1002-7550", luid: "0x00000000_0x0000ab6d", name: "GPU"}}
	sample := func(usage float64) pdhSample {
		return pdhSample{engines: map[string]float64{
			"pid_1_luid_0x00000000_0x0000ab6d_phys_0_eng_0_engtype_3D":                 usage,
			"pid_2_luid_0x00000000_0x0000ab6d_phys_0_eng_7_engtype_Video Codec Engine": 90,
		}}
	}

	gm.updatePdhGpuData(adapters, sample(10))
	gm.updatePdhGpuData(adapters, sample(30))
	assert.Equal(t, 20.0, gm.GetCurrentData(1)["p1002-7550"].Usage)

	gm.updatePdhGpuData(adapters, sample(50))
	assert.Equal(t, 50.0, gm.GetCurrentData(1)["p1002-7550"].Usage)
	// a second cache key averages every sample since it was last read
	assert.Equal(t, 30.0, gm.GetCurrentData(2)["p1002-7550"].Usage)
}
