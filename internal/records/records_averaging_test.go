//go:build testing

package records_test

import (
	"sort"
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/records"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAverageSystemStatsSlice_Empty(t *testing.T) {
	result := records.AverageSystemStatsSlice(nil)
	assert.Equal(t, system.Stats{}, result)

	result = records.AverageSystemStatsSlice([]system.Stats{})
	assert.Equal(t, system.Stats{}, result)
}

func TestAverageSystemStatsSlice_SingleRecord(t *testing.T) {
	input := []system.Stats{
		{
			Cpu:          45.67,
			Mem:          16.0,
			MemUsed:      8.5,
			MemPct:       53.12,
			MemBuffCache: 2.0,
			Swap:         4.0,
			SwapUsed:     1.0,
			DiskTotal:    500.0,
			DiskUsed:     250.0,
			DiskPct:      50.0,
			DiskReadPs:   100.5,
			DiskWritePs:  200.75,
			NetworkSent:  10.5,
			NetworkRecv:  20.25,
			LoadAvg:      [3]float64{1.5, 2.0, 3.5},
			Bandwidth:    [2]uint64{1000, 2000},
			DiskIO:       [2]uint64{500, 600},
			Battery:      [2]uint8{80, 1},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 45.67, result.Cpu)
	assert.Equal(t, 16.0, result.Mem)
	assert.Equal(t, 8.5, result.MemUsed)
	assert.Equal(t, 53.12, result.MemPct)
	assert.Equal(t, 2.0, result.MemBuffCache)
	assert.Equal(t, 4.0, result.Swap)
	assert.Equal(t, 1.0, result.SwapUsed)
	assert.Equal(t, 500.0, result.DiskTotal)
	assert.Equal(t, 250.0, result.DiskUsed)
	assert.Equal(t, 50.0, result.DiskPct)
	assert.Equal(t, 100.5, result.DiskReadPs)
	assert.Equal(t, 200.75, result.DiskWritePs)
	assert.Equal(t, 10.5, result.NetworkSent)
	assert.Equal(t, 20.25, result.NetworkRecv)
	assert.Equal(t, [3]float64{1.5, 2.0, 3.5}, result.LoadAvg)
	assert.Equal(t, [2]uint64{1000, 2000}, result.Bandwidth)
	assert.Equal(t, [2]uint64{500, 600}, result.DiskIO)
	assert.Equal(t, uint8(80), result.Battery[0])
	assert.Equal(t, uint8(1), result.Battery[1])
}

func TestAverageSystemStatsSlice_BasicAveraging(t *testing.T) {
	input := []system.Stats{
		{
			Cpu:          20.0,
			Mem:          16.0,
			MemUsed:      6.0,
			MemPct:       37.5,
			MemBuffCache: 1.0,
			MemZfsArc:    0.5,
			Swap:         4.0,
			SwapUsed:     1.0,
			DiskTotal:    500.0,
			DiskUsed:     200.0,
			DiskPct:      40.0,
			DiskReadPs:   100.0,
			DiskWritePs:  200.0,
			NetworkSent:  10.0,
			NetworkRecv:  20.0,
			LoadAvg:      [3]float64{1.0, 2.0, 3.0},
			Bandwidth:    [2]uint64{1000, 2000},
			DiskIO:       [2]uint64{400, 600},
			Battery:      [2]uint8{80, 1},
		},
		{
			Cpu:          40.0,
			Mem:          16.0,
			MemUsed:      10.0,
			MemPct:       62.5,
			MemBuffCache: 3.0,
			MemZfsArc:    1.5,
			Swap:         4.0,
			SwapUsed:     3.0,
			DiskTotal:    500.0,
			DiskUsed:     300.0,
			DiskPct:      60.0,
			DiskReadPs:   200.0,
			DiskWritePs:  400.0,
			NetworkSent:  30.0,
			NetworkRecv:  40.0,
			LoadAvg:      [3]float64{3.0, 4.0, 5.0},
			Bandwidth:    [2]uint64{3000, 4000},
			DiskIO:       [2]uint64{600, 800},
			Battery:      [2]uint8{60, 1},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 30.0, result.Cpu)
	assert.Equal(t, 16.0, result.Mem)
	assert.Equal(t, 8.0, result.MemUsed)
	assert.Equal(t, 50.0, result.MemPct)
	assert.Equal(t, 2.0, result.MemBuffCache)
	assert.Equal(t, 1.0, result.MemZfsArc)
	assert.Equal(t, 4.0, result.Swap)
	assert.Equal(t, 2.0, result.SwapUsed)
	assert.Equal(t, 500.0, result.DiskTotal)
	assert.Equal(t, 250.0, result.DiskUsed)
	assert.Equal(t, 50.0, result.DiskPct)
	assert.Equal(t, 150.0, result.DiskReadPs)
	assert.Equal(t, 300.0, result.DiskWritePs)
	assert.Equal(t, 20.0, result.NetworkSent)
	assert.Equal(t, 30.0, result.NetworkRecv)
	assert.Equal(t, [3]float64{2.0, 3.0, 4.0}, result.LoadAvg)
	assert.Equal(t, [2]uint64{2000, 3000}, result.Bandwidth)
	assert.Equal(t, [2]uint64{500, 700}, result.DiskIO)
	assert.Equal(t, uint8(70), result.Battery[0])
	assert.Equal(t, uint8(1), result.Battery[1])
}

func TestAverageSystemStatsSlice_PeakValues(t *testing.T) {
	input := []system.Stats{
		{
			Cpu:            20.0,
			MaxCpu:         25.0,
			MemUsed:        6.0,
			MaxMem:         7.0,
			NetworkSent:    10.0,
			MaxNetworkSent: 15.0,
			NetworkRecv:    20.0,
			MaxNetworkRecv: 25.0,
			DiskReadPs:     100.0,
			MaxDiskReadPs:  120.0,
			DiskWritePs:    200.0,
			MaxDiskWritePs: 220.0,
			Bandwidth:      [2]uint64{1000, 2000},
			MaxBandwidth:   [2]uint64{1500, 2500},
			DiskIO:         [2]uint64{400, 600},
			MaxDiskIO:      [2]uint64{500, 700},
			DiskIoStats:    [6]float64{10.0, 20.0, 30.0, 5.0, 8.0, 12.0},
			MaxDiskIoStats: [6]float64{15.0, 25.0, 35.0, 6.0, 9.0, 14.0},
		},
		{
			Cpu:            40.0,
			MaxCpu:         50.0,
			MemUsed:        10.0,
			MaxMem:         12.0,
			NetworkSent:    30.0,
			MaxNetworkSent: 35.0,
			NetworkRecv:    40.0,
			MaxNetworkRecv: 45.0,
			DiskReadPs:     200.0,
			MaxDiskReadPs:  210.0,
			DiskWritePs:    400.0,
			MaxDiskWritePs: 410.0,
			Bandwidth:      [2]uint64{3000, 4000},
			MaxBandwidth:   [2]uint64{3500, 4500},
			DiskIO:         [2]uint64{600, 800},
			MaxDiskIO:      [2]uint64{650, 850},
			DiskIoStats:    [6]float64{50.0, 60.0, 70.0, 15.0, 18.0, 22.0},
			MaxDiskIoStats: [6]float64{55.0, 65.0, 75.0, 16.0, 19.0, 23.0},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 50.0, result.MaxCpu)
	assert.Equal(t, 12.0, result.MaxMem)
	assert.Equal(t, 35.0, result.MaxNetworkSent)
	assert.Equal(t, 45.0, result.MaxNetworkRecv)
	assert.Equal(t, 210.0, result.MaxDiskReadPs)
	assert.Equal(t, 410.0, result.MaxDiskWritePs)
	assert.Equal(t, [2]uint64{3500, 4500}, result.MaxBandwidth)
	assert.Equal(t, [2]uint64{650, 850}, result.MaxDiskIO)
	assert.Equal(t, [6]float64{30.0, 40.0, 50.0, 10.0, 13.0, 17.0}, result.DiskIoStats)
	assert.Equal(t, [6]float64{55.0, 65.0, 75.0, 16.0, 19.0, 23.0}, result.MaxDiskIoStats)
}

func TestAverageSystemStatsSlice_DiskIoStats(t *testing.T) {
	input := []system.Stats{
		{
			Cpu:            10.0,
			DiskIoStats:    [6]float64{10.0, 20.0, 30.0, 5.0, 8.0, 12.0},
			MaxDiskIoStats: [6]float64{12.0, 22.0, 32.0, 6.0, 9.0, 13.0},
		},
		{
			Cpu:            20.0,
			DiskIoStats:    [6]float64{30.0, 40.0, 50.0, 15.0, 18.0, 22.0},
			MaxDiskIoStats: [6]float64{28.0, 38.0, 48.0, 14.0, 17.0, 21.0},
		},
		{
			Cpu:            30.0,
			DiskIoStats:    [6]float64{20.0, 30.0, 40.0, 10.0, 12.0, 16.0},
			MaxDiskIoStats: [6]float64{25.0, 35.0, 45.0, 11.0, 13.0, 17.0},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	// Average: (10+30+20)/3=20, (20+40+30)/3=30, (30+50+40)/3=40, (5+15+10)/3=10, (8+18+12)/3≈12.67, (12+22+16)/3≈16.67
	assert.Equal(t, 20.0, result.DiskIoStats[0])
	assert.Equal(t, 30.0, result.DiskIoStats[1])
	assert.Equal(t, 40.0, result.DiskIoStats[2])
	assert.Equal(t, 10.0, result.DiskIoStats[3])
	assert.Equal(t, 12.67, result.DiskIoStats[4])
	assert.Equal(t, 16.67, result.DiskIoStats[5])
	// Max: current DiskIoStats[0] wins for record 2 (30 > MaxDiskIoStats 28)
	assert.Equal(t, 30.0, result.MaxDiskIoStats[0])
	assert.Equal(t, 40.0, result.MaxDiskIoStats[1])
	assert.Equal(t, 50.0, result.MaxDiskIoStats[2])
	assert.Equal(t, 15.0, result.MaxDiskIoStats[3])
	assert.Equal(t, 18.0, result.MaxDiskIoStats[4])
	assert.Equal(t, 22.0, result.MaxDiskIoStats[5])
}

// Tests that current DiskIoStats values are considered when computing MaxDiskIoStats.
func TestAverageSystemStatsSlice_DiskIoStatsPeakFromCurrentValues(t *testing.T) {
	input := []system.Stats{
		{Cpu: 10.0, DiskIoStats: [6]float64{95.0, 90.0, 85.0, 50.0, 60.0, 80.0}, MaxDiskIoStats: [6]float64{80.0, 80.0, 80.0, 40.0, 50.0, 70.0}},
		{Cpu: 20.0, DiskIoStats: [6]float64{10.0, 10.0, 10.0, 5.0, 6.0, 8.0}, MaxDiskIoStats: [6]float64{20.0, 20.0, 20.0, 10.0, 12.0, 16.0}},
	}

	result := records.AverageSystemStatsSlice(input)

	// Current value from first record (95, 90, 85, 50, 60, 80) beats MaxDiskIoStats in both records
	assert.Equal(t, 95.0, result.MaxDiskIoStats[0])
	assert.Equal(t, 90.0, result.MaxDiskIoStats[1])
	assert.Equal(t, 85.0, result.MaxDiskIoStats[2])
	assert.Equal(t, 50.0, result.MaxDiskIoStats[3])
	assert.Equal(t, 60.0, result.MaxDiskIoStats[4])
	assert.Equal(t, 80.0, result.MaxDiskIoStats[5])
}

// Tests that current values are considered when computing peaks
// (i.e., current cpu > MaxCpu should still win).
func TestAverageSystemStatsSlice_PeakFromCurrentValues(t *testing.T) {
	input := []system.Stats{
		{Cpu: 95.0, MaxCpu: 80.0, MemUsed: 15.0, MaxMem: 10.0},
		{Cpu: 10.0, MaxCpu: 20.0, MemUsed: 5.0, MaxMem: 8.0},
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 95.0, result.MaxCpu)
	assert.Equal(t, 15.0, result.MaxMem)
}

// Tests that records without temperature data are excluded from the temperature average.
func TestAverageSystemStatsSlice_Temperatures(t *testing.T) {
	input := []system.Stats{
		{
			Cpu:          10.0,
			Temperatures: map[string]float64{"cpu": 60.0, "gpu": 70.0},
		},
		{
			Cpu:          20.0,
			Temperatures: map[string]float64{"cpu": 80.0, "gpu": 90.0},
		},
		{
			// No temperatures - should not affect temp averaging
			Cpu: 30.0,
		},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.Temperatures)
	// Average over 2 records that had temps, not 3
	assert.Equal(t, 70.0, result.Temperatures["cpu"])
	assert.Equal(t, 80.0, result.Temperatures["gpu"])
}

func TestAverageSystemStatsSlice_NetworkInterfaces(t *testing.T) {
	input := []system.Stats{
		{
			Cpu: 10.0,
			NetworkInterfaces: map[string][4]uint64{
				"eth0": {100, 200, 150, 250},
				"eth1": {50, 60, 70, 80},
			},
		},
		{
			Cpu: 20.0,
			NetworkInterfaces: map[string][4]uint64{
				"eth0": {200, 400, 300, 500},
				"eth1": {150, 160, 170, 180},
			},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.NetworkInterfaces)
	// [0] and [1] are averaged, [2] and [3] are max
	assert.Equal(t, [4]uint64{150, 300, 300, 500}, result.NetworkInterfaces["eth0"])
	assert.Equal(t, [4]uint64{100, 110, 170, 180}, result.NetworkInterfaces["eth1"])
}

func TestAverageSystemStatsSlice_ExtraFs(t *testing.T) {
	input := []system.Stats{
		{
			Cpu: 10.0,
			ExtraFs: map[string]*system.FsStats{
				"/data": {
					DiskTotal:         1000.0,
					DiskUsed:          400.0,
					DiskReadPs:        50.0,
					DiskWritePs:       100.0,
					MaxDiskReadPS:     60.0,
					MaxDiskWritePS:    110.0,
					DiskReadBytes:     5000,
					DiskWriteBytes:    10000,
					MaxDiskReadBytes:  6000,
					MaxDiskWriteBytes: 11000,
					DiskIoStats:       [6]float64{10.0, 20.0, 30.0, 5.0, 8.0, 12.0},
					MaxDiskIoStats:    [6]float64{12.0, 22.0, 32.0, 6.0, 9.0, 13.0},
				},
			},
		},
		{
			Cpu: 20.0,
			ExtraFs: map[string]*system.FsStats{
				"/data": {
					DiskTotal:         1000.0,
					DiskUsed:          600.0,
					DiskReadPs:        150.0,
					DiskWritePs:       200.0,
					MaxDiskReadPS:     160.0,
					MaxDiskWritePS:    210.0,
					DiskReadBytes:     15000,
					DiskWriteBytes:    20000,
					MaxDiskReadBytes:  16000,
					MaxDiskWriteBytes: 21000,
					DiskIoStats:       [6]float64{50.0, 60.0, 70.0, 15.0, 18.0, 22.0},
					MaxDiskIoStats:    [6]float64{55.0, 65.0, 75.0, 16.0, 19.0, 23.0},
				},
			},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.ExtraFs)
	require.NotNil(t, result.ExtraFs["/data"])
	fs := result.ExtraFs["/data"]
	assert.Equal(t, 1000.0, fs.DiskTotal)
	assert.Equal(t, 500.0, fs.DiskUsed)
	assert.Equal(t, 100.0, fs.DiskReadPs)
	assert.Equal(t, 150.0, fs.DiskWritePs)
	assert.Equal(t, 160.0, fs.MaxDiskReadPS)
	assert.Equal(t, 210.0, fs.MaxDiskWritePS)
	assert.Equal(t, uint64(10000), fs.DiskReadBytes)
	assert.Equal(t, uint64(15000), fs.DiskWriteBytes)
	assert.Equal(t, uint64(16000), fs.MaxDiskReadBytes)
	assert.Equal(t, uint64(21000), fs.MaxDiskWriteBytes)
	assert.Equal(t, [6]float64{30.0, 40.0, 50.0, 10.0, 13.0, 17.0}, fs.DiskIoStats)
	assert.Equal(t, [6]float64{55.0, 65.0, 75.0, 16.0, 19.0, 23.0}, fs.MaxDiskIoStats)
}

// Tests that ExtraFs DiskIoStats peak considers current values, not just previous peaks.
func TestAverageSystemStatsSlice_ExtraFsDiskIoStatsPeakFromCurrentValues(t *testing.T) {
	input := []system.Stats{
		{
			Cpu: 10.0,
			ExtraFs: map[string]*system.FsStats{
				"/data": {
					DiskIoStats:    [6]float64{95.0, 90.0, 85.0, 50.0, 60.0, 80.0}, // exceeds MaxDiskIoStats
					MaxDiskIoStats: [6]float64{80.0, 80.0, 80.0, 40.0, 50.0, 70.0},
				},
			},
		},
		{
			Cpu: 20.0,
			ExtraFs: map[string]*system.FsStats{
				"/data": {
					DiskIoStats:    [6]float64{10.0, 10.0, 10.0, 5.0, 6.0, 8.0},
					MaxDiskIoStats: [6]float64{20.0, 20.0, 20.0, 10.0, 12.0, 16.0},
				},
			},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	fs := result.ExtraFs["/data"]
	assert.Equal(t, 95.0, fs.MaxDiskIoStats[0])
	assert.Equal(t, 90.0, fs.MaxDiskIoStats[1])
	assert.Equal(t, 85.0, fs.MaxDiskIoStats[2])
	assert.Equal(t, 50.0, fs.MaxDiskIoStats[3])
	assert.Equal(t, 60.0, fs.MaxDiskIoStats[4])
	assert.Equal(t, 80.0, fs.MaxDiskIoStats[5])
}

// Tests that extra FS peak values consider current values, not just previous peaks.
func TestAverageSystemStatsSlice_ExtraFsPeaksFromCurrentValues(t *testing.T) {
	input := []system.Stats{
		{
			Cpu: 10.0,
			ExtraFs: map[string]*system.FsStats{
				"/data": {
					DiskReadPs:       500.0, // exceeds MaxDiskReadPS
					MaxDiskReadPS:    100.0,
					DiskReadBytes:    50000,
					MaxDiskReadBytes: 10000,
				},
			},
		},
		{
			Cpu: 20.0,
			ExtraFs: map[string]*system.FsStats{
				"/data": {
					DiskReadPs:       50.0,
					MaxDiskReadPS:    200.0,
					DiskReadBytes:    5000,
					MaxDiskReadBytes: 20000,
				},
			},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	fs := result.ExtraFs["/data"]
	assert.Equal(t, 500.0, fs.MaxDiskReadPS)
	assert.Equal(t, uint64(50000), fs.MaxDiskReadBytes)
}

func TestAverageSystemStatsSlice_GPUData(t *testing.T) {
	input := []system.Stats{
		{
			Cpu: 10.0,
			GPUData: map[string]system.GPUData{
				"gpu0": {
					Name:        "RTX 4090",
					Temperature: 60.0,
					MemoryUsed:  4.0,
					MemoryTotal: 24.0,
					Usage:       30.0,
					Power:       200.0,
					Count:       1.0,
					Engines: map[string]float64{
						"3D":    50.0,
						"Video": 10.0,
					},
				},
			},
		},
		{
			Cpu: 20.0,
			GPUData: map[string]system.GPUData{
				"gpu0": {
					Name:        "RTX 4090",
					Temperature: 80.0,
					MemoryUsed:  8.0,
					MemoryTotal: 24.0,
					Usage:       70.0,
					Power:       300.0,
					Count:       1.0,
					Engines: map[string]float64{
						"3D":    90.0,
						"Video": 30.0,
					},
				},
			},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.GPUData)
	gpu := result.GPUData["gpu0"]
	assert.Equal(t, "RTX 4090", gpu.Name)
	assert.Equal(t, 70.0, gpu.Temperature)
	assert.Equal(t, 6.0, gpu.MemoryUsed)
	assert.Equal(t, 24.0, gpu.MemoryTotal)
	assert.Equal(t, 50.0, gpu.Usage)
	assert.Equal(t, 250.0, gpu.Power)
	assert.Equal(t, 1.0, gpu.Count)
	require.NotNil(t, gpu.Engines)
	assert.Equal(t, 70.0, gpu.Engines["3D"])
	assert.Equal(t, 20.0, gpu.Engines["Video"])
}

func TestAverageSystemStatsSlice_MultipleGPUs(t *testing.T) {
	input := []system.Stats{
		{
			Cpu: 10.0,
			GPUData: map[string]system.GPUData{
				"gpu0": {Name: "GPU A", Usage: 20.0, Temperature: 50.0},
				"gpu1": {Name: "GPU B", Usage: 60.0, Temperature: 70.0},
			},
		},
		{
			Cpu: 20.0,
			GPUData: map[string]system.GPUData{
				"gpu0": {Name: "GPU A", Usage: 40.0, Temperature: 60.0},
				"gpu1": {Name: "GPU B", Usage: 80.0, Temperature: 80.0},
			},
		},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.GPUData)
	assert.Equal(t, 30.0, result.GPUData["gpu0"].Usage)
	assert.Equal(t, 55.0, result.GPUData["gpu0"].Temperature)
	assert.Equal(t, 70.0, result.GPUData["gpu1"].Usage)
	assert.Equal(t, 75.0, result.GPUData["gpu1"].Temperature)
}

func TestAverageSystemStatsSlice_CpuCoresUsage(t *testing.T) {
	input := []system.Stats{
		{Cpu: 10.0, CpuCoresUsage: system.Uint8Slice{10, 20, 30, 40}},
		{Cpu: 20.0, CpuCoresUsage: system.Uint8Slice{30, 40, 50, 60}},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.CpuCoresUsage)
	assert.Equal(t, system.Uint8Slice{20, 30, 40, 50}, result.CpuCoresUsage)
}

// Tests that per-core usage rounds correctly (e.g., 15.5 -> 16 via math.Round).
func TestAverageSystemStatsSlice_CpuCoresUsageRounding(t *testing.T) {
	input := []system.Stats{
		{Cpu: 10.0, CpuCoresUsage: system.Uint8Slice{11}},
		{Cpu: 20.0, CpuCoresUsage: system.Uint8Slice{20}},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.CpuCoresUsage)
	// (11+20)/2 = 15.5, rounds to 16
	assert.Equal(t, uint8(16), result.CpuCoresUsage[0])
}

func TestAverageSystemStatsSlice_CpuBreakdown(t *testing.T) {
	input := []system.Stats{
		{Cpu: 10.0, CpuBreakdown: []float64{5.0, 3.0, 1.0, 0.5, 90.5}},
		{Cpu: 20.0, CpuBreakdown: []float64{15.0, 7.0, 3.0, 1.5, 73.5}},
	}

	result := records.AverageSystemStatsSlice(input)

	require.NotNil(t, result.CpuBreakdown)
	assert.Equal(t, []float64{10.0, 5.0, 2.0, 1.0, 82.0}, result.CpuBreakdown)
}

// Tests that Battery[1] (charge state) uses the last record's value.
func TestAverageSystemStatsSlice_BatteryLastChargeState(t *testing.T) {
	input := []system.Stats{
		{Cpu: 10.0, Battery: [2]uint8{100, 1}}, // charging
		{Cpu: 20.0, Battery: [2]uint8{90, 0}},  // not charging
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, uint8(95), result.Battery[0])
	assert.Equal(t, uint8(0), result.Battery[1]) // last record's charge state
}

func TestAverageSystemStatsSlice_ThreeRecordsRounding(t *testing.T) {
	input := []system.Stats{
		{Cpu: 10.0, Mem: 8.0},
		{Cpu: 20.0, Mem: 8.0},
		{Cpu: 30.0, Mem: 8.0},
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 20.0, result.Cpu)
	assert.Equal(t, 8.0, result.Mem)
}

// Tests records where some have optional fields and others don't.
func TestAverageSystemStatsSlice_MixedOptionalFields(t *testing.T) {
	input := []system.Stats{
		{
			Cpu:           10.0,
			CpuCoresUsage: system.Uint8Slice{50, 60},
			CpuBreakdown:  []float64{5.0, 3.0, 1.0, 0.5, 90.5},
			GPUData: map[string]system.GPUData{
				"gpu0": {Name: "GPU", Usage: 40.0},
			},
		},
		{
			Cpu: 20.0,
			// No CpuCoresUsage, CpuBreakdown, or GPUData
		},
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 15.0, result.Cpu)
	// CpuCoresUsage: only 1 record had it, so sum/2
	require.NotNil(t, result.CpuCoresUsage)
	assert.Equal(t, uint8(25), result.CpuCoresUsage[0])
	assert.Equal(t, uint8(30), result.CpuCoresUsage[1])
	// CpuBreakdown: only 1 record had it, so sum/2
	require.NotNil(t, result.CpuBreakdown)
	assert.Equal(t, 2.5, result.CpuBreakdown[0])
	// GPUData: only 1 record had it, so sum/2
	require.NotNil(t, result.GPUData)
	assert.Equal(t, 20.0, result.GPUData["gpu0"].Usage)
}

// Tests with 10 records matching the common real-world case (10 x 1m -> 1 x 10m).
func TestAverageSystemStatsSlice_TenRecords(t *testing.T) {
	input := make([]system.Stats, 10)
	for i := range input {
		input[i] = system.Stats{
			Cpu:         float64(i * 10), // 0, 10, 20, ..., 90
			Mem:         16.0,
			MemUsed:     float64(4 + i),  // 4, 5, 6, ..., 13
			MemPct:      float64(25 + i), // 25, 26, ..., 34
			DiskTotal:   500.0,
			DiskUsed:    250.0,
			DiskPct:     50.0,
			NetworkSent: float64(i),
			NetworkRecv: float64(i * 2),
			Bandwidth:   [2]uint64{uint64(i * 1000), uint64(i * 2000)},
			LoadAvg:     [3]float64{float64(i), float64(i) * 0.5, float64(i) * 0.25},
		}
	}

	result := records.AverageSystemStatsSlice(input)

	assert.Equal(t, 45.0, result.Cpu)    // avg of 0..90
	assert.Equal(t, 16.0, result.Mem)    // constant
	assert.Equal(t, 8.5, result.MemUsed) // avg of 4..13
	assert.Equal(t, 29.5, result.MemPct) // avg of 25..34
	assert.Equal(t, 500.0, result.DiskTotal)
	assert.Equal(t, 250.0, result.DiskUsed)
	assert.Equal(t, 50.0, result.DiskPct)
	assert.Equal(t, 4.5, result.NetworkSent)
	assert.Equal(t, 9.0, result.NetworkRecv)
	assert.Equal(t, [2]uint64{4500, 9000}, result.Bandwidth)
}

// --- Container Stats Tests ---

func TestAverageContainerStatsSlice_Empty(t *testing.T) {
	result := records.AverageContainerStatsSlice(nil)
	assert.Equal(t, []container.Stats{}, result)

	result = records.AverageContainerStatsSlice([][]container.Stats{})
	assert.Equal(t, []container.Stats{}, result)
}

func TestAverageContainerStatsSlice_SingleRecord(t *testing.T) {
	input := [][]container.Stats{
		{
			{Name: "nginx", Cpu: 5.0, Mem: 128.0, Bandwidth: [2]uint64{1000, 2000}},
		},
	}

	result := records.AverageContainerStatsSlice(input)

	require.Len(t, result, 1)
	assert.Equal(t, "nginx", result[0].Name)
	assert.Equal(t, 5.0, result[0].Cpu)
	assert.Equal(t, 128.0, result[0].Mem)
	assert.Equal(t, [2]uint64{1000, 2000}, result[0].Bandwidth)
}

func TestAverageContainerStatsSlice_BasicAveraging(t *testing.T) {
	input := [][]container.Stats{
		{
			{Name: "nginx", Cpu: 10.0, Mem: 100.0, Bandwidth: [2]uint64{1000, 2000}},
			{Name: "redis", Cpu: 5.0, Mem: 64.0, Bandwidth: [2]uint64{500, 1000}},
		},
		{
			{Name: "nginx", Cpu: 20.0, Mem: 200.0, Bandwidth: [2]uint64{3000, 4000}},
			{Name: "redis", Cpu: 15.0, Mem: 128.0, Bandwidth: [2]uint64{1500, 2000}},
		},
	}

	result := records.AverageContainerStatsSlice(input)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	require.Len(t, result, 2)

	assert.Equal(t, "nginx", result[0].Name)
	assert.Equal(t, 15.0, result[0].Cpu)
	assert.Equal(t, 150.0, result[0].Mem)
	assert.Equal(t, [2]uint64{2000, 3000}, result[0].Bandwidth)

	assert.Equal(t, "redis", result[1].Name)
	assert.Equal(t, 10.0, result[1].Cpu)
	assert.Equal(t, 96.0, result[1].Mem)
	assert.Equal(t, [2]uint64{1000, 1500}, result[1].Bandwidth)
}

// Tests containers that appear in some records but not all.
func TestAverageContainerStatsSlice_ContainerAppearsInSomeRecords(t *testing.T) {
	input := [][]container.Stats{
		{
			{Name: "nginx", Cpu: 10.0, Mem: 100.0},
			{Name: "redis", Cpu: 5.0, Mem: 64.0},
		},
		{
			{Name: "nginx", Cpu: 20.0, Mem: 200.0},
			// redis not present
		},
	}

	result := records.AverageContainerStatsSlice(input)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	require.Len(t, result, 2)

	assert.Equal(t, "nginx", result[0].Name)
	assert.Equal(t, 15.0, result[0].Cpu)
	assert.Equal(t, 150.0, result[0].Mem)

	// redis: sum / count where count = total records (2), not records containing redis
	assert.Equal(t, "redis", result[1].Name)
	assert.Equal(t, 2.5, result[1].Cpu)
	assert.Equal(t, 32.0, result[1].Mem)
}

// Tests backward compatibility with deprecated NetworkSent/NetworkRecv (MB) when Bandwidth is zero.
func TestAverageContainerStatsSlice_DeprecatedNetworkFields(t *testing.T) {
	input := [][]container.Stats{
		{
			{Name: "nginx", Cpu: 10.0, Mem: 100.0, NetworkSent: 1.0, NetworkRecv: 2.0}, // 1 MB, 2 MB
		},
		{
			{Name: "nginx", Cpu: 20.0, Mem: 200.0, NetworkSent: 3.0, NetworkRecv: 4.0}, // 3 MB, 4 MB
		},
	}

	result := records.AverageContainerStatsSlice(input)

	require.Len(t, result, 1)
	assert.Equal(t, "nginx", result[0].Name)
	// avg sent = (1*1048576 + 3*1048576) / 2 = 2*1048576
	assert.Equal(t, uint64(2*1048576), result[0].Bandwidth[0])
	// avg recv = (2*1048576 + 4*1048576) / 2 = 3*1048576
	assert.Equal(t, uint64(3*1048576), result[0].Bandwidth[1])
}

// Tests that when Bandwidth is set, deprecated NetworkSent/NetworkRecv are ignored.
func TestAverageContainerStatsSlice_MixedBandwidthAndDeprecated(t *testing.T) {
	input := [][]container.Stats{
		{
			{Name: "nginx", Cpu: 10.0, Mem: 100.0, Bandwidth: [2]uint64{5000, 6000}, NetworkSent: 99.0, NetworkRecv: 99.0},
		},
		{
			{Name: "nginx", Cpu: 20.0, Mem: 200.0, Bandwidth: [2]uint64{7000, 8000}},
		},
	}

	result := records.AverageContainerStatsSlice(input)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(6000), result[0].Bandwidth[0])
	assert.Equal(t, uint64(7000), result[0].Bandwidth[1])
}

func TestAverageContainerStatsSlice_ThreeRecords(t *testing.T) {
	input := [][]container.Stats{
		{{Name: "app", Cpu: 1.0, Mem: 100.0}},
		{{Name: "app", Cpu: 2.0, Mem: 200.0}},
		{{Name: "app", Cpu: 3.0, Mem: 300.0}},
	}

	result := records.AverageContainerStatsSlice(input)

	require.Len(t, result, 1)
	assert.Equal(t, 2.0, result[0].Cpu)
	assert.Equal(t, 200.0, result[0].Mem)
}

func TestAverageContainerStatsSlice_ManyContainers(t *testing.T) {
	input := [][]container.Stats{
		{
			{Name: "a", Cpu: 10.0, Mem: 100.0},
			{Name: "b", Cpu: 20.0, Mem: 200.0},
			{Name: "c", Cpu: 30.0, Mem: 300.0},
			{Name: "d", Cpu: 40.0, Mem: 400.0},
		},
		{
			{Name: "a", Cpu: 20.0, Mem: 200.0},
			{Name: "b", Cpu: 30.0, Mem: 300.0},
			{Name: "c", Cpu: 40.0, Mem: 400.0},
			{Name: "d", Cpu: 50.0, Mem: 500.0},
		},
	}

	result := records.AverageContainerStatsSlice(input)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	require.Len(t, result, 4)
	assert.Equal(t, 15.0, result[0].Cpu)
	assert.Equal(t, 25.0, result[1].Cpu)
	assert.Equal(t, 35.0, result[2].Cpu)
	assert.Equal(t, 45.0, result[3].Cpu)
}
