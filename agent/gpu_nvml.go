//go:build amd64 && (windows || (linux && glibc))

package agent

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/henrygd/beszel/internal/entities/system"
)

// NVML constants and types
const (
	nvmlSuccess           int        = 0
	nvmlErrorNotFound     nvmlReturn = 6
	nvmlTotalPowerSamples int        = 0

	nvmlClockMem = 2 // NVML_CLOCK_MEM

	nvmlGpmMetricDramBwUtil  = 10 // NVML_GPM_METRIC_DRAM_BW_UTIL: % of theoretical DRAM bandwidth
	nvmlGpmMetricsGetVersion = 1  // NVML_GPM_METRICS_GET_VERSION
	nvmlGpmSupportVersion    = 1  // NVML_GPM_SUPPORT_VERSION
	// nvmlGpmMetricSlots is NVML_GPM_METRIC_MAX from nvml.h, the C size of
	// nvmlGpmMetricsGet_t.metrics. The Go array matches it so the struct is
	// ABI-exact and the driver can never write past it.
	nvmlGpmMetricSlots = 98
	// nvmlGpmMaxFailures consecutive GPM errors switch a device to the
	// utilization.memory fallback.
	nvmlGpmMaxFailures = 3
)

type nvmlDevice uintptr

type nvmlReturn int

type nvmlMemoryV1 struct {
	Total uint64
	Free  uint64
	Used  uint64
}

type nvmlMemoryV2 struct {
	Version  uint32
	Total    uint64
	Reserved uint64
	Free     uint64
	Used     uint64
}

type nvmlUtilization struct {
	Gpu    uint32
	Memory uint32
}

// nvmlSample mirrors nvmlSample_t: a timestamp plus an 8-byte nvmlValue_t union
type nvmlSample struct {
	TimeStamp   uint64
	SampleValue uint64
}

// nvmlGpmSample is an opaque nvmlGpmSample_t handle.
type nvmlGpmSample uintptr

type nvmlGpmSupport struct {
	Version           uint32
	IsSupportedDevice uint32
}

// nvmlGpmMetric mirrors nvmlGpmMetric_t (metricInfo is three char pointers).
type nvmlGpmMetric struct {
	MetricId   uint32
	NvmlReturn int32
	Value      float64
	ShortName  uintptr
	LongName   uintptr
	Unit       uintptr
}

// nvmlGpmMetricsGet mirrors nvmlGpmMetricsGet_t.
type nvmlGpmMetricsGet struct {
	Version    uint32
	NumMetrics uint32
	Sample1    nvmlGpmSample
	Sample2    nvmlGpmSample
	Metrics    [nvmlGpmMetricSlots]nvmlGpmMetric
}

type nvmlPciInfo struct {
	BusId          [16]byte
	Domain         uint32
	Bus            uint32
	Device         uint32
	PciDeviceId    uint32
	PciSubSystemId uint32
}

