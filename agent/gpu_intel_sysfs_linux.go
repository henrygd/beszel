//go:build linux

package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

var (
	drmSysfsRoot  = "/sys/class/drm"
	intelSysfsNow = time.Now
)

type intelSysfsEnergySnapshot struct {
	microjoules uint64
	timestamp   time.Time
}

type intelSysfsCard struct {
	cardPath string
	hwmonDir string
}

// hasIntelSysfs returns true if any Intel DRM card exposes an hwmon energy counter.
func (gm *GPUManager) hasIntelSysfs() bool {
	cards, err := discoverIntelSysfsCards()
	return err == nil && len(cards) > 0
}

// startIntelSysfsCollector starts Intel GPU collection via sysfs.
func (gm *GPUManager) startIntelSysfsCollector() bool {
	go func() {
		if err := gm.collectIntelSysfsStats(); err != nil {
			slog.Warn("Error collecting Intel GPU data via sysfs", "err", err)
		}
	}()
	return true
}

// collectIntelSysfsStats collects Intel GPU metrics directly from DRM sysfs / hwmon.
func (gm *GPUManager) collectIntelSysfsStats() error {
	sysfsPollInterval := 3000 * time.Millisecond
	cards, err := discoverIntelSysfsCards()
	if err != nil {
		return err
	}
	if len(cards) == 0 {
		return errNoValidData
	}

	slog.Debug("Using sysfs for Intel GPU data collection", "cards", len(cards))
	for _, card := range cards {
		slog.Debug("Intel sysfs card detected", "card", filepath.Base(card.cardPath), "hwmon", card.hwmonDir)
	}

	failures := 0
	for {
		hasData := false
		for _, card := range cards {
			if gm.updateIntelSysfsGpuData(card.cardPath, card.hwmonDir) {
				hasData = true
			}
		}
		if !hasData {
			failures++
			if failures > maxFailureRetries {
				return errNoValidData
			}
			slog.Warn("No Intel GPU data from sysfs", "failures", failures)
			time.Sleep(retryWaitTime)
			continue
		}
		failures = 0
		time.Sleep(sysfsPollInterval)
	}
}

func discoverIntelSysfsCards() ([]intelSysfsCard, error) {
	paths, err := filepath.Glob(filepath.Join(drmSysfsRoot, "card*"))
	if err != nil {
		return nil, err
	}

	var cards []intelSysfsCard
	for _, cardPath := range paths {
		if strings.Contains(filepath.Base(cardPath), "-") || !isIntelGpu(cardPath) {
			continue
		}
		hwmonDir := findIntelEnergyHwmon(filepath.Join(cardPath, "device"))
		if hwmonDir == "" {
			continue
		}
		cards = append(cards, intelSysfsCard{cardPath: cardPath, hwmonDir: hwmonDir})
	}
	return cards, nil
}

func isIntelGpu(cardPath string) bool {
	vendor, err := utils.ReadStringFileLimited(filepath.Join(cardPath, "device/vendor"), 64)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(vendor), "0x8086")
}

func findIntelEnergyHwmon(devicePath string) string {
	hwmons, _ := filepath.Glob(filepath.Join(devicePath, "hwmon/hwmon*"))
	var fallback string
	for _, hwmonDir := range hwmons {
		if !sysfsFileExists(filepath.Join(hwmonDir, "energy1_input")) {
			continue
		}
		if name, err := utils.ReadStringFileLimited(filepath.Join(hwmonDir, "name"), 64); err == nil && strings.EqualFold(strings.TrimSpace(name), "xe") {
			return hwmonDir
		}
		if fallback == "" {
			fallback = hwmonDir
		}
	}
	return fallback
}

func sysfsFileExists(path string) bool {
	_, err := utils.ReadStringFileLimited(path, 1)
	return err == nil
}

