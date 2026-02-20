//go:build linux

package agent

import (
	"bufio"
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

var amdgpuNameCache = struct {
	sync.RWMutex
	hits   map[string]string
	misses map[string]struct{}
}{
	hits:   make(map[string]string),
	misses: make(map[string]struct{}),
}

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
	sysfsPollInterval := 3000 * time.Millisecond
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
		time.Sleep(sysfsPollInterval)
	}
}

// isAmdGpu checks whether a DRM card path belongs to AMD vendor ID 0x1002.
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
	// if gtt is present, add it to the memory used and total (https://github.com/henrygd/beszel/issues/1569#issuecomment-3837640484)
	if gttUsed, err := readSysfsFloat(filepath.Join(devicePath, "mem_info_gtt_used")); err == nil && gttUsed > 0 {
		if gttTotal, err := readSysfsFloat(filepath.Join(devicePath, "mem_info_gtt_total")); err == nil {
			memUsed += gttUsed
			memTotal += gttTotal
		}
	}

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

// readSysfsFloat reads and parses a numeric value from a sysfs file.
func readSysfsFloat(path string) (float64, error) {
	val, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(val)), 64)
}

// normalizeHexID normalizes hex IDs by trimming spaces, lowercasing, and dropping 0x.
func normalizeHexID(id string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "0x")
}

// cacheKeyForAmdgpu builds the cache key for a device and optional revision.
func cacheKeyForAmdgpu(deviceID, revisionID string) string {
	if revisionID != "" {
		return deviceID + ":" + revisionID
	}
	return deviceID
}

// lookupAmdgpuNameInFile resolves an AMDGPU name from amdgpu.ids by device/revision.
func lookupAmdgpuNameInFile(deviceID, revisionID, filePath string) (name string, exact bool, found bool) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", false, false
	}
	defer file.Close()

	var byDevice string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 3 {
			continue
		}

		dev := normalizeHexID(parts[0])
		rev := normalizeHexID(parts[1])
		productName := strings.TrimSpace(parts[2])
		if dev == "" || productName == "" || dev != deviceID {
			continue
		}
		if byDevice == "" {
			byDevice = productName
		}
		if revisionID != "" && rev == revisionID {
			return productName, true, true
		}
	}
	if byDevice != "" {
		return byDevice, false, true
	}
	return "", false, false
}

// getCachedAmdgpuName returns cached hit/miss status for the given device/revision.
func getCachedAmdgpuName(deviceID, revisionID string) (name string, found bool, done bool) {
	// Build the list of cache keys to check. We always look up the exact device+revision key.
	// When revisionID is set, we also look up deviceID alone, since the cache may store a
	// device-only fallback when we couldn't resolve the exact revision.
	keys := []string{cacheKeyForAmdgpu(deviceID, revisionID)}
	if revisionID != "" {
		keys = append(keys, deviceID)
	}

	knownMisses := 0
	amdgpuNameCache.RLock()
	defer amdgpuNameCache.RUnlock()
	for _, key := range keys {
		if name, ok := amdgpuNameCache.hits[key]; ok {
			return name, true, true
		}
		if _, ok := amdgpuNameCache.misses[key]; ok {
			knownMisses++
		}
	}
	// done=true means "don't bother doing slow lookup": we either found a name (above) or
	// every key we checked was already a known miss, so we've tried before and failed.
	return "", false, knownMisses == len(keys)
}

// normalizeAmdgpuName trims standard suffixes from AMDGPU product names.
func normalizeAmdgpuName(name string) string {
	for _, suffix := range []string{" Graphics", " Series"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// cacheAmdgpuName stores a resolved AMDGPU name in the lookup cache.
func cacheAmdgpuName(deviceID, revisionID, name string, exact bool) {
	name = normalizeAmdgpuName(name)
	amdgpuNameCache.Lock()
	defer amdgpuNameCache.Unlock()
	if exact && revisionID != "" {
		amdgpuNameCache.hits[cacheKeyForAmdgpu(deviceID, revisionID)] = name
	}
	amdgpuNameCache.hits[deviceID] = name
}

// cacheMissingAmdgpuName records unresolved device/revision lookups.
func cacheMissingAmdgpuName(deviceID, revisionID string) {
	amdgpuNameCache.Lock()
	defer amdgpuNameCache.Unlock()
	amdgpuNameCache.misses[deviceID] = struct{}{}
	if revisionID != "" {
		amdgpuNameCache.misses[cacheKeyForAmdgpu(deviceID, revisionID)] = struct{}{}
	}
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
		id := normalizeHexID(string(deviceID))
		revision := ""
		if revBytes, revErr := os.ReadFile(filepath.Join(devicePath, "revision")); revErr == nil {
			revision = normalizeHexID(string(revBytes))
		}

		if name, found, done := getCachedAmdgpuName(id, revision); found {
			return name
		} else if !done {
			if name, exact, ok := lookupAmdgpuNameInFile(id, revision, "/usr/share/libdrm/amdgpu.ids"); ok {
				cacheAmdgpuName(id, revision, name, exact)
				return normalizeAmdgpuName(name)
			}
			cacheMissingAmdgpuName(id, revision)
		}

		return fmt.Sprintf("AMD GPU (%s)", id)
	}

	return "AMD GPU"
}
