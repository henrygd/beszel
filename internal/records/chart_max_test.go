//go:build testing

package records_test

import (
	"encoding/json"
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/records"
	"github.com/stretchr/testify/require"
)

func TestChartPeaksSurviveRetention(t *testing.T) {
	a := system.Stats{Cpu: 80, Temperatures: map[string]float64{"cpu": 80, "cold": -20}, Fans: map[string]uint16{"fan": 900}, WiFi: map[string]int8{"wifi": -40}, LoadAvg: [3]float64{8, 4, 2}, CpuBreakdown: []float64{80, 10}, CpuCoresUsage: system.Uint8Slice{90, 20}, MemBytes: [4]uint64{1000, 800, 100, 10}, SwapBytes: [2]uint64{500, 300}, DiskUsed: 80, Battery: system.Battery{90, 1}, Batteries: map[string]uint8{"battery": 90}, NetworkInterfaces: map[string][4]uint64{"eth": {800, 400, 1000, 2000}}, ExtraFs: map[string]*system.FsStats{"disk": {DiskTotal: 100, DiskUsed: 80}}, GPUData: map[string]system.GPUData{"gpu": {Usage: 90, Power: 200, PowerPkg: 250, MemoryUsed: 800, Engines: map[string]float64{"render": 70}}}, ZfsPools: map[string]*system.ZfsPool{"pool": {Total: 100, Used: 80, ReadBytes: 800, WriteBytes: 400}}}
	b := system.Stats{Cpu: 20, Temperatures: map[string]float64{"cpu": 20, "cold": -30, "new": -10}, WiFi: map[string]int8{"wifi": -80}, LoadAvg: [3]float64{2, 1, 0}, MemBytes: [4]uint64{1000, 200, 20, 2}, SwapBytes: [2]uint64{500, 100}, Battery: system.Battery{30, 0}}
	original, err := json.Marshal(a)
	require.NoError(t, err)
	avg := records.AverageSystemStatsSlice([]system.Stats{a, b})
	require.Equal(t, 50.0, avg.Temperatures["cpu"])
	require.Equal(t, 80.0, avg.ChartMax.Temperatures["cpu"])
	require.Equal(t, -20.0, avg.ChartMax.Temperatures["cold"])
	require.Equal(t, -10.0, avg.ChartMax.Temperatures["new"])
	require.Equal(t, int8(-40), avg.ChartMax.WiFi["wifi"])
	require.Equal(t, uint64(300), avg.ChartMax.SwapBytes[1])
	require.Equal(t, uint64(100), avg.ChartMax.MemBytes[2])
	require.Equal(t, float64(250), avg.ChartMax.GPUData["gpu"].PowerPkg)
	require.Equal(t, float64(70), avg.ChartMax.GPUData["gpu"].Engines["render"])
	require.Equal(t, uint64(800), avg.ChartMax.NetworkInterfaces["eth"][0])
	require.Equal(t, float64(80), avg.ChartMax.ExtraFs["disk"].DiskUsed)
	require.Equal(t, uint64(800), avg.ChartMax.ZfsPools["pool"].ReadBytes)
	encoded, err := json.Marshal(avg)
	require.NoError(t, err)
	without := avg
	without.ChartMax = nil
	baseline, err := json.Marshal(without)
	require.NoError(t, err)
	t.Logf("representative JSON bytes: average=%d with peaks=%d extra=%d", len(baseline), len(encoded), len(encoded)-len(baseline))
	var restored system.Stats
	require.NoError(t, json.Unmarshal(encoded, &restored))
	later := records.AverageSystemStatsSlice([]system.Stats{restored, b})
	require.Equal(t, avg.ChartMax.Temperatures, later.ChartMax.Temperatures)
	require.Equal(t, avg.ChartMax.Fans, later.ChartMax.Fans)
	require.Equal(t, avg.ChartMax.CpuBreakdown, later.ChartMax.CpuBreakdown)
	require.Equal(t, avg.ChartMax.CpuCoresUsage, later.ChartMax.CpuCoresUsage)
	require.Equal(t, avg.ChartMax.LoadAvg, later.ChartMax.LoadAvg)
	require.Equal(t, avg.ChartMax.Batteries, later.ChartMax.Batteries)
	after, err := json.Marshal(a)
	require.NoError(t, err)
	require.Equal(t, original, after)
	// Returned maps/slices must not alias input snapshots either.
	later.ChartMax.GPUData["gpu"].Engines["render"] = 0
	require.Equal(t, 70.0, restored.ChartMax.GPUData["gpu"].Engines["render"])
}

func TestContainerPeaksSurviveRetention(t *testing.T) {
	avg := records.AverageContainerStatsSlice([][]container.Stats{{{Name: "demo", Cpu: 80, Mem: 800, NetworkSent: 2}}, {{Name: "demo", Cpu: 20, Mem: 200, Bandwidth: [2]uint64{100, 400}}}})
	require.Equal(t, 50.0, avg[0].Cpu)
	require.Equal(t, 80.0, avg[0].Max.Cpu)
	require.Equal(t, 800.0, avg[0].Max.Mem)
	require.Equal(t, uint64(2*1024*1024), avg[0].Max.Bandwidth[0])
	later := records.AverageContainerStatsSlice([][]container.Stats{avg, {{Name: "demo", Cpu: 10}}})
	require.Equal(t, avg[0].Max, later[0].Max)
}
