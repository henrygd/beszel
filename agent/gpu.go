package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	// Commands
	nvidiaSmiCmd    string = "nvidia-smi"
	rocmSmiCmd      string = "rocm-smi"
	tegraStatsCmd   string = "tegrastats"
	nvtopCmd        string = "nvtop"
	powermetricsCmd string = "powermetrics"
	macmonCmd       string = "macmon"
	noGPUFoundMsg   string = "no GPU found - see https://beszel.dev/guide/gpu"

	// Command retry and timeout constants
	retryWaitTime     time.Duration = 5 * time.Second
	maxFailureRetries int           = 5

	// Unit Conversions
	mebibytesInAMegabyte float64 = 1.024  // nvidia-smi reports memory in MiB
	milliwattsInAWatt    float64 = 1000.0 // tegrastats reports power in mW
)

// GPUManager manages data collection for GPUs (either Nvidia or AMD)
type GPUManager struct {
	sync.Mutex
	GpuDataMap map[string]*system.GPUData
	// lastAvgData stores the last calculated averages for each GPU
	// Used when a collection happens before new data arrives (Count == 0)
	lastAvgData map[string]system.GPUData
	// Per-cache-key tracking for delta calculations
	// cacheKey -> gpuId -> snapshot of last count/usage/power values
	lastSnapshots map[uint16]map[string]*gpuSnapshot
}

// gpuSnapshot stores the last observed incremental values for delta tracking
type gpuSnapshot struct {
	count    uint32
	usage    float64
	power    float64
	powerPkg float64
	engines  map[string]float64
}

// RocmSmiJson represents the JSON structure of rocm-smi output
type RocmSmiJson struct {
	ID           string `json:"GUID"`
	Name         string `json:"Card series"`
	Temperature  string `json:"Temperature (Sensor edge) (C)"`
	MemoryUsed   string `json:"VRAM Total Used Memory (B)"`
	MemoryTotal  string `json:"VRAM Total Memory (B)"`
	Usage        string `json:"GPU use (%)"`
	PowerPackage string `json:"Average Graphics Package Power (W)"`
	PowerSocket  string `json:"Current Socket Graphics Package Power (W)"`
}

// gpuCollector defines a collector for a specific GPU management utility (nvidia-smi or rocm-smi)
type gpuCollector struct {
	name    string
	cmdArgs []string
	parse   func([]byte) bool // returns true if valid data was found
	buf     []byte
	bufSize uint16
}

var errNoValidData = fmt.Errorf("no valid GPU data found") // Error for missing data

// collectorSource identifies a selectable GPU collector in GPU_COLLECTOR.
type collectorSource string

const (
	collectorSourceNVTop        collectorSource = collectorSource(nvtopCmd)
	collectorSourceNVML         collectorSource = "nvml"
	collectorSourceNvidiaSMI    collectorSource = collectorSource(nvidiaSmiCmd)
	collectorSourceIntelGpuTop  collectorSource = collectorSource(intelGpuStatsCmd)
	collectorSourceAmdSysfs     collectorSource = "amd_sysfs"
	collectorSourceRocmSMI      collectorSource = collectorSource(rocmSmiCmd)
	collectorSourceMacmon       collectorSource = collectorSource(macmonCmd)
	collectorSourcePowermetrics collectorSource = collectorSource(powermetricsCmd)
	collectorGroupNvidia        string          = "nvidia"
	collectorGroupIntel         string          = "intel"
	collectorGroupAmd           string          = "amd"
	collectorGroupApple         string          = "apple"
)

func isValidCollectorSource(source collectorSource) bool {
	switch source {
	case collectorSourceNVTop,
		collectorSourceNVML,
		collectorSourceNvidiaSMI,
		collectorSourceIntelGpuTop,
		collectorSourceAmdSysfs,
		collectorSourceRocmSMI,
		collectorSourceMacmon,
		collectorSourcePowermetrics:
		return true
	}
	return false
}

