//go:build testing
// +build testing

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNvidiaData(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantData  map[string]system.GPUData
		wantValid bool
	}{
		{
			name:  "valid multi-gpu data",
			input: "0, NVIDIA GeForce RTX 3050 Ti Laptop GPU, 48, 12, 4096, 26.3, 12.73\n1, NVIDIA A100-PCIE-40GB, 38, 74, 40960, [N/A], 36.79",
			wantData: map[string]system.GPUData{
				"0": {
					Name:        "GeForce RTX 3050 Ti",
					Temperature: 48.0,
					MemoryUsed:  12.0 / 1.024,
					MemoryTotal: 4096.0 / 1.024,
					Usage:       26.3,
					Power:       12.73,
					Count:       1,
				},
				"1": {
					Name:        "A100-PCIE-40GB",
					Temperature: 38.0,
					MemoryUsed:  74.0 / 1.024,
					MemoryTotal: 40960.0 / 1.024,
					Usage:       0.0,
					Power:       36.79,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name: "more valid multi-gpu data",
			input: `0, NVIDIA A10, 45, 19676, 23028, 0, 58.98
1, NVIDIA A10, 45, 19638, 23028, 0, 62.35
2, NVIDIA A10, 44, 21700, 23028, 0, 59.57
3, NVIDIA A10, 45, 18222, 23028, 0, 61.76`,
			wantData: map[string]system.GPUData{
				"0": {
					Name:        "A10",
					Temperature: 45.0,
					MemoryUsed:  19676.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       58.98,
					Count:       1,
				},
				"1": {
					Name:        "A10",
					Temperature: 45.0,
					MemoryUsed:  19638.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       62.35,
					Count:       1,
				},
				"2": {
					Name:        "A10",
					Temperature: 44.0,
					MemoryUsed:  21700.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       59.57,
					Count:       1,
				},
				"3": {
					Name:        "A10",
					Temperature: 45.0,
					MemoryUsed:  18222.0 / 1.024,
					MemoryTotal: 23028.0 / 1.024,
					Usage:       0.0,
					Power:       61.76,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name:      "empty input",
			input:     "",
			wantData:  map[string]system.GPUData{},
			wantValid: false,
		},
		{
			name:      "malformed data",
			input:     "bad, data, here",
			wantData:  map[string]system.GPUData{},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{
				GpuDataMap: make(map[string]*system.GPUData),
			}
			valid := gm.parseNvidiaData([]byte(tt.input))
			assert.Equal(t, tt.wantValid, valid)

			if tt.wantValid {
				for id, want := range tt.wantData {
					got := gm.GpuDataMap[id]
					require.NotNil(t, got)
					assert.Equal(t, want.Name, got.Name)
					assert.InDelta(t, want.Temperature, got.Temperature, 0.01)
					assert.InDelta(t, want.MemoryUsed, got.MemoryUsed, 0.01)
					assert.InDelta(t, want.MemoryTotal, got.MemoryTotal, 0.01)
					assert.InDelta(t, want.Usage, got.Usage, 0.01)
					assert.InDelta(t, want.Power, got.Power, 0.01)
					assert.Equal(t, want.Count, got.Count)
				}
			}
		})
	}
}

func TestParseAmdData(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantData  map[string]system.GPUData
		wantValid bool
	}{
		{
			name: "valid single gpu data",
			input: `{
				"card0": {
					"GUID": "34756",
					"Temperature (Sensor edge) (C)": "47.0",
					"Current Socket Graphics Package Power (W)": "9.215",
					"GPU use (%)": "0",
					"VRAM Total Memory (B)": "536870912",
					"VRAM Total Used Memory (B)": "482263040",
					"Card Series": "Rembrandt [Radeon 680M]"
				}
			}`,
			wantData: map[string]system.GPUData{
				"34756": {
					Name:        "Rembrandt [Radeon 680M]",
					Temperature: 47.0,
					MemoryUsed:  482263040.0 / (1024 * 1024),
					MemoryTotal: 536870912.0 / (1024 * 1024),
					Usage:       0.0,
					Power:       9.215,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name: "valid multi gpu data",
			input: `{
				"card0": {
					"GUID": "34756",
					"Temperature (Sensor edge) (C)": "47.0",
					"Current Socket Graphics Package Power (W)": "9.215",
					"GPU use (%)": "0",
					"VRAM Total Memory (B)": "536870912",
					"VRAM Total Used Memory (B)": "482263040",
					"Card Series": "Rembrandt [Radeon 680M]"
				},
				"card1": {
					"GUID": "38294",
					"Temperature (Sensor edge) (C)": "49.0",
					"Temperature (Sensor junction) (C)": "49.0",
					"Temperature (Sensor memory) (C)": "62.0",
					"Average Graphics Package Power (W)": "19.0",
					"GPU use (%)": "20.3",
					"VRAM Total Memory (B)": "25753026560",
					"VRAM Total Used Memory (B)": "794341376",
					"Card Series": "Navi 31 [Radeon RX 7900 XT]"
				}
			}`,
			wantData: map[string]system.GPUData{
				"34756": {
					Name:        "Rembrandt [Radeon 680M]",
					Temperature: 47.0,
					MemoryUsed:  482263040.0 / (1024 * 1024),
					MemoryTotal: 536870912.0 / (1024 * 1024),
					Usage:       0.0,
					Power:       9.215,
					Count:       1,
				},
				"38294": {
					Name:        "Navi 31 [Radeon RX 7900 XT]",
					Temperature: 49.0,
					MemoryUsed:  794341376.0 / (1024 * 1024),
					MemoryTotal: 25753026560.0 / (1024 * 1024),
					Usage:       20.3,
					Power:       19.0,
					Count:       1,
				},
			},
			wantValid: true,
		},
		{
			name:  "invalid json",
			input: "{bad json",
		},
		{
			name:      "invalid json",
			input:     "{bad json",
			wantData:  map[string]system.GPUData{},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{
				GpuDataMap: make(map[string]*system.GPUData),
			}
			valid := gm.parseAmdData([]byte(tt.input))
			assert.Equal(t, tt.wantValid, valid)

			if tt.wantValid {
				for id, want := range tt.wantData {
					got := gm.GpuDataMap[id]
					require.NotNil(t, got)
					assert.Equal(t, want.Name, got.Name)
					assert.InDelta(t, want.Temperature, got.Temperature, 0.01)
					assert.InDelta(t, want.MemoryUsed, got.MemoryUsed, 0.01)
					assert.InDelta(t, want.MemoryTotal, got.MemoryTotal, 0.01)
					assert.InDelta(t, want.Usage, got.Usage, 0.01)
					assert.InDelta(t, want.Power, got.Power, 0.01)
					assert.Equal(t, want.Count, got.Count)
				}
			}
		})
	}
}

func TestParseJetsonData(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMetrics *system.GPUData
	}{
		{
			name:  "valid data",
			input: "11-14-2024 22:54:33 RAM 4300/30698MB GR3D_FREQ 45% tj@52.468C VDD_GPU_SOC 2171mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  4300.0,
				MemoryTotal: 30698.0,
				Usage:       45.0,
				Temperature: 52.468,
				Power:       2.171,
				Count:       1,
			},
		},
		{
			name:  "more valid data",
			input: "11-15-2024 08:38:09 RAM 6185/7620MB (lfb 8x2MB) SWAP 851/3810MB (cached 1MB) CPU [15%@729,11%@729,14%@729,13%@729,11%@729,8%@729] EMC_FREQ 43%@2133 GR3D_FREQ 63%@[621] NVDEC off NVJPG off NVJPG1 off VIC off OFA off APE 200 cpu@53.968C soc2@52.437C soc0@50.75C gpu@53.343C tj@53.968C soc1@51.656C VDD_IN 12479mW/12479mW VDD_CPU_GPU_CV 4667mW/4667mW VDD_SOC 2817mW/2817mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  6185.0,
				MemoryTotal: 7620.0,
				Usage:       63.0,
				Temperature: 53.968,
				Power:       4.667,
				Count:       1,
			},
		},
		{
			name:  "orin nano",
			input: "06-18-2025 11:25:24 RAM 3452/7620MB (lfb 25x4MB) SWAP 1518/16384MB (cached 174MB) CPU [1%@1420,2%@1420,0%@1420,2%@1420,2%@729,1%@729] GR3D_FREQ 0% cpu@50.031C soc2@49.031C soc0@50C gpu@49.031C tj@50.25C soc1@50.25C VDD_IN 4824mW/4824mW VDD_CPU_GPU_CV 518mW/518mW VDD_SOC 1475mW/1475mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  3452.0,
				MemoryTotal: 7620.0,
				Usage:       0.0,
				Temperature: 50.25,
				Power:       0.518,
				Count:       1,
			},
		},
		{
			name:  "missing temperature",
			input: "11-14-2024 22:54:33 RAM 4300/30698MB GR3D_FREQ 45% VDD_GPU_SOC 2171mW",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  4300.0,
				MemoryTotal: 30698.0,
				Usage:       45.0,
				Power:       2.171,
				Count:       1,
			},
		},
		{
			name:  "orin-style output with GPU@ temp and VDD_SYS_GPU power",
			input: "RAM 3276/7859MB (lfb 5x4MB) SWAP 1626/12122MB (cached 181MB) CPU [44%@1421,49%@2031,67%@2034,17%@1420,25%@1419,8%@1420] EMC_FREQ 1%@1866 GR3D_FREQ 0%@114 APE 150 MTS fg 1% bg 1% PLL@42.5C MCPU@42.5C PMIC@50C Tboard@38C GPU@39.5C BCPU@42.5C thermal@41.3C Tdiode@39.25C VDD_SYS_GPU 182/182 VDD_SYS_SOC 730/730 VDD_4V0_WIFI 0/0 VDD_IN 5297/5297 VDD_SYS_CPU 1917/1917 VDD_SYS_DDR 1241/1241",
			wantMetrics: &system.GPUData{
				Name:        "GPU",
				MemoryUsed:  3276.0,
				MemoryTotal: 7859.0,
				Usage:       0.0,
				Power:       0.182, // 182mW -> 0.182W
				Temperature: 39.5,
				Count:       1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{
				GpuDataMap: make(map[string]*system.GPUData),
			}
			parser := gm.getJetsonParser()
			valid := parser([]byte(tt.input))
			assert.Equal(t, true, valid)

			got := gm.GpuDataMap["0"]
			require.NotNil(t, got)
			assert.Equal(t, tt.wantMetrics.Name, got.Name)
			assert.InDelta(t, tt.wantMetrics.MemoryUsed, got.MemoryUsed, 0.01)
			assert.InDelta(t, tt.wantMetrics.MemoryTotal, got.MemoryTotal, 0.01)
			assert.InDelta(t, tt.wantMetrics.Usage, got.Usage, 0.01)
			if tt.wantMetrics.Temperature > 0 {
				assert.InDelta(t, tt.wantMetrics.Temperature, got.Temperature, 0.01)
			}
			assert.InDelta(t, tt.wantMetrics.Power, got.Power, 0.01)
			assert.Equal(t, tt.wantMetrics.Count, got.Count)
		})
	}
}

