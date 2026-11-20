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
)

type nvmlCollector struct {
	gm      *GPUManager
	lib     uintptr
	devices []nvmlDevice
	bdfs    []string
	isV2    bool
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

	return nil
}

func (c *nvmlCollector) start() {
	defer nvmlShutdown()
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
			gpu.Temperature = 0
			gpu.MemoryUsed = 0
			continue
		}

		// Utilization
		var utilization nvmlUtilization
		if ret := nvmlDeviceGetUtilizationRates(device, &utilization); ret != nvmlReturn(nvmlSuccess) {
			slog.Debug("NVML: Utilization failed (GPU likely suspended)", "bdf", bdf, "ret", ret)
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

		gpu.Temperature = float64(temp)
		gpu.Usage += float64(utilization.Gpu)
		gpu.Power += powerWatts
		gpu.Count++
		slog.Debug("NVML: Collected data", "gpu", gpu)
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