// gpuCapabilities describes detected GPU tooling and sysfs support on the host.
type gpuCapabilities struct {
	hasNvidiaSmi    bool
	hasRocmSmi      bool
	hasAmdSysfs     bool
	hasTegrastats   bool
	hasIntelGpuTop  bool
	hasNvtop        bool
	hasMacmon       bool
	hasPowermetrics bool
}

type collectorDefinition struct {
	group              string
	available          bool
	start              func(onFailure func()) bool
	deprecationWarning string
}

// starts and manages the ongoing collection of GPU data for the specified GPU management utility
func (c *gpuCollector) start() {
	for {
		err := c.collect()
		if err != nil {
			if err == errNoValidData {
				slog.Warn(c.name + " found no valid GPU data, stopping")
				break
			}
			slog.Warn(c.name+" failed, restarting", "err", err)
			time.Sleep(retryWaitTime)
			continue
		}
	}
}

// collect executes the command, parses output with the assigned parser function
func (c *gpuCollector) collect() error {
	cmd := exec.Command(c.name, c.cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	if c.buf == nil {
		c.buf = make([]byte, 0, c.bufSize)
	}
	scanner.Buffer(c.buf, bufio.MaxScanTokenSize)

	for scanner.Scan() {
		hasValidData := c.parse(scanner.Bytes())
		if !hasValidData {
			return errNoValidData
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return cmd.Wait()
}

// getJetsonParser returns a function to parse the output of tegrastats and update the GPUData map
func (gm *GPUManager) getJetsonParser() func(output []byte) bool {
	// use closure to avoid recompiling the regex
	ramPattern := regexp.MustCompile(`RAM (\d+)/(\d+)MB`)
	gr3dPattern := regexp.MustCompile(`GR3D_FREQ (\d+)%`)
	tempPattern := regexp.MustCompile(`(?:tj|GPU)@(\d+\.?\d*)C`)
	// Orin Nano / NX do not have GPU specific power monitor
	// TODO: Maybe use VDD_IN for Nano / NX and add a total system power chart
	powerPattern := regexp.MustCompile(`(GPU_SOC|CPU_GPU_CV)\s+(\d+)mW|VDD_SYS_GPU\s+(\d+)/\d+`)

	// jetson devices have only one gpu so we'll just initialize here
	gpuData := &system.GPUData{Name: "GPU"}
	gm.GpuDataMap["0"] = gpuData

	return func(output []byte) bool {
		gm.Lock()
		defer gm.Unlock()
		// Parse RAM usage
		ramMatches := ramPattern.FindSubmatch(output)
		if ramMatches != nil {
			gpuData.MemoryUsed, _ = strconv.ParseFloat(string(ramMatches[1]), 64)
			gpuData.MemoryTotal, _ = strconv.ParseFloat(string(ramMatches[2]), 64)
		}
		// Parse GR3D (GPU) usage
		gr3dMatches := gr3dPattern.FindSubmatch(output)
		if gr3dMatches != nil {
			gr3dUsage, _ := strconv.ParseFloat(string(gr3dMatches[1]), 64)
			gpuData.Usage += gr3dUsage
		}
		// Parse temperature
		tempMatches := tempPattern.FindSubmatch(output)
		if tempMatches != nil {
			gpuData.Temperature, _ = strconv.ParseFloat(string(tempMatches[1]), 64)
		}
		// Parse power usage
		powerMatches := powerPattern.FindSubmatch(output)
		if powerMatches != nil {
			// powerMatches[2] is the "(GPU_SOC|CPU_GPU_CV) <N>mW" capture
			// powerMatches[3] is the "VDD_SYS_GPU <N>/<N>" capture
			powerStr := string(powerMatches[2])
			if powerStr == "" {
				powerStr = string(powerMatches[3])
			}
			power, _ := strconv.ParseFloat(powerStr, 64)
			gpuData.Power += power / milliwattsInAWatt
		}
		gpuData.Count++
		return true
	}
}

// parseNvidiaData parses the output of nvidia-smi and updates the GPUData map
func (gm *GPUManager) parseNvidiaData(output []byte) bool {
	gm.Lock()
	defer gm.Unlock()
	scanner := bufio.NewScanner(bytes.NewReader(output))
	var valid bool
	for scanner.Scan() {
		line := scanner.Text() // Or use scanner.Bytes() for []byte
		fields := strings.Split(strings.TrimSpace(line), ", ")
		if len(fields) < 7 {
			continue
		}
		valid = true
		id := fields[0]
		temp, _ := strconv.ParseFloat(fields[2], 64)
		memoryUsage, _ := strconv.ParseFloat(fields[3], 64)
		totalMemory, _ := strconv.ParseFloat(fields[4], 64)
		usage, _ := strconv.ParseFloat(fields[5], 64)
		power, _ := strconv.ParseFloat(fields[6], 64)
		// add gpu if not exists
		if _, ok := gm.GpuDataMap[id]; !ok {
			name := strings.TrimPrefix(fields[1], "NVIDIA ")
			gm.GpuDataMap[id] = &system.GPUData{Name: strings.TrimSuffix(name, " Laptop GPU")}
		}
		// update gpu data
		gpu := gm.GpuDataMap[id]
		gpu.Temperature = temp
		gpu.MemoryUsed = memoryUsage / mebibytesInAMegabyte
		gpu.MemoryTotal = totalMemory / mebibytesInAMegabyte
		gpu.Usage += usage
		gpu.Power += power
		gpu.Count++
	}
	return valid
}

// parseAmdData parses the output of rocm-smi and updates the GPUData map
func (gm *GPUManager) parseAmdData(output []byte) bool {
	var rocmSmiInfo map[string]RocmSmiJson
	if err := json.Unmarshal(output, &rocmSmiInfo); err != nil || len(rocmSmiInfo) == 0 {
		return false
	}
	gm.Lock()
	defer gm.Unlock()
	for _, v := range rocmSmiInfo {
		var power float64
		if v.PowerPackage != "" {
			power, _ = strconv.ParseFloat(v.PowerPackage, 64)
		} else {
			power, _ = strconv.ParseFloat(v.PowerSocket, 64)
		}
		memoryUsage, _ := strconv.ParseFloat(v.MemoryUsed, 64)
		totalMemory, _ := strconv.ParseFloat(v.MemoryTotal, 64)
		usage, _ := strconv.ParseFloat(v.Usage, 64)

		id := v.ID
		if _, ok := gm.GpuDataMap[id]; !ok {
			gm.GpuDataMap[id] = &system.GPUData{Name: v.Name}
		}
		gpu := gm.GpuDataMap[id]
		gpu.Temperature, _ = strconv.ParseFloat(v.Temperature, 64)
		gpu.MemoryUsed = bytesToMegabytes(memoryUsage)
		gpu.MemoryTotal = bytesToMegabytes(totalMemory)
		gpu.Usage += usage
		gpu.Power += power
		gpu.Count++
	}
	return true
}

// GetCurrentData returns GPU utilization data averaged since the last call with this cacheKey
func (gm *GPUManager) GetCurrentData(cacheKey uint16) map[string]system.GPUData {
	gm.Lock()
	defer gm.Unlock()

	gm.initializeSnapshots(cacheKey)
	nameCounts := gm.countGPUNames()

	gpuData := make(map[string]system.GPUData, len(gm.GpuDataMap))
	for id, gpu := range gm.GpuDataMap {
		gpuAvg := gm.calculateGPUAverage(id, gpu, cacheKey)
		gm.updateInstantaneousValues(&gpuAvg, gpu)
		gm.storeSnapshot(id, gpu, cacheKey)

		// Append id to name if there are multiple GPUs with the same name
		if nameCounts[gpu.Name] > 1 {
			gpuAvg.Name = fmt.Sprintf("%s %s", gpu.Name, id)
		}
		gpuData[id] = gpuAvg
	}
	slog.Debug("GPU", "data", gpuData)
	return gpuData
}

// initializeSnapshots ensures snapshot maps are initialized for the given cache key
func (gm *GPUManager) initializeSnapshots(cacheKey uint16) {
	if gm.lastAvgData == nil {
		gm.lastAvgData = make(map[string]system.GPUData)
	}
	if gm.lastSnapshots == nil {
		gm.lastSnapshots = make(map[uint16]map[string]*gpuSnapshot)
	}
	if gm.lastSnapshots[cacheKey] == nil {
		gm.lastSnapshots[cacheKey] = make(map[string]*gpuSnapshot)
	}
}

// countGPUNames returns a map of GPU names to their occurrence count
func (gm *GPUManager) countGPUNames() map[string]int {
	nameCounts := make(map[string]int)
	for _, gpu := range gm.GpuDataMap {
		nameCounts[gpu.Name]++
	}
	return nameCounts
}

// calculateGPUAverage computes the average GPU metrics since the last snapshot for this cache key
func (gm *GPUManager) calculateGPUAverage(id string, gpu *system.GPUData, cacheKey uint16) system.GPUData {
	lastSnapshot := gm.lastSnapshots[cacheKey][id]
	currentCount := uint32(gpu.Count)
	deltaCount := gm.calculateDeltaCount(currentCount, lastSnapshot)

	// If no new data arrived
	if deltaCount == 0 {
		// If GPU appears suspended (instantaneous values are 0), return zero values
		// Otherwise return last known average for temporary collection gaps
		if gpu.Temperature == 0 && gpu.MemoryUsed == 0 {
			return system.GPUData{Name: gpu.Name}
		}
		return gm.lastAvgData[id] // zero value if not found
	}

	// Calculate new average
	gpuAvg := *gpu
	deltaUsage, deltaPower, deltaPowerPkg := gm.calculateDeltas(gpu, lastSnapshot)

	gpuAvg.Power = twoDecimals(deltaPower / float64(deltaCount))

	if gpu.Engines != nil {
		// make fresh map for averaged engine metrics to avoid mutating
		// the accumulator map stored in gm.GpuDataMap
		gpuAvg.Engines = make(map[string]float64, len(gpu.Engines))
		gpuAvg.Usage = gm.calculateIntelGPUUsage(&gpuAvg, gpu, lastSnapshot, deltaCount)
		gpuAvg.PowerPkg = twoDecimals(deltaPowerPkg / float64(deltaCount))
	} else {
		gpuAvg.Usage = twoDecimals(deltaUsage / float64(deltaCount))
	}

	gm.lastAvgData[id] = gpuAvg
	return gpuAvg
}

// calculateDeltaCount returns the change in count since the last snapshot
func (gm *GPUManager) calculateDeltaCount(currentCount uint32, lastSnapshot *gpuSnapshot) uint32 {
	if lastSnapshot != nil {
		return currentCount - lastSnapshot.count
	}
	return currentCount
}

// calculateDeltas computes the change in usage, power, and powerPkg since the last snapshot
func (gm *GPUManager) calculateDeltas(gpu *system.GPUData, lastSnapshot *gpuSnapshot) (deltaUsage, deltaPower, deltaPowerPkg float64) {
	if lastSnapshot != nil {
		return gpu.Usage - lastSnapshot.usage,
			gpu.Power - lastSnapshot.power,
			gpu.PowerPkg - lastSnapshot.powerPkg
	}
	return gpu.Usage, gpu.Power, gpu.PowerPkg
}

// calculateIntelGPUUsage computes Intel GPU usage from engine metrics and returns max engine usage
func (gm *GPUManager) calculateIntelGPUUsage(gpuAvg, gpu *system.GPUData, lastSnapshot *gpuSnapshot, deltaCount uint32) float64 {
	maxEngineUsage := 0.0
	for name, engine := range gpu.Engines {
		var deltaEngine float64
		if lastSnapshot != nil && lastSnapshot.engines != nil {
			deltaEngine = engine - lastSnapshot.engines[name]
		} else {
			deltaEngine = engine
		}
		gpuAvg.Engines[name] = twoDecimals(deltaEngine / float64(deltaCount))
		maxEngineUsage = max(maxEngineUsage, deltaEngine/float64(deltaCount))
	}
	return twoDecimals(maxEngineUsage)
}

// updateInstantaneousValues updates values that should reflect current state, not averages
func (gm *GPUManager) updateInstantaneousValues(gpuAvg *system.GPUData, gpu *system.GPUData) {
	gpuAvg.Temperature = twoDecimals(gpu.Temperature)
	gpuAvg.MemoryUsed = twoDecimals(gpu.MemoryUsed)
	gpuAvg.MemoryTotal = twoDecimals(gpu.MemoryTotal)
}

// storeSnapshot saves the current GPU state for this cache key
func (gm *GPUManager) storeSnapshot(id string, gpu *system.GPUData, cacheKey uint16) {
	snapshot := &gpuSnapshot{
		count:    uint32(gpu.Count),
		usage:    gpu.Usage,
		power:    gpu.Power,
		powerPkg: gpu.PowerPkg,
	}
	if gpu.Engines != nil {
		snapshot.engines = make(map[string]float64, len(gpu.Engines))
		maps.Copy(snapshot.engines, gpu.Engines)
	}
	gm.lastSnapshots[cacheKey][id] = snapshot
}

// discoverGpuCapabilities checks for available GPU tooling and sysfs support.
// It only reports capability presence and does not apply policy decisions.
func (gm *GPUManager) discoverGpuCapabilities() gpuCapabilities {
	caps := gpuCapabilities{
		hasAmdSysfs: gm.hasAmdSysfs(),
	}
	if _, err := exec.LookPath(nvidiaSmiCmd); err == nil {
		caps.hasNvidiaSmi = true
	}
	if _, err := exec.LookPath(rocmSmiCmd); err == nil {
		caps.hasRocmSmi = true
	}
	if _, err := exec.LookPath(tegraStatsCmd); err == nil {
		caps.hasTegrastats = true
	}
	if _, err := exec.LookPath(intelGpuStatsCmd); err == nil {
		caps.hasIntelGpuTop = true
	}
	if _, err := exec.LookPath(nvtopCmd); err == nil {
		caps.hasNvtop = true
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath(macmonCmd); err == nil {
			caps.hasMacmon = true
		}
		if _, err := exec.LookPath(powermetricsCmd); err == nil {
			caps.hasPowermetrics = true
		}
	}
	return caps
}

func hasAnyGpuCollector(caps gpuCapabilities) bool {
	return caps.hasNvidiaSmi || caps.hasRocmSmi || caps.hasAmdSysfs || caps.hasTegrastats || caps.hasIntelGpuTop || caps.hasNvtop || caps.hasMacmon || caps.hasPowermetrics
}

func (gm *GPUManager) startIntelCollector() {
	go func() {
		failures := 0
		for {
			if err := gm.collectIntelStats(); err != nil {
				failures++
				if failures > maxFailureRetries {
					break
				}
				slog.Warn("Error collecting Intel GPU data; see https://beszel.dev/guide/gpu", "err", err)
				time.Sleep(retryWaitTime)
				continue
			}
		}
	}()
}

func (gm *GPUManager) startNvidiaSmiCollector(intervalSeconds string) {
	collector := gpuCollector{
		name:    nvidiaSmiCmd,
		bufSize: 10 * 1024,
		cmdArgs: []string{
			"-l", intervalSeconds,
			"--query-gpu=index,name,temperature.gpu,memory.used,memory.total,utilization.gpu,power.draw",
			"--format=csv,noheader,nounits",
		},
		parse: gm.parseNvidiaData,
	}
	go collector.start()
}

func (gm *GPUManager) startTegraStatsCollector(intervalMilliseconds string) {
	collector := gpuCollector{
		name:    tegraStatsCmd,
		bufSize: 10 * 1024,
		cmdArgs: []string{"--interval", intervalMilliseconds},
		parse:   gm.getJetsonParser(),
	}
	go collector.start()
}

func (gm *GPUManager) startRocmSmiCollector(pollInterval time.Duration) {
	collector := gpuCollector{
		name:    rocmSmiCmd,
		bufSize: 10 * 1024,
		cmdArgs: []string{"--showid", "--showtemp", "--showuse", "--showpower", "--showproductname", "--showmeminfo", "vram", "--json"},
		parse:   gm.parseAmdData,
	}
	go func() {
		failures := 0
		for {
			if err := collector.collect(); err != nil {
				failures++
				if failures > maxFailureRetries {
					break
				}
				slog.Warn("Error collecting AMD GPU data via rocm-smi", "err", err)
			}
			time.Sleep(pollInterval)
		}
	}()
}

func (gm *GPUManager) collectorDefinitions(caps gpuCapabilities) map[collectorSource]collectorDefinition {
	return map[collectorSource]collectorDefinition{
		collectorSourceNVML: {
			group:     collectorGroupNvidia,
			available: caps.hasNvidiaSmi,
			start: func(_ func()) bool {
				return gm.startNvmlCollector()
			},
		},
		collectorSourceNvidiaSMI: {
			group:     collectorGroupNvidia,
			available: caps.hasNvidiaSmi,
			start: func(_ func()) bool {
				gm.startNvidiaSmiCollector("4") // seconds
				return true
			},
		},
		collectorSourceIntelGpuTop: {
			group:     collectorGroupIntel,
			available: caps.hasIntelGpuTop,
			start: func(_ func()) bool {
				gm.startIntelCollector()
				return true
			},
		},
		collectorSourceAmdSysfs: {
			group:     collectorGroupAmd,
			available: caps.hasAmdSysfs,
			start: func(_ func()) bool {
				return gm.startAmdSysfsCollector()
			},
		},
		collectorSourceRocmSMI: {
			group:              collectorGroupAmd,
			available:          caps.hasRocmSmi,
			deprecationWarning: "rocm-smi is deprecated and may be removed in a future release",
			start: func(_ func()) bool {
				gm.startRocmSmiCollector(4300 * time.Millisecond)
				return true
			},
		},
		collectorSourceNVTop: {
			available: caps.hasNvtop,
			start: func(onFailure func()) bool {
				gm.startNvtopCollector("30", onFailure) // tens of milliseconds
				return true
			},
		},
		collectorSourceMacmon: {
			group:     collectorGroupApple,
			available: caps.hasMacmon,
			start: func(_ func()) bool {
				gm.startMacmonCollector()
				return true
			},
		},
		collectorSourcePowermetrics: {
			group:     collectorGroupApple,
			available: caps.hasPowermetrics,
			start: func(_ func()) bool {
				gm.startPowermetricsCollector()
				return true
			},
		},
	}
}

// parseCollectorPriority parses GPU_COLLECTOR and returns valid ordered entries.
func parseCollectorPriority(value string) []collectorSource {
	parts := strings.Split(value, ",")
	priorities := make([]collectorSource, 0, len(parts))
	for _, raw := range parts {
		name := collectorSource(strings.TrimSpace(strings.ToLower(raw)))
		if !isValidCollectorSource(name) {
			if name != "" {
				slog.Warn("Ignoring unknown GPU collector", "collector", name)
			}
			continue
		}
		priorities = append(priorities, name)
	}
	return priorities
}

// startNvmlCollector initializes NVML and starts its polling loop.
func (gm *GPUManager) startNvmlCollector() bool {
	collector := &nvmlCollector{gm: gm}
	if err := collector.init(); err != nil {
		slog.Warn("Failed to initialize NVML", "err", err)
		return false
	}
	go collector.start()
	return true
}

// startAmdSysfsCollector starts AMD GPU collection via sysfs.
func (gm *GPUManager) startAmdSysfsCollector() bool {
	go func() {
		if err := gm.collectAmdStats(); err != nil {
			slog.Warn("Error collecting AMD GPU data via sysfs", "err", err)
		}
	}()
	return true
}

// startCollectorsByPriority starts collectors in order with one source per vendor group.
func (gm *GPUManager) startCollectorsByPriority(priorities []collectorSource, caps gpuCapabilities) int {
	definitions := gm.collectorDefinitions(caps)
	selectedGroups := make(map[string]bool, 3)
	started := 0
	for i, source := range priorities {
		definition, ok := definitions[source]
		if !ok || !definition.available {
			continue
		}
		// nvtop is not a vendor-specific collector, so should only be used if no other collectors are selected or it is first in GPU_COLLECTOR.
		if source == collectorSourceNVTop {
			if len(selectedGroups) > 0 {
				slog.Warn("Skipping nvtop because other collectors are selected")
				continue
			}
			// if nvtop fails, fall back to remaining collectors.
			remaining := append([]collectorSource(nil), priorities[i+1:]...)
			if definition.start(func() {
				gm.startCollectorsByPriority(remaining, caps)
			}) {
				started++
				return started
			}
		}
		group := definition.group
		if group == "" || selectedGroups[group] {
			continue
		}
		if definition.deprecationWarning != "" {
			slog.Warn(definition.deprecationWarning)
		}
		if definition.start(nil) {
			selectedGroups[group] = true
			started++
		}
	}
	return started
}

// resolveLegacyCollectorPriority builds the default collector order when GPU_COLLECTOR is unset.
func (gm *GPUManager) resolveLegacyCollectorPriority(caps gpuCapabilities) []collectorSource {
	priorities := make([]collectorSource, 0, 4)

	if caps.hasNvidiaSmi && !caps.hasTegrastats {
		if nvml, _ := GetEnv("NVML"); nvml == "true" {
			priorities = append(priorities, collectorSourceNVML, collectorSourceNvidiaSMI)
		} else {
			priorities = append(priorities, collectorSourceNvidiaSMI)
		}
	}

	if caps.hasRocmSmi {
		if val, _ := GetEnv("AMD_SYSFS"); val == "true" {
			priorities = append(priorities, collectorSourceAmdSysfs)
		} else {
			priorities = append(priorities, collectorSourceRocmSMI)
		}
	} else if caps.hasAmdSysfs {
		priorities = append(priorities, collectorSourceAmdSysfs)
	}

	if caps.hasIntelGpuTop {
		priorities = append(priorities, collectorSourceIntelGpuTop)
	}

	// Apple collectors are currently opt-in only for testing.
	// Enable them with GPU_COLLECTOR=macmon or GPU_COLLECTOR=powermetrics.
	// TODO: uncomment below when Apple collectors are confirmed to be working.
	//
	// Prefer macmon on macOS (no sudo). Fall back to powermetrics if present.
	// if caps.hasMacmon {
	// 	priorities = append(priorities, collectorSourceMacmon)
	// } else if caps.hasPowermetrics {
	// 	priorities = append(priorities, collectorSourcePowermetrics)
	// }

	// Keep nvtop as a last resort only when no vendor collector exists.
	if len(priorities) == 0 && caps.hasNvtop {
		priorities = append(priorities, collectorSourceNVTop)
	}
	return priorities
}

// NewGPUManager creates and initializes a new GPUManager
func NewGPUManager() (*GPUManager, error) {
	if skipGPU, _ := GetEnv("SKIP_GPU"); skipGPU == "true" {
		return nil, nil
	}
	var gm GPUManager
	caps := gm.discoverGpuCapabilities()
	if !hasAnyGpuCollector(caps) {
		return nil, fmt.Errorf(noGPUFoundMsg)
	}
	gm.GpuDataMap = make(map[string]*system.GPUData)

	// Jetson devices should always use tegrastats (ignore GPU_COLLECTOR).
	if caps.hasTegrastats {
		gm.startTegraStatsCollector("3700")
		return &gm, nil
	}

	// if GPU_COLLECTOR is set, start user-defined collectors.
	if collectorConfig, ok := GetEnv("GPU_COLLECTOR"); ok && strings.TrimSpace(collectorConfig) != "" {
		priorities := parseCollectorPriority(collectorConfig)
		if gm.startCollectorsByPriority(priorities, caps) == 0 {
			return nil, fmt.Errorf("no configured GPU collectors are available")
		}
		return &gm, nil
	}

	// auto-detect and start collectors when GPU_COLLECTOR is unset.
	if gm.startCollectorsByPriority(gm.resolveLegacyCollectorPriority(caps), caps) == 0 {
		return nil, fmt.Errorf(noGPUFoundMsg)
	}

	return &gm, nil
}