func TestGetCurrentData(t *testing.T) {
	t.Run("calculates averages with per-cache-key delta tracking", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {
					Name:        "GPU1",
					Temperature: 50,
					MemoryUsed:  2048,
					MemoryTotal: 4096,
					Usage:       100, // 100 over 2 counts = 50 avg
					Power:       200, // 200 over 2 counts = 100 avg
					Count:       2,
				},
				"1": {
					Name:        "GPU1",
					Temperature: 60,
					MemoryUsed:  3072,
					MemoryTotal: 8192,
					Usage:       30,
					Power:       60,
					Count:       1,
				},
				"2": {
					Name:        "GPU 2",
					Temperature: 70,
					MemoryUsed:  4096,
					MemoryTotal: 8192,
					Usage:       200,
					Power:       400,
					Count:       1,
				},
			},
		}

		cacheKey := uint16(5000)
		result := gm.GetCurrentData(cacheKey)

		// Verify name disambiguation
		assert.Equal(t, "GPU1 0", result["0"].Name)
		assert.Equal(t, "GPU1 1", result["1"].Name)
		assert.Equal(t, "GPU 2", result["2"].Name)

		// Check averaged values in the result
		assert.InDelta(t, 50.0, result["0"].Usage, 0.01)
		assert.InDelta(t, 100.0, result["0"].Power, 0.01)
		assert.InDelta(t, 30.0, result["1"].Usage, 0.01)
		assert.InDelta(t, 60.0, result["1"].Power, 0.01)

		// Verify that accumulators in the original map are NOT reset (they keep growing)
		assert.EqualValues(t, 2, gm.GpuDataMap["0"].Count, "GPU 0 Count should remain at 2")
		assert.EqualValues(t, 100, gm.GpuDataMap["0"].Usage, "GPU 0 Usage should remain at 100")
		assert.Equal(t, 200.0, gm.GpuDataMap["0"].Power, "GPU 0 Power should remain at 200")
		assert.Equal(t, 1.0, gm.GpuDataMap["1"].Count, "GPU 1 Count should remain at 1")
		assert.Equal(t, 30.0, gm.GpuDataMap["1"].Usage, "GPU 1 Usage should remain at 30")
		assert.Equal(t, 60.0, gm.GpuDataMap["1"].Power, "GPU 1 Power should remain at 60")

		// Verify snapshots were stored for this cache key
		assert.NotNil(t, gm.lastSnapshots[cacheKey]["0"])
		assert.Equal(t, uint32(2), gm.lastSnapshots[cacheKey]["0"].count)
		assert.Equal(t, 100.0, gm.lastSnapshots[cacheKey]["0"].usage)
		assert.Equal(t, 200.0, gm.lastSnapshots[cacheKey]["0"].power)
	})

	t.Run("handles zero count without panicking", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {
					Name:  "TestGPU",
					Count: 0,
					Usage: 0,
					Power: 0,
				},
			},
		}

		cacheKey := uint16(5000)
		var result map[string]system.GPUData
		assert.NotPanics(t, func() {
			result = gm.GetCurrentData(cacheKey)
		})

		// Check that usage and power are 0
		assert.Equal(t, 0.0, result["0"].Usage)
		assert.Equal(t, 0.0, result["0"].Power)

		// Verify count remains 0
		assert.EqualValues(t, 0, gm.GpuDataMap["0"].Count)
	})

	t.Run("uses last average when no new data arrives", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {
					Name:        "TestGPU",
					Temperature: 55.0,
					MemoryUsed:  1500,
					MemoryTotal: 8000,
					Usage:       100, // Will average to 50
					Power:       200, // Will average to 100
					Count:       2,
				},
			},
		}

		cacheKey := uint16(5000)

		// First collection - should calculate averages and store them
		result1 := gm.GetCurrentData(cacheKey)
		assert.InDelta(t, 50.0, result1["0"].Usage, 0.01)
		assert.InDelta(t, 100.0, result1["0"].Power, 0.01)
		assert.EqualValues(t, 2, gm.GpuDataMap["0"].Count, "Count should remain at 2")

		// Update temperature but no new usage/power data (count stays same)
		gm.GpuDataMap["0"].Temperature = 60.0
		gm.GpuDataMap["0"].MemoryUsed = 1600

		// Second collection - should use last averages since count hasn't changed (delta = 0)
		result2 := gm.GetCurrentData(cacheKey)
		assert.InDelta(t, 50.0, result2["0"].Usage, 0.01, "Should use last average")
		assert.InDelta(t, 100.0, result2["0"].Power, 0.01, "Should use last average")
		assert.InDelta(t, 60.0, result2["0"].Temperature, 0.01, "Should use current temperature")
		assert.InDelta(t, 1600.0, result2["0"].MemoryUsed, 0.01, "Should use current memory")
		assert.EqualValues(t, 2, gm.GpuDataMap["0"].Count, "Count should still be 2")
	})

	t.Run("tracks separate averages per cache key", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {
					Name:        "TestGPU",
					Temperature: 55.0,
					MemoryUsed:  1500,
					MemoryTotal: 8000,
					Usage:       100, // Initial: 100 over 2 counts = 50 avg
					Power:       200, // Initial: 200 over 2 counts = 100 avg
					Count:       2,
				},
			},
		}

		cacheKey1 := uint16(5000)
		cacheKey2 := uint16(10000)

		// First check with cacheKey1 - baseline
		result1 := gm.GetCurrentData(cacheKey1)
		assert.InDelta(t, 50.0, result1["0"].Usage, 0.01, "CacheKey1: Initial average should be 50")
		assert.InDelta(t, 100.0, result1["0"].Power, 0.01, "CacheKey1: Initial average should be 100")

		// Simulate GPU activity - accumulate more data
		gm.GpuDataMap["0"].Usage += 60  // Now total: 160
		gm.GpuDataMap["0"].Power += 150 // Now total: 350
		gm.GpuDataMap["0"].Count += 3   // Now total: 5

		// Check with cacheKey1 again - should get delta since last cacheKey1 check
		result2 := gm.GetCurrentData(cacheKey1)
		assert.InDelta(t, 20.0, result2["0"].Usage, 0.01, "CacheKey1: Delta average should be 60/3 = 20")
		assert.InDelta(t, 50.0, result2["0"].Power, 0.01, "CacheKey1: Delta average should be 150/3 = 50")

		// Check with cacheKey2 for the first time - should get average since beginning
		result3 := gm.GetCurrentData(cacheKey2)
		assert.InDelta(t, 32.0, result3["0"].Usage, 0.01, "CacheKey2: Total average should be 160/5 = 32")
		assert.InDelta(t, 70.0, result3["0"].Power, 0.01, "CacheKey2: Total average should be 350/5 = 70")

		// Simulate more GPU activity
		gm.GpuDataMap["0"].Usage += 80  // Now total: 240
		gm.GpuDataMap["0"].Power += 160 // Now total: 510
		gm.GpuDataMap["0"].Count += 2   // Now total: 7

		// Check with cacheKey1 - should get delta since last cacheKey1 check
		result4 := gm.GetCurrentData(cacheKey1)
		assert.InDelta(t, 40.0, result4["0"].Usage, 0.01, "CacheKey1: New delta average should be 80/2 = 40")
		assert.InDelta(t, 80.0, result4["0"].Power, 0.01, "CacheKey1: New delta average should be 160/2 = 80")

		// Check with cacheKey2 - should get delta since last cacheKey2 check
		result5 := gm.GetCurrentData(cacheKey2)
		assert.InDelta(t, 40.0, result5["0"].Usage, 0.01, "CacheKey2: Delta average should be 80/2 = 40")
		assert.InDelta(t, 80.0, result5["0"].Power, 0.01, "CacheKey2: Delta average should be 160/2 = 80")

		// Verify snapshots exist for both cache keys
		assert.NotNil(t, gm.lastSnapshots[cacheKey1])
		assert.NotNil(t, gm.lastSnapshots[cacheKey2])
		assert.NotNil(t, gm.lastSnapshots[cacheKey1]["0"])
		assert.NotNil(t, gm.lastSnapshots[cacheKey2]["0"])
	})
}