// NVML function signatures
var (
	nvmlInit                      func() nvmlReturn
	nvmlShutdown                  func() nvmlReturn
	nvmlDeviceGetCount            func(count *uint32) nvmlReturn
	nvmlDeviceGetHandleByIndex    func(index uint32, device *nvmlDevice) nvmlReturn
	nvmlDeviceGetName             func(device nvmlDevice, name *byte, length uint32) nvmlReturn
	nvmlDeviceGetMemoryInfo       func(device nvmlDevice, memory uintptr) nvmlReturn
	nvmlDeviceGetUtilizationRates func(device nvmlDevice, utilization *nvmlUtilization) nvmlReturn
	nvmlDeviceGetTemperature      func(device nvmlDevice, sensorType int, temp *uint32) nvmlReturn
	nvmlDeviceGetPowerUsage       func(device nvmlDevice, power *uint32) nvmlReturn
	nvmlDeviceGetPciInfo          func(device nvmlDevice, pci *nvmlPciInfo) nvmlReturn
	nvmlDeviceGetSamples          func(device nvmlDevice, sampleType int, lastSeenTimeStamp uint64, sampleValType *int32, sampleCount *uint32, samples *nvmlSample) nvmlReturn
	nvmlErrorString               func(result nvmlReturn) string
	// Optional symbols for memory bandwidth (nil when the driver lacks them)
	nvmlDeviceGetMaxClockInfo   func(device nvmlDevice, clockType int, clock *uint32) nvmlReturn
	nvmlDeviceGetMemoryBusWidth func(device nvmlDevice, busWidth *uint32) nvmlReturn
	nvmlGpmQueryDeviceSupport   func(device nvmlDevice, support *nvmlGpmSupport) nvmlReturn
	nvmlGpmSampleAlloc          func(sample *nvmlGpmSample) nvmlReturn
	nvmlGpmSampleFree           func(sample nvmlGpmSample) nvmlReturn
	nvmlGpmSampleGet            func(device nvmlDevice, sample nvmlGpmSample) nvmlReturn
	nvmlGpmMetricsGetFn         func(metrics *nvmlGpmMetricsGet) nvmlReturn
)

// nvmlMemBw holds per-device memory bandwidth state. Peak bandwidth and GPM
// support are detected once, on the first tick the device is active, so
// detection never wakes a suspended GPU.
type nvmlMemBw struct {
	detected bool
	peak     float64 // theoretical GB/s; 0 disables bandwidth for the device
	gpm      bool    // GPM DRAM_BW_UTIL available; otherwise approximate
	samples  [2]nvmlGpmSample
	cur      int  // index of the newest sample in samples
	hasPrev  bool // samples[cur] holds a valid sample
	failures int
}

type nvmlCollector struct {
	gm      *GPUManager
	lib     uintptr
	devices []nvmlDevice
	bdfs    []string
	isV2    bool
	memBw   []nvmlMemBw
	hasGpm  bool
	gpmReq  nvmlGpmMetricsGet
	// per-device state for the power sample fallback
	lastSampleTs []uint64
	lastPower    []float64
}

