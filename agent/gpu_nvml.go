//go:build amd64 && (windows || (linux && glibc))

package agent

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/henrygd/beszel/internal/entities/system"
)

// NVML constants and types
const (
	nvmlSuccess int = 0
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
	nvmlErrorString               func(result nvmlReturn) string
)

type nvmlCollector struct {
	gm      *GPUManager
	lib     uintptr
	devices []nvmlDevice
	bdfs    []string
	isV2    bool
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
		nvmlDeviceGetPowerUsage(device, &power)

		gpu.Temperature = float64(temp)
		gpu.Usage += float64(utilization.Gpu)
		gpu.Power += float64(power) / 1000.0
		gpu.Count++
		slog.Debug("NVML: Collected data", "gpu", gpu)
	}
}