func TestCalculateDeltaCount(t *testing.T) {
	gm := &GPUManager{}

	t.Run("with no previous snapshot", func(t *testing.T) {
		delta := gm.calculateDeltaCount(10, nil)
		assert.Equal(t, uint32(10), delta, "Should return current count when no snapshot exists")
	})

	t.Run("with previous snapshot", func(t *testing.T) {
		snapshot := &gpuSnapshot{count: 5}
		delta := gm.calculateDeltaCount(15, snapshot)
		assert.Equal(t, uint32(10), delta, "Should return difference between current and snapshot")
	})

	t.Run("with same count", func(t *testing.T) {
		snapshot := &gpuSnapshot{count: 10}
		delta := gm.calculateDeltaCount(10, snapshot)
		assert.Equal(t, uint32(0), delta, "Should return zero when count hasn't changed")
	})
}

func TestCalculateDeltas(t *testing.T) {
	gm := &GPUManager{}

	t.Run("with no previous snapshot", func(t *testing.T) {
		gpu := &system.GPUData{
			Usage:    100.5,
			Power:    250.75,
			PowerPkg: 300.25,
		}
		deltaUsage, deltaPower, deltaPowerPkg := gm.calculateDeltas(gpu, nil)
		assert.Equal(t, 100.5, deltaUsage)
		assert.Equal(t, 250.75, deltaPower)
		assert.Equal(t, 300.25, deltaPowerPkg)
	})

	t.Run("with previous snapshot", func(t *testing.T) {
		gpu := &system.GPUData{
			Usage:    150.5,
			Power:    300.75,
			PowerPkg: 400.25,
		}
		snapshot := &gpuSnapshot{
			usage:    100.5,
			power:    250.75,
			powerPkg: 300.25,
		}
		deltaUsage, deltaPower, deltaPowerPkg := gm.calculateDeltas(gpu, snapshot)
		assert.InDelta(t, 50.0, deltaUsage, 0.01)
		assert.InDelta(t, 50.0, deltaPower, 0.01)
		assert.InDelta(t, 100.0, deltaPowerPkg, 0.01)
	})
}

func TestCalculateIntelGPUUsage(t *testing.T) {
	gm := &GPUManager{}

	t.Run("with no previous snapshot", func(t *testing.T) {
		gpuAvg := &system.GPUData{
			Engines: make(map[string]float64),
		}
		gpu := &system.GPUData{
			Engines: map[string]float64{
				"Render/3D": 80.0,
				"Video":     40.0,
				"Compute":   60.0,
			},
		}
		maxUsage := gm.calculateIntelGPUUsage(gpuAvg, gpu, nil, 2)

		assert.Equal(t, 40.0, maxUsage, "Should return max engine usage (80/2=40)")
		assert.Equal(t, 40.0, gpuAvg.Engines["Render/3D"])
		assert.Equal(t, 20.0, gpuAvg.Engines["Video"])
		assert.Equal(t, 30.0, gpuAvg.Engines["Compute"])
	})

	t.Run("with previous snapshot", func(t *testing.T) {
		gpuAvg := &system.GPUData{
			Engines: make(map[string]float64),
		}
		gpu := &system.GPUData{
			Engines: map[string]float64{
				"Render/3D": 180.0,
				"Video":     100.0,
				"Compute":   140.0,
			},
		}
		snapshot := &gpuSnapshot{
			engines: map[string]float64{
				"Render/3D": 80.0,
				"Video":     40.0,
				"Compute":   60.0,
			},
		}
		maxUsage := gm.calculateIntelGPUUsage(gpuAvg, gpu, snapshot, 5)

		// Deltas: Render/3D=100, Video=60, Compute=80 over 5 counts
		assert.Equal(t, 20.0, maxUsage, "Should return max engine delta (100/5=20)")
		assert.Equal(t, 20.0, gpuAvg.Engines["Render/3D"])
		assert.Equal(t, 12.0, gpuAvg.Engines["Video"])
		assert.Equal(t, 16.0, gpuAvg.Engines["Compute"])
	})

	t.Run("handles missing engine in snapshot", func(t *testing.T) {
		gpuAvg := &system.GPUData{
			Engines: make(map[string]float64),
		}
		gpu := &system.GPUData{
			Engines: map[string]float64{
				"Render/3D": 100.0,
				"NewEngine": 50.0,
			},
		}
		snapshot := &gpuSnapshot{
			engines: map[string]float64{
				"Render/3D": 80.0,
				// NewEngine doesn't exist in snapshot
			},
		}
		maxUsage := gm.calculateIntelGPUUsage(gpuAvg, gpu, snapshot, 2)

		assert.Equal(t, 25.0, maxUsage)
		assert.Equal(t, 10.0, gpuAvg.Engines["Render/3D"], "Should use delta for existing engine")
		assert.Equal(t, 25.0, gpuAvg.Engines["NewEngine"], "Should use full value for new engine")
	})
}