func (c *nvmlCollector) init() error {
	slog.Debug("NVML: Initializing")
	libPath := getNVMLPath()

	lib, err := openLibrary(libPath)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", libPath, err)
	}
	c.lib = lib

	purego.RegisterLibFunc(&nvmlInit, lib, "nvmlInit")
	purego.RegisterLibFunc(&nvmlShutdown, lib, "nvmlShutdown")
	purego.RegisterLibFunc(&nvmlDeviceGetCount, lib, "nvmlDeviceGetCount")
	purego.RegisterLibFunc(&nvmlDeviceGetHandleByIndex, lib, "nvmlDeviceGetHandleByIndex")
	purego.RegisterLibFunc(&nvmlDeviceGetName, lib, "nvmlDeviceGetName")
	// Try to get v2 memory info, fallback to v1 if not available
	if hasSymbol(lib, "nvmlDeviceGetMemoryInfo_v2") {
		c.isV2 = true
		purego.RegisterLibFunc(&nvmlDeviceGetMemoryInfo, lib, "nvmlDeviceGetMemoryInfo_v2")
	} else {
		purego.RegisterLibFunc(&nvmlDeviceGetMemoryInfo, lib, "nvmlDeviceGetMemoryInfo")
	}
	purego.RegisterLibFunc(&nvmlDeviceGetUtilizationRates, lib, "nvmlDeviceGetUtilizationRates")
	purego.RegisterLibFunc(&nvmlDeviceGetTemperature, lib, "nvmlDeviceGetTemperature")
	purego.RegisterLibFunc(&nvmlDeviceGetPowerUsage, lib, "nvmlDeviceGetPowerUsage")
	purego.RegisterLibFunc(&nvmlDeviceGetPciInfo, lib, "nvmlDeviceGetPciInfo")
	purego.RegisterLibFunc(&nvmlErrorString, lib, "nvmlErrorString")
	if hasSymbol(lib, "nvmlDeviceGetMaxClockInfo") && hasSymbol(lib, "nvmlDeviceGetMemoryBusWidth") {
		purego.RegisterLibFunc(&nvmlDeviceGetMaxClockInfo, lib, "nvmlDeviceGetMaxClockInfo")
		purego.RegisterLibFunc(&nvmlDeviceGetMemoryBusWidth, lib, "nvmlDeviceGetMemoryBusWidth")
	}
	if hasSymbol(lib, "nvmlGpmQueryDeviceSupport") && hasSymbol(lib, "nvmlGpmSampleAlloc") &&
		hasSymbol(lib, "nvmlGpmSampleFree") && hasSymbol(lib, "nvmlGpmSampleGet") && hasSymbol(lib, "nvmlGpmMetricsGet") {
		purego.RegisterLibFunc(&nvmlGpmQueryDeviceSupport, lib, "nvmlGpmQueryDeviceSupport")
		purego.RegisterLibFunc(&nvmlGpmSampleAlloc, lib, "nvmlGpmSampleAlloc")
		purego.RegisterLibFunc(&nvmlGpmSampleFree, lib, "nvmlGpmSampleFree")
		purego.RegisterLibFunc(&nvmlGpmSampleGet, lib, "nvmlGpmSampleGet")
		purego.RegisterLibFunc(&nvmlGpmMetricsGetFn, lib, "nvmlGpmMetricsGet")
		c.hasGpm = true
	}
	if hasSymbol(lib, "nvmlDeviceGetSamples") {
		purego.RegisterLibFunc(&nvmlDeviceGetSamples, lib, "nvmlDeviceGetSamples")
	}

	if ret := nvmlInit(); ret != nvmlReturn(nvmlSuccess) {
		return fmt.Errorf("nvmlInit failed: %v", ret)
	}

	var count uint32
	if ret := nvmlDeviceGetCount(&count); ret != nvmlReturn(nvmlSuccess) {
		return fmt.Errorf("nvmlDeviceGetCount failed: %v", ret)
	}

	for i := uint32(0); i < count; i++ {
		var device nvmlDevice
		if ret := nvmlDeviceGetHandleByIndex(i, &device); ret == nvmlReturn(nvmlSuccess) {
			c.devices = append(c.devices, device)
			// Get BDF for power state check
			var pci nvmlPciInfo
			if ret := nvmlDeviceGetPciInfo(device, &pci); ret == nvmlReturn(nvmlSuccess) {
				busID := string(pci.BusId[:])
				if idx := strings.Index(busID, "\x00"); idx != -1 {
					busID = busID[:idx]
				}
				c.bdfs = append(c.bdfs, strings.ToLower(busID))
			} else {
				c.bdfs = append(c.bdfs, "")
			}
			c.lastSampleTs = append(c.lastSampleTs, 0)
			c.lastPower = append(c.lastPower, 0)
		}
	}
	c.memBw = make([]nvmlMemBw, len(c.devices))

	return nil
}

func (c *nvmlCollector) start() {
	defer nvmlShutdown()
	defer c.freeGpmSamples()
	ticker := time.Tick(3 * time.Second)

	for range ticker {
		c.collect()
	}
}

