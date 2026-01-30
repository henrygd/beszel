//go:build linux

package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

// hasAmdSysfs returns true if any AMD GPU sysfs nodes are found
func (gm *GPUManager) hasAmdSysfs() bool {
	cards, err := filepath.Glob("/sys/class/drm/card*/device/vendor")
	if err != nil {
		return false
	}
	for _, vendorPath := range cards {
		vendor, err := os.ReadFile(vendorPath)
		if err == nil && strings.TrimSpace(string(vendor)) == "0x1002" {
			return true
		}
	}
	return false
}

// collectAmdStats collects AMD GPU metrics directly from sysfs to avoid the overhead of rocm-smi
func (gm *GPUManager) collectAmdStats() error {
	cards, err := filepath.Glob("/sys/class/drm/card*")
	if err != nil {
		return err
	}

	var amdGpuPaths []string
	for _, card := range cards {
		// Ignore symbolic links and non-main card directories
		if strings.Contains(filepath.Base(card), "-") || !isAmdGpu(card) {
			continue
		}
		amdGpuPaths = append(amdGpuPaths, card)
	}

	if len(amdGpuPaths) == 0 {
		return errNoValidData
	}

	slog.Debug("Using sysfs for AMD GPU data collection")

	failures := 0
	for {
		hasData := false
		for _, cardPath := range amdGpuPaths {
			if gm.updateAmdGpuData(cardPath) {
				hasData = true
			}
		}
		if !hasData {
			failures++
			if failures > maxFailureRetries {
				return errNoValidData
			}
			slog.Warn("No AMD GPU data from sysfs", "failures", failures)
			time.Sleep(retryWaitTime)
			continue
		}
		failures = 0
		time.Sleep(rocmSmiInterval)
	}
}

func isAmdGpu(cardPath string) bool {
	vendorPath := filepath.Join(cardPath, "device/vendor")
	vendor, err := os.ReadFile(vendorPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(vendor)) == "0x1002"
}

// updateAmdGpuData reads GPU metrics from sysfs and updates the GPU data map.
// Returns true if at least some data was successfully read.
func (gm *GPUManager) updateAmdGpuData(cardPath string) bool {
	devicePath := filepath.Join(cardPath, "device")
	id := filepath.Base(cardPath)

	// Read all sysfs values first (no lock needed - these can be slow)
	usage, usageErr := readSysfsFloat(filepath.Join(devicePath, "gpu_busy_percent"))
	memUsed, memUsedErr := readSysfsFloat(filepath.Join(devicePath, "mem_info_vram_used"))
	memTotal, _ := readSysfsFloat(filepath.Join(devicePath, "mem_info_vram_total"))

	var temp, power float64
	hwmons, _ := filepath.Glob(filepath.Join(devicePath, "hwmon/hwmon*"))
	for _, hwmonDir := range hwmons {
		if t, err := readSysfsFloat(filepath.Join(hwmonDir, "temp1_input")); err == nil {
			temp = t / 1000.0
		}
		if p, err := readSysfsFloat(filepath.Join(hwmonDir, "power1_average")); err == nil {
			power += p / 1000000.0
		} else if p, err := readSysfsFloat(filepath.Join(hwmonDir, "power1_input")); err == nil {
			power += p / 1000000.0
		}
	}

	// Check if we got any meaningful data
	if usageErr != nil && memUsedErr != nil && temp == 0 {
		return false
	}

	// Single lock to update all values atomically
	gm.Lock()
	defer gm.Unlock()

	gpu, ok := gm.GpuDataMap[id]
	if !ok {
		gpu = &system.GPUData{Name: getAmdGpuName(devicePath)}
		gm.GpuDataMap[id] = gpu
	}

	if usageErr == nil {
		gpu.Usage += usage
	}
	gpu.MemoryUsed = bytesToMegabytes(memUsed)
	gpu.MemoryTotal = bytesToMegabytes(memTotal)
	gpu.Temperature = temp
	gpu.Power += power
	gpu.Count++
	return true
}

func readSysfsFloat(path string) (float64, error) {
	val, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(val)), 64)
}

// getAmdGpuName attempts to get a descriptive GPU name.
// First tries product_name (rarely available), then looks up the PCI device ID.
// Falls back to showing the raw device ID if not found in the lookup table.
func getAmdGpuName(devicePath string) string {
	// Try product_name first (works for some enterprise GPUs)
	if prod, err := os.ReadFile(filepath.Join(devicePath, "product_name")); err == nil {
		return strings.TrimSpace(string(prod))
	}

	// Read PCI device ID and look it up
	if deviceID, err := os.ReadFile(filepath.Join(devicePath, "device")); err == nil {
		id := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(string(deviceID))), "0x")
		if name, ok := getRadeonNames()[id]; ok {
			return fmt.Sprintf("Radeon %s", name)
		}
		return fmt.Sprintf("AMD GPU (%s)", id)
	}

	return "AMD GPU"
}

// getRadeonNames returns the AMD GPU name lookup table
// Device IDs from https://pci-ids.ucw.cz/read/PC/1002
var getRadeonNames = sync.OnceValue(func() map[string]string {
	return map[string]string{
		"7550": "RX 9070",
		"7590": "RX 9060 XT",
		"7551": "AI PRO R9700",

		"744c": "RX 7900",

		"1681": "680M",

		"7448": "PRO W7900",
		"745e": "PRO W7800",
		"7470": "PRO W7700",
		"73e3": "PRO W6600",
		"7422": "PRO W6400",
		"7341": "PRO W5500",
	}
})