func TestUpdateInstantaneousValues(t *testing.T) {
	gm := &GPUManager{}

	t.Run("updates temperature, memory used and total", func(t *testing.T) {
		gpuAvg := &system.GPUData{
			Temperature: 50.123,
			MemoryUsed:  1000.456,
			MemoryTotal: 8000.789,
		}
		gpu := &system.GPUData{
			Temperature: 75.567,
			MemoryUsed:  2500.891,
			MemoryTotal: 8192.234,
		}

		gm.updateInstantaneousValues(gpuAvg, gpu)

		assert.Equal(t, 75.57, gpuAvg.Temperature, "Should update and round temperature")
		assert.Equal(t, 2500.89, gpuAvg.MemoryUsed, "Should update and round memory used")
		assert.Equal(t, 8192.23, gpuAvg.MemoryTotal, "Should update and round memory total")
	})
}

func TestStoreSnapshot(t *testing.T) {
	gm := &GPUManager{
		lastSnapshots: make(map[uint16]map[string]*gpuSnapshot),
	}

	t.Run("stores standard GPU snapshot", func(t *testing.T) {
		cacheKey := uint16(5000)
		gm.lastSnapshots[cacheKey] = make(map[string]*gpuSnapshot)

		gpu := &system.GPUData{
			Count:    10.0,
			Usage:    150.5,
			Power:    250.75,
			PowerPkg: 300.25,
		}

		gm.storeSnapshot("0", gpu, cacheKey)

		snapshot := gm.lastSnapshots[cacheKey]["0"]
		assert.NotNil(t, snapshot)
		assert.Equal(t, uint32(10), snapshot.count)
		assert.Equal(t, 150.5, snapshot.usage)
		assert.Equal(t, 250.75, snapshot.power)
		assert.Equal(t, 300.25, snapshot.powerPkg)
		assert.Nil(t, snapshot.engines, "Should not have engines for standard GPU")
	})

	t.Run("stores Intel GPU snapshot with engines", func(t *testing.T) {
		cacheKey := uint16(10000)
		gm.lastSnapshots[cacheKey] = make(map[string]*gpuSnapshot)

		gpu := &system.GPUData{
			Count:    5.0,
			Usage:    100.0,
			Power:    200.0,
			PowerPkg: 250.0,
			Engines: map[string]float64{
				"Render/3D": 80.0,
				"Video":     40.0,
			},
		}

		gm.storeSnapshot("0", gpu, cacheKey)

		snapshot := gm.lastSnapshots[cacheKey]["0"]
		assert.NotNil(t, snapshot)
		assert.Equal(t, uint32(5), snapshot.count)
		assert.NotNil(t, snapshot.engines, "Should have engines for Intel GPU")
		assert.Equal(t, 80.0, snapshot.engines["Render/3D"])
		assert.Equal(t, 40.0, snapshot.engines["Video"])
		assert.Len(t, snapshot.engines, 2)
	})

	t.Run("overwrites existing snapshot", func(t *testing.T) {
		cacheKey := uint16(5000)
		gm.lastSnapshots[cacheKey] = make(map[string]*gpuSnapshot)

		// Store initial snapshot
		gpu1 := &system.GPUData{Count: 5.0, Usage: 100.0, Power: 200.0}
		gm.storeSnapshot("0", gpu1, cacheKey)

		// Store updated snapshot
		gpu2 := &system.GPUData{Count: 10.0, Usage: 250.0, Power: 400.0}
		gm.storeSnapshot("0", gpu2, cacheKey)

		snapshot := gm.lastSnapshots[cacheKey]["0"]
		assert.Equal(t, uint32(10), snapshot.count, "Should overwrite previous count")
		assert.Equal(t, 250.0, snapshot.usage, "Should overwrite previous usage")
		assert.Equal(t, 400.0, snapshot.power, "Should overwrite previous power")
	})
}

func TestCountGPUNames(t *testing.T) {
	t.Run("returns empty map for no GPUs", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: make(map[string]*system.GPUData),
		}
		counts := gm.countGPUNames()
		assert.Empty(t, counts)
	})

	t.Run("counts unique GPU names", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {Name: "GPU A"},
				"1": {Name: "GPU B"},
				"2": {Name: "GPU C"},
			},
		}
		counts := gm.countGPUNames()
		assert.Equal(t, 1, counts["GPU A"])
		assert.Equal(t, 1, counts["GPU B"])
		assert.Equal(t, 1, counts["GPU C"])
		assert.Len(t, counts, 3)
	})

	t.Run("counts duplicate GPU names", func(t *testing.T) {
		gm := &GPUManager{
			GpuDataMap: map[string]*system.GPUData{
				"0": {Name: "RTX 4090"},
				"1": {Name: "RTX 4090"},
				"2": {Name: "RTX 4090"},
				"3": {Name: "RTX 3080"},
			},
		}
		counts := gm.countGPUNames()
		assert.Equal(t, 3, counts["RTX 4090"])
		assert.Equal(t, 1, counts["RTX 3080"])
		assert.Len(t, counts, 2)
	})
}

func TestInitializeSnapshots(t *testing.T) {
	t.Run("initializes all maps from scratch", func(t *testing.T) {
		gm := &GPUManager{}
		cacheKey := uint16(5000)

		gm.initializeSnapshots(cacheKey)

		assert.NotNil(t, gm.lastAvgData)
		assert.NotNil(t, gm.lastSnapshots)
		assert.NotNil(t, gm.lastSnapshots[cacheKey])
	})

	t.Run("initializes only missing maps", func(t *testing.T) {
		gm := &GPUManager{
			lastAvgData: make(map[string]system.GPUData),
		}
		cacheKey := uint16(5000)

		gm.initializeSnapshots(cacheKey)

		assert.NotNil(t, gm.lastAvgData, "Should preserve existing lastAvgData")
		assert.NotNil(t, gm.lastSnapshots)
		assert.NotNil(t, gm.lastSnapshots[cacheKey])
	})

	t.Run("adds new cache key to existing snapshots", func(t *testing.T) {
		existingKey := uint16(5000)
		newKey := uint16(10000)

		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				existingKey: {"0": {count: 10}},
			},
		}

		gm.initializeSnapshots(newKey)

		assert.NotNil(t, gm.lastSnapshots[existingKey], "Should preserve existing cache key")
		assert.NotNil(t, gm.lastSnapshots[newKey], "Should add new cache key")
		assert.NotNil(t, gm.lastSnapshots[existingKey]["0"], "Should preserve existing snapshot data")
	})
}