func (c *nvmlCollector) collect() {
	c.gm.Lock()
	defer c.gm.Unlock()

	for i, device := range c.devices {
		id := fmt.Sprintf("%d", i)
		bdf := c.bdfs[i]

		// Update GPUDataMap
		if _, ok := c.gm.GpuDataMap[id]; !ok {
			var nameBuf [64]byte
			if ret := nvmlDeviceGetName(device, &nameBuf[0], 64); ret != nvmlReturn(nvmlSuccess) {
				continue
			}
			name := string(nameBuf[:strings.Index(string(nameBuf[:]), "\x00")])
			name = strings.TrimPrefix(name, "NVIDIA ")
			c.gm.GpuDataMap[id] = &system.GPUData{Name: strings.TrimSuffix(name, " Laptop GPU")}
		}
		gpu := c.gm.GpuDataMap[id]

		if bdf != "" && !c.isGPUActive(bdf) {
			slog.Debug("NVML: GPU is suspended, skipping", "bdf", bdf)
			c.memBw[i].hasPrev = false
			gpu.Temperature = 0
			gpu.MemoryUsed = 0
			continue
		}

		// Utilization
		var utilization nvmlUtilization
		if ret := nvmlDeviceGetUtilizationRates(device, &utilization); ret != nvmlReturn(nvmlSuccess) {
			slog.Debug("NVML: Utilization failed (GPU likely suspended)", "bdf", bdf, "ret", ret)
			c.memBw[i].hasPrev = false
			gpu.Temperature = 0
			gpu.MemoryUsed = 0
			continue
		}

		slog.Debug("NVML: Collecting data for GPU", "bdf", bdf)

		// Temperature
		var temp uint32
		nvmlDeviceGetTemperature(device, 0, &temp) // 0 is NVML_TEMPERATURE_GPU

		// Memory: only poll if GPU is active to avoid leaving D3cold state (#1522)
		if utilization.Gpu > 0 {
			var usedMem, totalMem uint64
			if c.isV2 {
				var memory nvmlMemoryV2
				memory.Version = 0x02000028 // (2 << 24) | 40 bytes
				if ret := nvmlDeviceGetMemoryInfo(device, uintptr(unsafe.Pointer(&memory))); ret != nvmlReturn(nvmlSuccess) {
					slog.Debug("NVML: MemoryInfo_v2 failed", "bdf", bdf, "ret", ret)
				} else {
					usedMem = memory.Used
					totalMem = memory.Total
				}
			} else {
				var memory nvmlMemoryV1
				if ret := nvmlDeviceGetMemoryInfo(device, uintptr(unsafe.Pointer(&memory))); ret != nvmlReturn(nvmlSuccess) {
					slog.Debug("NVML: MemoryInfo failed", "bdf", bdf, "ret", ret)
				} else {
					usedMem = memory.Used
					totalMem = memory.Total
				}
			}
			if totalMem > 0 {
				gpu.MemoryUsed = float64(usedMem) / 1024 / 1024 / mebibytesInAMegabyte
				gpu.MemoryTotal = float64(totalMem) / 1024 / 1024 / mebibytesInAMegabyte
			}
		} else {
			slog.Debug("NVML: Skipping memory info (utilization=0)", "bdf", bdf)
		}

		// Power
		var power uint32
		powerWatts := 0.0
		if ret := nvmlDeviceGetPowerUsage(device, &power); ret == nvmlReturn(nvmlSuccess) {
			powerWatts = float64(power) / 1000.0
		} else {
			// Some GPUs / drivers (e.g. RTX 4060 on 580.x) report power.draw as N/A
			// but still expose the driver's power sample buffer
			powerWatts = c.powerFromSamples(i, device)
		}

		// Memory bandwidth
		if bw, approx, ok := c.memBandwidth(i, device, utilization.Memory); ok {
			gpu.MemBandwidth += bw
			gpu.MemBandwidthCount++
			gpu.MemBandwidthPeak = c.memBw[i].peak
			gpu.MemBandwidthApprox = approx
		}

		gpu.Temperature = float64(temp)
		gpu.Usage += float64(utilization.Gpu)
		gpu.Power += powerWatts
		gpu.Count++
		slog.Debug("NVML: Collected data", "gpu", gpu)
	}
}

