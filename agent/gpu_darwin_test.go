//go:build darwin

package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const powermetricsRealSample = `Machine model: Mac14,10
OS version: 25D125

**** GPU usage ****
GPU HW active frequency: 444 MHz
GPU HW active residency:   0.00% (444 MHz: .00% 612 MHz:   0% 808 MHz:   0% 968 MHz:   0% 1110 MHz:   0% 1236 MHz:   0% 1338 MHz:   0% 1398 MHz:   0%)
GPU SW requested state: (P1 : 100% P2 :   0% P3 :   0% P4 :   0% P5 :   0% P6 :   0% P7 :   0% P8 :   0%)
GPU idle residency:  100.00%
GPU Power: 0 mW
`

func newDarwinGPUManager() *GPUManager {
	return &GPUManager{
		GpuDataMap:    make(map[string]*system.GPUData),
		lastAvgData:   make(map[string]system.GPUData),
		lastSnapshots: make(map[uint16]map[string]*gpuSnapshot),
	}
}

func writeAppleCommand(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return path
}

func requireProcessGone(t *testing.T, pidFile string) {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	out, _ := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "pid=").Output()
	assert.Empty(t, strings.TrimSpace(string(out)), "collector child %d should be killed and reaped", pid)
}

func TestParsePowermetricsDataRealZeroSampleIsValid(t *testing.T) {
	gm := newDarwinGPUManager()
	require.True(t, gm.parsePowermetricsData([]byte(powermetricsRealSample)))

	gpu, ok := gm.GpuDataMap[appleGPUID]
	require.True(t, ok)
	assert.Equal(t, "Apple GPU", gpu.Name)
	assert.Equal(t, 0.0, gpu.Usage)
	assert.Equal(t, 0.0, gpu.Power)
	assert.Equal(t, 1.0, gpu.Count)
}

func TestParsePowermetricsDataValidValuesAndUnits(t *testing.T) {
	for _, power := range []string{"1.25 W", "1250 mW", "0.00125 kW"} {
		gm := newDarwinGPUManager()
		input := `GPU HW active residency: 12.50% (444 MHz: 12.50%)
GPU idle residency: 87.50%
GPU Power: ` + power
		require.True(t, gm.parsePowermetricsData([]byte(input)))
		gpu := gm.GpuDataMap[appleGPUID]
		assert.InDelta(t, 12.5, gpu.Usage, 0.001)
		assert.InDelta(t, 1.25, gpu.Power, 0.001)
		assert.Equal(t, 1.0, gpu.Count)
	}
}

func TestParsePowermetricsDataRejectsInvalidAndIncompleteSamples(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "power only", input: "GPU Power: 120 mW"},
		{name: "usage only", input: "GPU HW active residency: 0.00%"},
		{name: "missing percent unit", input: "GPU HW active residency: 0.00\nGPU Power: 1 W"},
		{name: "missing unit", input: "GPU HW active residency: 0.00%\nGPU Power: 1"},
		{name: "negative usage", input: "GPU HW active residency: -0.01%\nGPU Power: 0 mW"},
		{name: "usage above range", input: "GPU HW active residency: 100.01%\nGPU Power: 0 mW"},
		{name: "nonfinite usage", input: "GPU HW active residency: NaN%\nGPU Power: 0 mW"},
		{name: "nonfinite usage infinity", input: "GPU HW active residency: +Inf%\nGPU Power: 0 mW"},
		{name: "negative power", input: "GPU HW active residency: 0.00%\nGPU Power: -1 mW"},
		{name: "nonfinite power", input: "GPU HW active residency: 0.00%\nGPU Power: NaN mW"},
		{name: "overflowed power conversion", input: "GPU HW active residency: 0.00%\nGPU Power: 1e308 kW"},
		{name: "unknown power unit", input: "GPU HW active residency: 0.00%\nGPU Power: 1 jW"},
		{name: "empty", input: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := newDarwinGPUManager()
			assert.False(t, gm.parsePowermetricsData([]byte(tt.input)))
			assert.Empty(t, gm.GpuDataMap)
		})
	}
}

func TestParsePowermetricsPowerOnlyDoesNotDiluteExistingCount(t *testing.T) {
	gm := newDarwinGPUManager()
	gm.GpuDataMap[appleGPUID] = &system.GPUData{Name: "Apple GPU", Usage: 2, Power: 4, Count: 2}
	before := *gm.GpuDataMap[appleGPUID]
	assert.False(t, gm.parsePowermetricsData([]byte("GPU Power: 120 mW")))
	assert.Equal(t, before, *gm.GpuDataMap[appleGPUID])
}