func TestCalculateGPUAverage(t *testing.T) {
	t.Run("returns cached average when deltaCount is zero", func(t *testing.T) {
		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				5000: {
					"0": {count: 10, usage: 100, power: 200},
				},
			},
			lastAvgData: map[string]system.GPUData{
				"0": {Usage: 50.0, Power: 100.0},
			},
		}

		gpu := &system.GPUData{
			Count:       10.0, // Same as snapshot, so delta = 0
			Usage:       100.0,
			Power:       200.0,
			Temperature: 50.0, // Non-zero to avoid "suspended" check
		}

		result := gm.calculateGPUAverage("0", gpu, 5000)

		assert.Equal(t, 50.0, result.Usage, "Should return cached average")
		assert.Equal(t, 100.0, result.Power, "Should return cached average")
	})

	t.Run("returns zero value when GPU is suspended", func(t *testing.T) {
		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				5000: {
					"0": {count: 10, usage: 100, power: 200},
				},
			},
			lastAvgData: map[string]system.GPUData{
				"0": {Usage: 50.0, Power: 100.0},
			},
		}

		gpu := &system.GPUData{
			Name:        "Test GPU",
			Count:       10.0,
			Temperature: 0,
			MemoryUsed:  0,
		}

		result := gm.calculateGPUAverage("0", gpu, 5000)

		assert.Equal(t, 0.0, result.Usage, "Should return zero usage")
		assert.Equal(t, 0.0, result.Power, "Should return zero power")
	})

	t.Run("calculates average for standard GPU", func(t *testing.T) {
		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				5000: {},
			},
			lastAvgData: make(map[string]system.GPUData),
		}

		gpu := &system.GPUData{
			Name:  "Test GPU",
			Count: 4.0,
			Usage: 200.0, // 200 / 4 = 50
			Power: 400.0, // 400 / 4 = 100
		}

		result := gm.calculateGPUAverage("0", gpu, 5000)

		assert.Equal(t, 50.0, result.Usage)
		assert.Equal(t, 100.0, result.Power)
		assert.Equal(t, "Test GPU", result.Name)
	})

	t.Run("calculates average for Intel GPU with engines", func(t *testing.T) {
		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				5000: {},
			},
			lastAvgData: make(map[string]system.GPUData),
		}

		gpu := &system.GPUData{
			Name:     "Intel GPU",
			Count:    5.0,
			Power:    500.0,
			PowerPkg: 600.0,
			Engines: map[string]float64{
				"Render/3D": 100.0, // 100 / 5 = 20
				"Video":     50.0,  // 50 / 5 = 10
			},
		}

		result := gm.calculateGPUAverage("0", gpu, 5000)

		assert.Equal(t, 100.0, result.Power)
		assert.Equal(t, 120.0, result.PowerPkg)
		assert.Equal(t, 20.0, result.Usage, "Should use max engine usage")
		assert.Equal(t, 20.0, result.Engines["Render/3D"])
		assert.Equal(t, 10.0, result.Engines["Video"])
	})

	t.Run("calculates delta from previous snapshot", func(t *testing.T) {
		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				5000: {
					"0": {
						count:    2,
						usage:    50.0,
						power:    100.0,
						powerPkg: 120.0,
					},
				},
			},
			lastAvgData: make(map[string]system.GPUData),
		}

		gpu := &system.GPUData{
			Name:     "Test GPU",
			Count:    7.0,   // Delta = 7 - 2 = 5
			Usage:    200.0, // Delta = 200 - 50 = 150, avg = 150/5 = 30
			Power:    350.0, // Delta = 350 - 100 = 250, avg = 250/5 = 50
			PowerPkg: 420.0, // Delta = 420 - 120 = 300, avg = 300/5 = 60
		}

		result := gm.calculateGPUAverage("0", gpu, 5000)

		assert.Equal(t, 30.0, result.Usage)
		assert.Equal(t, 50.0, result.Power)
	})

	t.Run("stores result in lastAvgData", func(t *testing.T) {
		gm := &GPUManager{
			lastSnapshots: map[uint16]map[string]*gpuSnapshot{
				5000: {},
			},
			lastAvgData: make(map[string]system.GPUData),
		}

		gpu := &system.GPUData{
			Count: 2.0,
			Usage: 100.0,
			Power: 200.0,
		}

		result := gm.calculateGPUAverage("0", gpu, 5000)

		assert.Equal(t, result, gm.lastAvgData["0"], "Should store calculated average")
	})
}