// updateIntelSysfsGpuData reads GPU metrics from sysfs and updates the GPU data map.
// Returns true if the required energy counter was read successfully.
func (gm *GPUManager) updateIntelSysfsGpuData(cardPath, hwmonDir string) bool {
	devicePath := filepath.Join(cardPath, "device")
	id := filepath.Base(cardPath)

	energy, err := readSysfsUint(filepath.Join(hwmonDir, "energy1_input"))
	if err != nil {
		return false
	}

	now := intelSysfsNow()
	power, hasPower := gm.calculateIntelSysfsPower(id, energy, now)
	powerPkg, hasPowerPkg := gm.readIntelSysfsPowerPkg(id, hwmonDir, now)
	temp := readIntelSysfsTemperature(hwmonDir)
	usage, usageErr := readOptionalSysfsFloat(filepath.Join(devicePath, "gpu_busy_percent"))
	memUsed, memUsedErr := readFirstOptionalSysfsFloat(
		filepath.Join(devicePath, "mem_info_vram_used"),
		filepath.Join(devicePath, "mem_info_lmem_used"),
		filepath.Join(devicePath, "mem_info_local_mem_used"),
	)
	memTotal, memTotalErr := readFirstOptionalSysfsFloat(
		filepath.Join(devicePath, "mem_info_vram_total"),
		filepath.Join(devicePath, "mem_info_lmem_total"),
		filepath.Join(devicePath, "mem_info_local_mem_total"),
	)

	gm.Lock()
	defer gm.Unlock()

	gpu, ok := gm.GpuDataMap[id]
	if !ok {
		gpu = &system.GPUData{Name: getIntelSysfsGpuName(cardPath)}
		gm.GpuDataMap[id] = gpu
	}

	if usageErr == nil {
		gpu.Usage += usage
	}
	if memUsedErr == nil {
		gpu.MemoryUsed = utils.BytesToMegabytes(memUsed)
	}
	if memTotalErr == nil {
		gpu.MemoryTotal = utils.BytesToMegabytes(memTotal)
	}
	if temp > 0 {
		gpu.Temperature = temp
	}
	if hasPower {
		gpu.Power += power
		slog.Debug("Computed Intel sysfs GPU power", "card", id, "watts", power)
	}
	if hasPowerPkg {
		gpu.PowerPkg += powerPkg
	}
	gpu.Count++
	return true
}

func (gm *GPUManager) calculateIntelSysfsPower(cardID string, microjoules uint64, timestamp time.Time) (float64, bool) {
	if gm.intelSysfsEnergySnapshots == nil {
		gm.intelSysfsEnergySnapshots = make(map[string]intelSysfsEnergySnapshot)
	}

	last, ok := gm.intelSysfsEnergySnapshots[cardID]
	gm.intelSysfsEnergySnapshots[cardID] = intelSysfsEnergySnapshot{microjoules: microjoules, timestamp: timestamp}
	if !ok {
		return 0, false
	}
	if microjoules < last.microjoules {
		slog.Debug("Intel sysfs energy counter reset", "card", cardID)
		return 0, false
	}
	elapsed := timestamp.Sub(last.timestamp).Seconds()
	if elapsed <= 0 {
		return 0, false
	}
	delta := microjoules - last.microjoules
	return float64(delta) / 1_000_000.0 / elapsed, true
}

func (gm *GPUManager) readIntelSysfsPowerPkg(cardID, hwmonDir string, timestamp time.Time) (float64, bool) {
	energyPaths, _ := filepath.Glob(filepath.Join(hwmonDir, "energy*_input"))
	for _, path := range energyPaths {
		if filepath.Base(path) == "energy1_input" {
			continue
		}
		energy, err := readSysfsUint(path)
		if err != nil {
			continue
		}
		return gm.calculateIntelSysfsPower(cardID+":"+filepath.Base(path), energy, timestamp)
	}
	return 0, false
}

func readIntelSysfsTemperature(hwmonDir string) float64 {
	tempPaths, _ := filepath.Glob(filepath.Join(hwmonDir, "temp*_input"))
	for _, path := range tempPaths {
		temp, err := readSysfsFloat(path)
		if err == nil && temp > 0 {
			return temp / 1000.0
		}
	}
	return 0
}

func readSysfsUint(path string) (uint64, error) {
	val, err := utils.ReadStringFileLimited(path, 64)
	if err != nil {
		slog.Debug("Failed to read sysfs value", "path", path, "error", err)
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(val), 10, 64)
}

func readOptionalSysfsFloat(path string) (float64, error) {
	val, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(val)), 64)
}

func readFirstOptionalSysfsFloat(paths ...string) (float64, error) {
	for _, path := range paths {
		val, err := readOptionalSysfsFloat(path)
		if err == nil {
			return val, nil
		}
	}
	return 0, fmt.Errorf("no sysfs values found")
}

func getIntelSysfsGpuName(cardPath string) string {
	devicePath := filepath.Join(cardPath, "device")
	if product, err := utils.ReadStringFileLimited(filepath.Join(devicePath, "product_name"), 128); err == nil && strings.TrimSpace(product) != "" {
		return strings.TrimSpace(product)
	}
	if name, err := utils.ReadStringFileLimited(filepath.Join(devicePath, "name"), 128); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fmt.Sprintf("Intel GPU %s", filepath.Base(cardPath))
}