func TestParseMacmonLineRealZeroSampleIsValid(t *testing.T) {
	gm := newDarwinGPUManager()
	input := `{"all_power":0.0,"ane_power":0.0,"cpu_power":0.0,"ecpu_usage":[2061,0.0],"gpu_power":0.0,"gpu_ram_power":0.0,"gpu_usage":[503,0.0],"memory":{"ram_total":17179869184,"ram_usage":12322914304,"swap_total":0,"swap_usage":0},"pcpu_usage":[1248,0.0],"ram_power":0.0,"sys_power":0.0,"temp":{"cpu_temp_avg":23.0,"gpu_temp_avg":29.445},"timestamp":"2026-02-17T19:34:27.942556+00:00"}`
	require.True(t, gm.parseMacmonLine([]byte(input)))
	gpu, ok := gm.GpuDataMap[appleGPUID]
	require.True(t, ok)
	assert.Equal(t, 0.0, gpu.Usage)
	assert.Equal(t, 0.0, gpu.Power)
	assert.Equal(t, 1.0, gpu.Count)
	assert.Equal(t, 29.445, gpu.Temperature)
	assert.Zero(t, gpu.MemoryUsed)
	assert.Zero(t, gpu.MemoryTotal)
}

func TestParseMacmonLineConvertsRealRatioAndPower(t *testing.T) {
	gm := newDarwinGPUManager()
	input := `{"gpu_power":1.25,"gpu_ram_power":0.5,"gpu_usage":[503,0.25],"temp":{"gpu_temp_avg":42.5}}`
	require.True(t, gm.parseMacmonLine([]byte(input)))
	gpu := gm.GpuDataMap[appleGPUID]
	assert.InDelta(t, 25.0, gpu.Usage, 0.001)
	assert.InDelta(t, 1.75, gpu.Power, 0.001)
	assert.Equal(t, 1.0, gpu.Count)
}