func TestDetectGPUs(t *testing.T) {
	// Save original PATH
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	// Set up temp dir with the commands
	tempDir := t.TempDir()
	os.Setenv("PATH", tempDir)

	tests := []struct {
		name           string
		setupCommands  func() error
		wantNvidiaSmi  bool
		wantRocmSmi    bool
		wantTegrastats bool
		wantErr        bool
	}{
		{
			name: "nvidia-smi not available",
			setupCommands: func() error {
				return nil
			},
			wantNvidiaSmi:  false,
			wantRocmSmi:    false,
			wantTegrastats: false,
			wantErr:        true,
		},
		{
			name: "nvidia-smi available",
			setupCommands: func() error {
				path := filepath.Join(tempDir, "nvidia-smi")
				script := `#!/bin/sh
echo "test"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			wantNvidiaSmi:  true,
			wantTegrastats: false,
			wantRocmSmi:    false,
			wantErr:        false,
		},
		{
			name: "rocm-smi available",
			setupCommands: func() error {
				path := filepath.Join(tempDir, "rocm-smi")
				script := `#!/bin/sh
echo "test"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			wantNvidiaSmi:  true,
			wantRocmSmi:    true,
			wantTegrastats: false,
			wantErr:        false,
		},
		{
			name: "tegrastats available",
			setupCommands: func() error {
				path := filepath.Join(tempDir, "tegrastats")
				script := `#!/bin/sh
echo "test"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			wantNvidiaSmi:  false,
			wantRocmSmi:    true,
			wantTegrastats: true,
			wantErr:        false,
		},
		{
			name: "no gpu tools available",
			setupCommands: func() error {
				os.Setenv("PATH", "")
				return nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setupCommands(); err != nil {
				t.Fatal(err)
			}

			gm := &GPUManager{}
			err := gm.detectGPUs()

			t.Logf("nvidiaSmi: %v, rocmSmi: %v, tegrastats: %v", gm.nvidiaSmi, gm.rocmSmi, gm.tegrastats)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantNvidiaSmi, gm.nvidiaSmi)
			assert.Equal(t, tt.wantRocmSmi, gm.rocmSmi)
			assert.Equal(t, tt.wantTegrastats, gm.tegrastats)
		})
	}
}

func TestStartCollector(t *testing.T) {
	// Save original PATH
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	// Set up temp dir with the commands
	dir := t.TempDir()
	os.Setenv("PATH", dir)

	tests := []struct {
		name     string
		command  string
		setup    func(t *testing.T) error
		validate func(t *testing.T, gm *GPUManager)
		gm       *GPUManager
	}{
		{
			name:    "nvidia-smi collector",
			command: "nvidia-smi",
			setup: func(t *testing.T) error {
				path := filepath.Join(dir, "nvidia-smi")
				script := `#!/bin/sh
echo "0, NVIDIA Test GPU, 50, 1024, 4096, 25, 100"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			validate: func(t *testing.T, gm *GPUManager) {
				gpu, exists := gm.GpuDataMap["0"]
				assert.True(t, exists)
				if exists {
					assert.Equal(t, "Test GPU", gpu.Name)
					assert.Equal(t, 50.0, gpu.Temperature)

				}
			},
		},
		{
			name:    "rocm-smi collector",
			command: "rocm-smi",
			setup: func(t *testing.T) error {
				path := filepath.Join(dir, "rocm-smi")
				script := `#!/bin/sh
echo '{"card0": {"Temperature (Sensor edge) (C)": "49.0", "Current Socket Graphics Package Power (W)": "28.159", "GPU use (%)": "0", "VRAM Total Memory (B)": "536870912", "VRAM Total Used Memory (B)": "445550592", "Card Series": "Rembrandt [Radeon 680M]", "Card Model": "0x1681", "Card Vendor": "Advanced Micro Devices, Inc. [AMD/ATI]", "Card SKU": "REMBRANDT", "Subsystem ID": "0x8a22", "Device Rev": "0xc8", "Node ID": "1", "GUID": "34756", "GFX Version": "gfx1035"}}'`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			validate: func(t *testing.T, gm *GPUManager) {
				gpu, exists := gm.GpuDataMap["34756"]
				assert.True(t, exists)
				if exists {
					assert.Equal(t, "Rembrandt [Radeon 680M]", gpu.Name)
					assert.InDelta(t, 49.0, gpu.Temperature, 0.01)
					assert.InDelta(t, 28.159, gpu.Power, 0.01)
				}
			},
		},
		{
			name:    "tegrastats collector",
			command: "tegrastats",
			setup: func(t *testing.T) error {
				path := filepath.Join(dir, "tegrastats")
				script := `#!/bin/sh
echo "11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 80% tj@70C VDD_GPU_SOC 1000mW"`
				if err := os.WriteFile(path, []byte(script), 0755); err != nil {
					return err
				}
				return nil
			},
			validate: func(t *testing.T, gm *GPUManager) {
				gpu, exists := gm.GpuDataMap["0"]
				assert.True(t, exists)
				if exists {
					assert.InDelta(t, 70.0, gpu.Temperature, 0.1)
				}
			},
			gm: &GPUManager{
				GpuDataMap: map[string]*system.GPUData{
					"0": {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setup(t); err != nil {
				t.Fatal(err)
			}
			if tt.gm == nil {
				tt.gm = &GPUManager{
					GpuDataMap: make(map[string]*system.GPUData),
				}
			}
			tt.gm.startCollector(tt.command)
			time.Sleep(50 * time.Millisecond) // Give collector time to run
			tt.validate(t, tt.gm)
		})
	}
}

// TestAccumulationTableDriven tests the accumulation behavior for all three GPU types
func TestAccumulation(t *testing.T) {
	type expectedGPUValues struct {
		temperature float64
		memoryUsed  float64
		memoryTotal float64
		usage       float64
		power       float64
		count       float64
		avgUsage    float64
		avgPower    float64
	}

	tests := []struct {
		name           string
		initialGPUData map[string]*system.GPUData
		dataSamples    [][]byte
		parser         func(*GPUManager) func([]byte) bool
		expectedValues map[string]expectedGPUValues
	}{
		{
			name: "Jetson GPU accumulation",
			initialGPUData: map[string]*system.GPUData{
				"0": {
					Name:        "Jetson",
					Temperature: 0,
					Usage:       0,
					Power:       0,
					Count:       0,
				},
			},
			dataSamples: [][]byte{
				[]byte("11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 30% tj@50.5C VDD_GPU_SOC 1000mW"),
				[]byte("11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 40% tj@60.5C VDD_GPU_SOC 1200mW"),
				[]byte("11-14-2024 22:54:33 RAM 1024/4096MB GR3D_FREQ 50% tj@70.5C VDD_GPU_SOC 1400mW"),
			},
			parser: func(gm *GPUManager) func([]byte) bool {
				return gm.getJetsonParser()
			},
			expectedValues: map[string]expectedGPUValues{
				"0": {
					temperature: 70.5,  // Last value
					memoryUsed:  1024,  // Last value
					memoryTotal: 4096,  // Last value
					usage:       120.0, // Accumulated: 30 + 40 + 50
					power:       3.6,   // Accumulated: 1.0 + 1.2 + 1.4
					count:       3,
					avgUsage:    40.0, // 120 / 3
					avgPower:    1.2,  // 3.6 / 3
				},
			},
		},
		{
			name:           "NVIDIA GPU accumulation",
			initialGPUData: map[string]*system.GPUData{
				// NVIDIA parser will create the GPU data entries
			},
			dataSamples: [][]byte{
				[]byte("0, NVIDIA GeForce RTX 3080, 50, 5000, 10000, 30, 200"),
				[]byte("0, NVIDIA GeForce RTX 3080, 60, 6000, 10000, 40, 250"),
				[]byte("0, NVIDIA GeForce RTX 3080, 70, 7000, 10000, 50, 300"),
			},
			parser: func(gm *GPUManager) func([]byte) bool {
				return gm.parseNvidiaData
			},
			expectedValues: map[string]expectedGPUValues{
				"0": {
					temperature: 70.0,            // Last value
					memoryUsed:  7000.0 / 1.024,  // Last value
					memoryTotal: 10000.0 / 1.024, // Last value
					usage:       120.0,           // Accumulated: 30 + 40 + 50
					power:       750.0,           // Accumulated: 200 + 250 + 300
					count:       3,
					avgUsage:    40.0,  // 120 / 3
					avgPower:    250.0, // 750 / 3
				},
			},
		},
		{
			name:           "AMD GPU accumulation",
			initialGPUData: map[string]*system.GPUData{
				// AMD parser will create the GPU data entries
			},
			dataSamples: [][]byte{
				[]byte(`{"card0": {"GUID": "34756", "Temperature (Sensor edge) (C)": "50.0", "Current Socket Graphics Package Power (W)": "100.0", "GPU use (%)": "30", "VRAM Total Memory (B)": "10737418240", "VRAM Total Used Memory (B)": "1073741824", "Card Series": "Radeon RX 6800"}}`),
				[]byte(`{"card0": {"GUID": "34756", "Temperature (Sensor edge) (C)": "60.0", "Current Socket Graphics Package Power (W)": "150.0", "GPU use (%)": "40", "VRAM Total Memory (B)": "10737418240", "VRAM Total Used Memory (B)": "2147483648", "Card Series": "Radeon RX 6800"}}`),
				[]byte(`{"card0": {"GUID": "34756", "Temperature (Sensor edge) (C)": "70.0", "Current Socket Graphics Package Power (W)": "200.0", "GPU use (%)": "50", "VRAM Total Memory (B)": "10737418240", "VRAM Total Used Memory (B)": "3221225472", "Card Series": "Radeon RX 6800"}}`),
			},
			parser: func(gm *GPUManager) func([]byte) bool {
				return gm.parseAmdData
			},
			expectedValues: map[string]expectedGPUValues{
				"34756": {
					temperature: 70.0,                          // Last value
					memoryUsed:  3221225472.0 / (1024 * 1024),  // Last value
					memoryTotal: 10737418240.0 / (1024 * 1024), // Last value
					usage:       120.0,                         // Accumulated: 30 + 40 + 50
					power:       450.0,                         // Accumulated: 100 + 150 + 200
					count:       3,
					avgUsage:    40.0,  // 120 / 3
					avgPower:    150.0, // 450 / 3
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new GPUManager for each test
			gm := &GPUManager{
				GpuDataMap: tt.initialGPUData,
			}

			// Get the parser function
			parser := tt.parser(gm)

			// Process each data sample
			for i, sample := range tt.dataSamples {
				valid := parser(sample)
				assert.True(t, valid, "Sample %d should be valid", i)
			}

			// Check accumulated values
			for id, expected := range tt.expectedValues {
				gpu, exists := gm.GpuDataMap[id]
				assert.True(t, exists, "GPU with ID %s should exist", id)
				if !exists {
					continue
				}

				assert.EqualValues(t, expected.temperature, gpu.Temperature, "Temperature should match")
				assert.EqualValues(t, expected.memoryUsed, gpu.MemoryUsed, "Memory used should match")
				assert.EqualValues(t, expected.memoryTotal, gpu.MemoryTotal, "Memory total should match")
				assert.EqualValues(t, expected.usage, gpu.Usage, "Usage should match")
				assert.EqualValues(t, expected.power, gpu.Power, "Power should match")
				assert.Equal(t, expected.count, gpu.Count, "Count should match")
			}

			// Verify average calculation in GetCurrentData
			cacheKey := uint16(5000)
			result := gm.GetCurrentData(cacheKey)
			for id, expected := range tt.expectedValues {
				gpu, exists := result[id]
				assert.True(t, exists, "GPU with ID %s should exist in GetCurrentData result", id)
				if !exists {
					continue
				}

				assert.EqualValues(t, expected.temperature, gpu.Temperature, "Temperature in GetCurrentData should match")
				assert.EqualValues(t, expected.avgUsage, gpu.Usage, "Average usage in GetCurrentData should match")
				assert.EqualValues(t, expected.avgPower, gpu.Power, "Average power in GetCurrentData should match")
			}

			// Verify that accumulators in the original map are NOT reset (they keep growing)
			for id, expected := range tt.expectedValues {
				gpu, exists := gm.GpuDataMap[id]
				assert.True(t, exists, "GPU with ID %s should still exist after GetCurrentData", id)
				if !exists {
					continue
				}
				assert.EqualValues(t, expected.count, gpu.Count, "Count should remain at accumulated value for GPU ID %s", id)
				assert.EqualValues(t, expected.usage, gpu.Usage, "Usage should remain at accumulated value for GPU ID %s", id)
				assert.EqualValues(t, expected.power, gpu.Power, "Power should remain at accumulated value for GPU ID %s", id)
			}
		})
	}
}

func TestIntelUpdateFromStats(t *testing.T) {
	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}

	// First sample with power and two engines
	sample1 := intelGpuStats{
		PowerGPU: 10.5,
		Engines: map[string]float64{
			"Render/3D": 20.0,
			"Video":     5.0,
		},
	}

	ok := gm.updateIntelFromStats(&sample1)
	assert.True(t, ok)

	gpu := gm.GpuDataMap["i0"]
	require.NotNil(t, gpu)
	assert.Equal(t, "GPU", gpu.Name)
	assert.EqualValues(t, 10.5, gpu.Power)
	assert.EqualValues(t, 20.0, gpu.Engines["Render/3D"])
	assert.EqualValues(t, 5.0, gpu.Engines["Video"])
	assert.Equal(t, float64(1), gpu.Count)

	// Second sample with zero power (should not add) and additional engine busy
	sample2 := intelGpuStats{
		PowerGPU: 0.0,
		Engines: map[string]float64{
			"Render/3D": 10.0,
			"Video":     2.5,
			"Blitter":   1.0,
		},
	}
	// zero power should not increment power accumulator

	ok = gm.updateIntelFromStats(&sample2)
	assert.True(t, ok)

	gpu = gm.GpuDataMap["i0"]
	require.NotNil(t, gpu)
	assert.EqualValues(t, 10.5, gpu.Power)
	assert.EqualValues(t, 30.0, gpu.Engines["Render/3D"]) // 20 + 10
	assert.EqualValues(t, 7.5, gpu.Engines["Video"])      // 5 + 2.5
	assert.EqualValues(t, 1.0, gpu.Engines["Blitter"])
	assert.Equal(t, float64(2), gpu.Count)
}

func TestIntelCollectorStreaming(t *testing.T) {
	// Save and override PATH
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	dir := t.TempDir()
	os.Setenv("PATH", dir)

	// Create a fake intel_gpu_top that prints -l format with four samples (first will be skipped) and exits
	scriptPath := filepath.Join(dir, "intel_gpu_top")
	script := `#!/bin/sh
echo "Freq MHz      IRQ RC6     Power W     IMC MiB/s             RCS             BCS             VCS"
echo " req  act       /s   %   gpu   pkg     rd     wr       %  se  wa       %  se  wa       %  se  wa"
echo "373  373      224  45  1.50  4.13   2554    714   12.34   0   0    0.00   0   0    5.00   0   0"
echo "226  223      338  58  2.00  2.69   1820    965   0.00    0   0    0.00   0   0    0.00   0   0"
echo "189  187      412  67  1.80  2.45   1950    823   8.50    2   1    15.00   1   0    22.00  0   1"
echo "298  295      278  51  2.20  3.12   1675    942   5.75    1   2    9.50    3   1    12.00  1   0"`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}

	// Run the collector once; it should read four samples but skip the first and return
	if err := gm.collectIntelStats(); err != nil {
		t.Fatalf("collectIntelStats error: %v", err)
	}

	gpu := gm.GpuDataMap["i0"]
	require.NotNil(t, gpu)
	// Power should be sum of samples 2-4 (first is skipped): 2.0 + 1.8 + 2.2 = 6.0
	assert.EqualValues(t, 6.0, gpu.Power)
	assert.InDelta(t, 8.26, gpu.PowerPkg, 0.01) // Allow small floating point differences
	// Engines aggregated from samples 2-4
	assert.EqualValues(t, 14.25, gpu.Engines["Render/3D"]) // 0.00 + 8.50 + 5.75
	assert.EqualValues(t, 34.0, gpu.Engines["Video"])      // 0.00 + 22.00 + 12.00
	assert.EqualValues(t, 24.5, gpu.Engines["Blitter"])    // 0.00 + 15.00 + 9.50
	// Count should be 3 samples (first is skipped)
	assert.Equal(t, float64(3), gpu.Count)
}

func TestParseIntelHeaders(t *testing.T) {
	tests := []struct {
		name              string
		header1           string
		header2           string
		wantEngineNames   []string
		wantFriendlyNames []string
		wantPowerIndex    int
		wantPreEngineCols int
	}{
		{
			name:              "basic headers with RCS BCS VCS",
			header1:           "Freq MHz      IRQ RC6     Power W     IMC MiB/s             RCS             BCS             VCS",
			header2:           " req  act       /s   %   gpu   pkg     rd     wr       %  se  wa       %  se  wa       %  se  wa",
			wantEngineNames:   []string{"RCS", "BCS", "VCS"},
			wantFriendlyNames: []string{"Render/3D", "Blitter", "Video"},
			wantPowerIndex:    4, // "gpu" is at index 4
			wantPreEngineCols: 8, // 17 total cols - 3*3 = 8
		},
		{
			name:              "basic headers with RCS BCS VCS using index in name",
			header1:           "Freq MHz      IRQ RC6     Power W     IMC MiB/s           RCS/0           BCS/1           VCS/2",
			header2:           " req  act       /s   %   gpu   pkg     rd     wr       %  se  wa       %  se  wa       %  se  wa",
			wantEngineNames:   []string{"RCS", "BCS", "VCS"},
			wantFriendlyNames: []string{"Render/3D", "Blitter", "Video"},
			wantPowerIndex:    4, // "gpu" is at index 4
			wantPreEngineCols: 8, // 17 total cols - 3*3 = 8
		},
		{
			name:              "headers with only RCS",
			header1:           "Freq MHz      IRQ RC6     Power W     IMC MiB/s             RCS",
			header2:           " req  act       /s   %   gpu   pkg     rd     wr       %  se  wa",
			wantEngineNames:   []string{"RCS"},
			wantFriendlyNames: []string{"Render/3D"},
			wantPowerIndex:    4,
			wantPreEngineCols: 8, // 11 total - 3*1 = 8
		},
		{
			name:              "headers with VECS and CCS",
			header1:           "Freq MHz      IRQ RC6     Power W     IMC MiB/s             VECS            CCS",
			header2:           " req  act       /s   %   gpu   pkg     rd     wr       %  se  wa     %  se  wa",
			wantEngineNames:   []string{"VECS", "CCS"},
			wantFriendlyNames: []string{"VideoEnhance", "Compute"},
			wantPowerIndex:    4,
			wantPreEngineCols: 8, // 14 total - 3*2 = 8
		},
		{
			name:              "no engines",
			header1:           "Freq MHz      IRQ RC6     Power W     IMC MiB/s",
			header2:           " req  act       /s   %   gpu   pkg     rd     wr",
			wantEngineNames:   nil, // no engines found, slices remain nil
			wantFriendlyNames: nil,
			wantPowerIndex:    -1, // no engines, so no search
			wantPreEngineCols: 0,
		},
		{
			name:              "power index not found",
			header1:           "Freq MHz      IRQ RC6     Power W     IMC MiB/s             RCS",
			header2:           " req  act       /s   %   pkg   cpu     rd     wr       %  se  wa", // no "gpu"
			wantEngineNames:   []string{"RCS"},
			wantFriendlyNames: []string{"Render/3D"},
			wantPowerIndex:    -1, // "gpu" not found
			wantPreEngineCols: 8,  // 11 total - 3*1 = 8
		},
		{
			name:              "empty headers",
			header1:           "",
			header2:           "",
			wantEngineNames:   nil, // empty input, slices remain nil
			wantFriendlyNames: nil,
			wantPowerIndex:    -1,
			wantPreEngineCols: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{}
			engineNames, friendlyNames, powerIndex, preEngineCols := gm.parseIntelHeaders(tt.header1, tt.header2)

			assert.Equal(t, tt.wantEngineNames, engineNames)
			assert.Equal(t, tt.wantFriendlyNames, friendlyNames)
			assert.Equal(t, tt.wantPowerIndex, powerIndex)
			assert.Equal(t, tt.wantPreEngineCols, preEngineCols)
		})
	}
}

func TestParseIntelData(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		engineNames   []string
		friendlyNames []string
		powerIndex    int
		preEngineCols int
		wantPowerGPU  float64
		wantEngines   map[string]float64
		wantErr       error
	}{
		{
			name:          "basic data with power and engines",
			line:          "373  373      224  45  1.50  4.13   2554    714   12.34   0   0    0.00   0   0    5.00   0   0",
			engineNames:   []string{"RCS", "BCS", "VCS"},
			friendlyNames: []string{"Render/3D", "Blitter", "Video"},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  1.50,
			wantEngines: map[string]float64{
				"Render/3D": 12.34,
				"Blitter":   0.00,
				"Video":     5.00,
			},
		},
		{
			name:          "data with zero power",
			line:          "226  223      338  58  0.00  2.69   1820    965   0.00    0   0    0.00   0   0    0.00   0   0",
			engineNames:   []string{"RCS", "BCS", "VCS"},
			friendlyNames: []string{"Render/3D", "Blitter", "Video"},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  0.00,
			wantEngines: map[string]float64{
				"Render/3D": 0.00,
				"Blitter":   0.00,
				"Video":     0.00,
			},
		},
		{
			name:          "data with no power index",
			line:          "373  373      224  45  1.50  4.13   2554    714   12.34   0   0    0.00   0   0    5.00   0   0",
			engineNames:   []string{"RCS", "BCS", "VCS"},
			friendlyNames: []string{"Render/3D", "Blitter", "Video"},
			powerIndex:    -1,
			preEngineCols: 8,
			wantPowerGPU:  0.0, // no power parsed
			wantEngines: map[string]float64{
				"Render/3D": 12.34,
				"Blitter":   0.00,
				"Video":     5.00,
			},
		},
		{
			name:          "data with insufficient columns",
			line:          "373  373      224  45  1.50", // too few columns
			engineNames:   []string{"RCS", "BCS", "VCS"},
			friendlyNames: []string{"Render/3D", "Blitter", "Video"},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  0.0,
			wantEngines:   nil, // empty sample returned
			wantErr:       errNoValidData,
		},
		{
			name:          "empty line",
			line:          "",
			engineNames:   []string{"RCS"},
			friendlyNames: []string{"Render/3D"},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  0.0,
			wantEngines:   nil,
			wantErr:       errNoValidData,
		},
		{
			name:          "data with invalid power value",
			line:          "373  373      224  45  N/A  4.13   2554    714   12.34   0   0    0.00   0   0    5.00   0   0",
			engineNames:   []string{"RCS", "BCS", "VCS"},
			friendlyNames: []string{"Render/3D", "Blitter", "Video"},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  0.0, // N/A can't be parsed
			wantEngines: map[string]float64{
				"Render/3D": 12.34,
				"Blitter":   0.00,
				"Video":     5.00,
			},
		},
		{
			name:          "data with invalid engine value",
			line:          "373  373      224  45  1.50  4.13   2554    714   N/A     0   0    0.00   0   0    5.00   0   0",
			engineNames:   []string{"RCS", "BCS", "VCS"},
			friendlyNames: []string{"Render/3D", "Blitter", "Video"},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  1.50,
			wantEngines: map[string]float64{
				"Render/3D": 0.0, // N/A becomes 0
				"Blitter":   0.00,
				"Video":     5.00,
			},
		},
		{
			name:          "data with no engines",
			line:          "373  373      224  45  1.50  4.13   2554    714",
			engineNames:   []string{},
			friendlyNames: []string{},
			powerIndex:    4,
			preEngineCols: 8,
			wantPowerGPU:  1.50,
			wantEngines:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GPUManager{}
			sample, err := gm.parseIntelData(tt.line, tt.engineNames, tt.friendlyNames, tt.powerIndex, tt.preEngineCols)
			assert.Equal(t, tt.wantErr, err)

			assert.Equal(t, tt.wantPowerGPU, sample.PowerGPU)
			assert.Equal(t, tt.wantEngines, sample.Engines)
		})
	}
}

func TestIntelCollectorDeviceEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	// Prepare a file to capture args
	argsFile := filepath.Join(dir, "args.txt")

	// Create a fake intel_gpu_top that records its arguments and prints minimal valid output
	scriptPath := filepath.Join(dir, "intel_gpu_top")
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" > %s
echo "Freq MHz      IRQ RC6     Power W     IMC MiB/s             RCS             VCS"
echo " req  act       /s   %%   gpu   pkg     rd     wr       %%  se  wa       %%  se  wa"
echo "226  223      338  58  2.00  2.69   1820    965   0.00    0   0    0.00   0   0"
echo "189  187      412  67  1.80  2.45   1950    823   8.50    2   1    15.00   1   0"
`, argsFile)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Set device selector via prefixed env var
	t.Setenv("BESZEL_AGENT_INTEL_GPU_DEVICE", "sriov")

	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	if err := gm.collectIntelStats(); err != nil {
		t.Fatalf("collectIntelStats error: %v", err)
	}

	// Verify that -d sriov was passed
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed reading args file: %v", err)
	}
	argsStr := strings.TrimSpace(string(data))
	require.Contains(t, argsStr, "-d sriov")
	require.Contains(t, argsStr, "-s ")
	require.Contains(t, argsStr, "-l")
}