// detectMemBw computes peak bandwidth and GPM support for one device.
func (c *nvmlCollector) detectMemBw(i int, device nvmlDevice) {
	state := &c.memBw[i]
	state.detected = true
	if nvmlDeviceGetMaxClockInfo == nil || nvmlDeviceGetMemoryBusWidth == nil {
		return
	}
	var clockMHz, busWidth uint32
	if ret := nvmlDeviceGetMaxClockInfo(device, nvmlClockMem, &clockMHz); ret != nvmlReturn(nvmlSuccess) {
		slog.Debug("NVML: max memory clock unavailable", "gpu", i, "ret", ret)
		return
	}
	if ret := nvmlDeviceGetMemoryBusWidth(device, &busWidth); ret != nvmlReturn(nvmlSuccess) {
		slog.Debug("NVML: memory bus width unavailable", "gpu", i, "ret", ret)
		return
	}
	state.peak = nvmlPeakMemBandwidthGBps(clockMHz, busWidth)
	if state.peak == 0 || !c.hasGpm {
		return
	}
	support := nvmlGpmSupport{Version: nvmlGpmSupportVersion}
	if ret := nvmlGpmQueryDeviceSupport(device, &support); ret != nvmlReturn(nvmlSuccess) || support.IsSupportedDevice == 0 {
		slog.Debug("NVML: GPM unsupported, memory bandwidth is approximate", "gpu", i, "ret", ret)
		return
	}
	for j := range state.samples {
		if ret := nvmlGpmSampleAlloc(&state.samples[j]); ret != nvmlReturn(nvmlSuccess) {
			slog.Debug("NVML: GPM sample alloc failed", "gpu", i, "ret", ret)
			c.disableGpm(state)
			return
		}
	}
	state.gpm = true
	slog.Debug("NVML: memory bandwidth", "gpu", i, "peakGBps", state.peak, "clockMHz", clockMHz, "busWidth", busWidth, "gpm", state.gpm)
}

// memBandwidth returns this tick's bandwidth sample in GB/s. approx is true
// when derived from memory busy time instead of GPM DRAM throughput. ok is
// false when no sample is available (unsupported, first GPM tick, or error).
func (c *nvmlCollector) memBandwidth(i int, device nvmlDevice, memUtil uint32) (bw float64, approx bool, ok bool) {
	state := &c.memBw[i]
	if !state.detected {
		c.detectMemBw(i, device)
	}
	if state.peak == 0 {
		return 0, false, false
	}
	if !state.gpm {
		return gpuMemBandwidthFromUtil(float64(memUtil), state.peak), true, true
	}

	next := 1 - state.cur
	if ret := nvmlGpmSampleGet(device, state.samples[next]); ret != nvmlReturn(nvmlSuccess) {
		c.gpmFailed(i, state, ret)
		return 0, false, false
	}
	if !state.hasPrev {
		state.cur, state.hasPrev = next, true
		return 0, false, false
	}
	req := &c.gpmReq
	*req = nvmlGpmMetricsGet{
		Version:    nvmlGpmMetricsGetVersion,
		NumMetrics: 1,
		Sample1:    state.samples[state.cur],
		Sample2:    state.samples[next],
	}
	req.Metrics[0].MetricId = nvmlGpmMetricDramBwUtil
	state.cur = next
	if ret := nvmlGpmMetricsGetFn(req); ret != nvmlReturn(nvmlSuccess) || req.Metrics[0].NvmlReturn != int32(nvmlSuccess) {
		if ret == nvmlReturn(nvmlSuccess) {
			ret = nvmlReturn(req.Metrics[0].NvmlReturn)
		}
		c.gpmFailed(i, state, ret)
		return 0, false, false
	}
	// A non-finite or negative DRAM_BW_UTIL would poison the MemBandwidth
	// accumulator for the life of the process (NaN - NaN stays NaN), and one
	// NaN in Stats makes the hub store the whole record as null.
	if v := req.Metrics[0].Value; math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		slog.Debug("NVML: GPM DRAM_BW_UTIL not finite", "gpu", i, "value", v)
		c.gpmFailed(i, state, nvmlReturn(nvmlSuccess))
		return 0, false, false
	}
	state.failures = 0
	return gpuMemBandwidthFromUtil(req.Metrics[0].Value, state.peak), false, true
}