func TestParseMacmonLineRejectsMissingNullMalformedAndOutOfRangeFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing usage", input: `{"gpu_power":0}`},
		{name: "null usage", input: `{"gpu_usage":null,"gpu_power":0}`},
		{name: "null frequency", input: `{"gpu_usage":[null,0.25],"gpu_power":1}`},
		{name: "null usage element", input: `{"gpu_usage":[503,null],"gpu_power":0}`},
		{name: "short usage array", input: `{"gpu_usage":[503],"gpu_power":0}`},
		{name: "usage above ratio", input: `{"gpu_usage":[503,1.01],"gpu_power":0}`},
		{name: "negative usage ratio", input: `{"gpu_usage":[503,-0.01],"gpu_power":0}`},
		{name: "nonfinite usage", input: `{"gpu_usage":[503,1e999],"gpu_power":0}`},
		{name: "missing power", input: `{"gpu_usage":[503,0]}`},
		{name: "null power", input: `{"gpu_usage":[503,0],"gpu_power":null}`},
		{name: "negative power", input: `{"gpu_usage":[503,0],"gpu_power":-1}`},
		{name: "nonfinite power", input: `{"gpu_usage":[503,0],"gpu_power":1e999}`},
		{name: "invalid ram power", input: `{"gpu_usage":[503,0],"gpu_power":0,"gpu_ram_power":-1}`},
		{name: "malformed json", input: `{"gpu_usage":[503,0]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := newDarwinGPUManager()
			assert.False(t, gm.parseMacmonLine([]byte(tt.input)))
			assert.Empty(t, gm.GpuDataMap)
		})
	}
}

func TestAppleGPUDataIsRemovedAfterStaleGap(t *testing.T) {
	gm := newDarwinGPUManager()
	input := `{"gpu_power":1.25,"gpu_ram_power":0.5,"gpu_usage":[503,0.25],"temp":{"gpu_temp_avg":42.5}}`
	require.True(t, gm.parseMacmonLine([]byte(input)))
	cacheKey := uint16(5000)
	data := gm.GetCurrentData(cacheKey)
	require.Contains(t, data, appleGPUID)
	require.Contains(t, gm.lastAvgData, appleGPUID)
	require.Contains(t, gm.lastSnapshots[cacheKey], appleGPUID)
	require.Contains(t, gm.GetCurrentData(60000), appleGPUID)

	gm.Lock()
	gm.appleGPULastSample = time.Now().Add(-appleGPUDataStaleAfter - time.Second)
	gm.Unlock()
	data = gm.GetCurrentData(cacheKey)
	assert.Empty(t, data)
	assert.NotContains(t, gm.GpuDataMap, appleGPUID)
	assert.NotContains(t, gm.lastAvgData, appleGPUID)
	assert.NotContains(t, gm.lastSnapshots[cacheKey], appleGPUID)
	assert.NotContains(t, gm.lastSnapshots[60000], appleGPUID)

	require.True(t, gm.parseMacmonLine([]byte(`{"gpu_usage":[503,0.5],"gpu_power":2}`)))
	data = gm.GetCurrentData(cacheKey)
	assert.Equal(t, 50.0, data[appleGPUID].Usage)
	assert.Equal(t, 2.0, data[appleGPUID].Power)
}

func TestAppleGPUDataIsRemovedWhenPowermetricsCollectorStops(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	writeAppleCommand(t, dir, powermetricsCmd, "exit 7")
	gm := newDarwinGPUManager()
	require.True(t, gm.parsePowermetricsData([]byte(powermetricsRealSample)))
	gm.lastAvgData[appleGPUID] = system.GPUData{Name: "Apple GPU"}
	gm.lastSnapshots[1] = map[string]*gpuSnapshot{appleGPUID: {count: 1}}

	gm.runPowermetricsCollector(context.Background(), 2*time.Second, 0)
	assert.NotContains(t, gm.GpuDataMap, appleGPUID)
	assert.NotContains(t, gm.lastAvgData, appleGPUID)
	assert.NotContains(t, gm.lastSnapshots[1], appleGPUID)
}

func TestCollectPowermetricsTimeoutKillsAndWaits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	pidFile := filepath.Join(dir, "pid")
	writeAppleCommand(t, dir, powermetricsCmd, `printf '%s\n' "$$" > `+strconv.Quote(pidFile)+`; exec /bin/sleep 10`)
	gm := newDarwinGPUManager()

	start := time.Now()
	err := gm.collectPowermetricsWithTimeout(context.Background(), 2*time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 3*time.Second)
	requireProcessGone(t, pidFile)
	assert.Empty(t, gm.GpuDataMap)
}

func TestCollectMacmonHealthTimeoutKillsAndWaits(t *testing.T) {
	tests := []struct {
		name    string
		first   string
		wantErr error
	}{
		{name: "no valid sample and bad JSON", first: `printf '%s\n' 'bad json'`, wantErr: errNoValidData},
		{name: "valid sample then stuck", first: `printf '%s\n' '{"gpu_usage":[503,0],"gpu_power":0}'`, wantErr: errNoValidData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			pidFile := filepath.Join(dir, "pid")
			writeAppleCommand(t, dir, macmonCmd, `printf '%s\n' "$$" > `+strconv.Quote(pidFile)+`; `+tt.first+`; printf '%s\n' 'stderr noise' >&2; exec /bin/sleep 10`)
			gm := newDarwinGPUManager()

			start := time.Now()
			err := gm.collectMacmonPipeWithHealthTimeout(context.Background(), 2*time.Second)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Less(t, time.Since(start), 3*time.Second)
			requireProcessGone(t, pidFile)
			if tt.name == "valid sample then stuck" {
				assert.Equal(t, 1.0, gm.GpuDataMap[appleGPUID].Count)
			}
		})
	}
}

func TestCollectMacmonValidSamplesResetHealthTimeout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	writeAppleCommand(t, dir, macmonCmd, `printf '%s\n' '{"gpu_usage":[503,0],"gpu_power":0}'
/bin/sleep 0.2
printf '%s\n' '{"gpu_usage":[503,0.25],"gpu_power":1}'
/bin/sleep 0.2
printf '%s\n' '{"gpu_usage":[503,0.5],"gpu_power":2}'`)
	gm := newDarwinGPUManager()
	require.NoError(t, gm.collectMacmonPipeWithHealthTimeout(context.Background(), 300*time.Millisecond))
	assert.Equal(t, 3.0, gm.GpuDataMap[appleGPUID].Count)
}

func TestCollectMacmonCancellationKillsAndWaits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	pidFile := filepath.Join(dir, "pid")
	writeAppleCommand(t, dir, macmonCmd, `printf '%s\n' "$$" > `+strconv.Quote(pidFile)+`; exec /bin/sleep 10`)
	gm := newDarwinGPUManager()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, gm.collectMacmonPipeWithHealthTimeout(ctx, 5*time.Second), context.DeadlineExceeded)
	requireProcessGone(t, pidFile)
}

func TestAppleGPUDataIsRemovedWhenMacmonCollectorStops(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	writeAppleCommand(t, dir, macmonCmd, `printf '%s\n' 'bad json'; exec /bin/sleep 10`)
	gm := newDarwinGPUManager()
	require.True(t, gm.parseMacmonLine([]byte(`{"gpu_usage":[503,0],"gpu_power":0}`)))
	gm.lastAvgData[appleGPUID] = system.GPUData{Name: "Apple GPU"}
	gm.lastSnapshots[1] = map[string]*gpuSnapshot{appleGPUID: {count: 1}}

	gm.runMacmonCollector(context.Background(), 20*time.Millisecond, 0)
	assert.NotContains(t, gm.GpuDataMap, appleGPUID)
	assert.NotContains(t, gm.lastAvgData, appleGPUID)
	assert.NotContains(t, gm.lastSnapshots[1], appleGPUID)
}

func TestAppleGPUKeepsFreshAverageBetweenSamples(t *testing.T) {
	gm := newDarwinGPUManager()
	require.True(t, gm.parsePowermetricsData([]byte("GPU HW active residency: 12.50%\nGPU Power: 1.25 W\n")))
	first := gm.GetCurrentData(60000)
	require.Contains(t, first, appleGPUID)
	require.Equal(t, 12.5, first[appleGPUID].Usage)
	require.Equal(t, 1.25, first[appleGPUID].Power)
	second := gm.GetCurrentData(60000)
	require.Contains(t, second, appleGPUID)
	require.Equal(t, first[appleGPUID].Usage, second[appleGPUID].Usage, "retain fresh averages between collector updates")
	require.Equal(t, first[appleGPUID].Power, second[appleGPUID].Power)
}

func TestAppleGPUKeepsAverageForEachCacheKey(t *testing.T) {
	gm := newDarwinGPUManager()
	require.True(t, gm.parsePowermetricsData([]byte("GPU HW active residency: 10%\nGPU Power: 1 W\n")))
	gm.GetCurrentData(5000)
	require.True(t, gm.parsePowermetricsData([]byte("GPU HW active residency: 30%\nGPU Power: 3 W\n")))
	longWindow := gm.GetCurrentData(60000)
	shortWindow := gm.GetCurrentData(5000)
	require.Equal(t, 20.0, longWindow[appleGPUID].Usage)
	require.Equal(t, 30.0, shortWindow[appleGPUID].Usage)

	for range 3 {
		longWindow = gm.GetCurrentData(60000)
		shortWindow = gm.GetCurrentData(5000)
		assert.Equal(t, 20.0, longWindow[appleGPUID].Usage)
		assert.Equal(t, 2.0, longWindow[appleGPUID].Power)
		assert.Equal(t, 30.0, shortWindow[appleGPUID].Usage)
		assert.Equal(t, 3.0, shortWindow[appleGPUID].Power)
	}
}

func TestParseMacmonRejectsOverflowedTotalPower(t *testing.T) {
	gm := newDarwinGPUManager()
	require.False(t, gm.parseMacmonLine([]byte(`{"gpu_usage":[503,0.25],"gpu_power":1e308,"gpu_ram_power":1e308}`)))
	require.Empty(t, gm.GpuDataMap)
}

// GPU-only samples captured from macmon 0.9.0 on Apple Silicon.
func TestParseMacmonSamplesReachGPUWireFormat(t *testing.T) {
	gm := newDarwinGPUManager()
	for _, input := range []string{
		`{"gpu_usage":[939,0.5949013829231262],"gpu_power":5.272632598876953,"gpu_ram_power":0,"temp":{"gpu_temp_avg":66.88365936279297}}`,
		`{"gpu_usage":[990,0.6064913272857666],"gpu_power":5.506616115570068,"gpu_ram_power":0,"temp":{"gpu_temp_avg":66.91873931884766}}`,
		`{"gpu_usage":[963,0.6104052662849426],"gpu_power":5.40929651260376,"gpu_ram_power":0,"temp":{"gpu_temp_avg":66.91873931884766}}`,
	} {
		require.True(t, gm.parseMacmonLine([]byte(input)))
	}
	data := gm.GetCurrentData(60000)
	require.Contains(t, data, appleGPUID)
	require.InDelta(t, 60.39, data[appleGPUID].Usage, 0.01)
	require.InDelta(t, 5.4, data[appleGPUID].Power, 0.01)
	wire, err := json.Marshal(data[appleGPUID])
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(wire, &fields))
	require.Contains(t, fields, "u")
	require.Contains(t, fields, "p")
	require.NotContains(t, fields, "mu", "unified memory must not be presented as VRAM")
	require.NotContains(t, fields, "mt")
}