func (c *nvmlCollector) gpmFailed(i int, state *nvmlMemBw, ret nvmlReturn) {
	state.hasPrev = false
	state.failures++
	slog.Debug("NVML: GPM sample failed", "gpu", i, "ret", ret, "failures", state.failures)
	if state.failures >= nvmlGpmMaxFailures {
		slog.Debug("NVML: GPM disabled, memory bandwidth is approximate", "gpu", i)
		c.disableGpm(state)
	}
}

// disableGpm frees a device's GPM samples and switches it to the fallback.
func (c *nvmlCollector) disableGpm(state *nvmlMemBw) {
	for j, sample := range state.samples {
		if sample != 0 {
			nvmlGpmSampleFree(sample)
			state.samples[j] = 0
		}
	}
	state.gpm, state.hasPrev = false, false
}

func (c *nvmlCollector) freeGpmSamples() {
	for i := range c.memBw {
		if c.memBw[i].gpm {
			c.disableGpm(&c.memBw[i])
		}
	}
}

// powerFromSamples returns the average power in watts of the NVML power samples
// recorded since the previous call, or the last known value if there are none.
func (c *nvmlCollector) powerFromSamples(i int, device nvmlDevice) float64 {
	if nvmlDeviceGetSamples == nil {
		return 0
	}
	var valType int32
	var count uint32
	// first call with nil buffer returns the maximum number of samples
	if ret := nvmlDeviceGetSamples(device, nvmlTotalPowerSamples, c.lastSampleTs[i], &valType, &count, nil); ret != nvmlReturn(nvmlSuccess) || count == 0 {
		if ret != nvmlErrorNotFound {
			slog.Debug("NVML: power samples unavailable", "ret", ret)
		}
		return c.lastPower[i]
	}
	samples := make([]nvmlSample, count)
	if ret := nvmlDeviceGetSamples(device, nvmlTotalPowerSamples, c.lastSampleTs[i], &valType, &count, &samples[0]); ret != nvmlReturn(nvmlSuccess) || count == 0 {
		if ret != nvmlErrorNotFound {
			slog.Debug("NVML: power samples failed", "ret", ret)
		}
		return c.lastPower[i]
	}
	var sum float64
	var n int
	for _, s := range samples[:count] {
		v, ok := nvmlSampleToFloat(valType, s.SampleValue)
		if !ok {
			continue
		}
		sum += v
		n++
		if s.TimeStamp > c.lastSampleTs[i] {
			c.lastSampleTs[i] = s.TimeStamp
		}
	}
	if n > 0 {
		c.lastPower[i] = sum / float64(n) / 1000.0 // milliwatts -> watts
	}
	return c.lastPower[i]
}

// nvmlSampleToFloat decodes an nvmlValue_t union according to its nvmlValueType_t
func nvmlSampleToFloat(valType int32, raw uint64) (float64, bool) {
	switch valType {
	case 0: // NVML_VALUE_TYPE_DOUBLE
		return math.Float64frombits(raw), true
	case 1: // NVML_VALUE_TYPE_UNSIGNED_INT
		return float64(uint32(raw)), true
	case 2, 3: // NVML_VALUE_TYPE_UNSIGNED_LONG, NVML_VALUE_TYPE_UNSIGNED_LONG_LONG
		return float64(raw), true
	case 4: // NVML_VALUE_TYPE_SIGNED_LONG_LONG
		return float64(int64(raw)), true
	case 5: // NVML_VALUE_TYPE_SIGNED_INT
		return float64(int32(uint32(raw))), true
	case 6: // NVML_VALUE_TYPE_UNSIGNED_SHORT
		return float64(uint16(raw)), true
	}
	return 0, false
}
