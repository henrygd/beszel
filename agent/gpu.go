package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"

	"golang.org/x/exp/slog"
)

const (
	// Commands
	nvidiaSmiCmd  string = "nvidia-smi"
	rocmSmiCmd    string = "rocm-smi"
	tegraStatsCmd string = "tegrastats"

	// Polling intervals
	nvidiaSmiInterval  string        = "4"    // in seconds
	tegraStatsInterval string        = "3700" // in milliseconds
	rocmSmiInterval    time.Duration = 4300 * time.Millisecond
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
	nvidiaSmi     bool
	rocmSmi       bool
	tegrastats    bool
	intelGpuStats bool
	GpuDataMap    map[string]*system.GPUData
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
	tempPattern := regexp.MustCompile(`tj@(\d+\.?\d*)C`)
	// Orin Nano / NX do not have GPU specific power monitor
	// TODO: Maybe use VDD_IN for Nano / NX and add a total system power chart
	powerPattern := regexp.MustCompile(`(GPU_SOC|CPU_GPU_CV) (\d+)mW`)

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
			power, _ := strconv.ParseFloat(string(powerMatches[2]), 64)
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

		if _, ok := gm.GpuDataMap[v.ID]; !ok {
			gm.GpuDataMap[v.ID] = &system.GPUData{Name: v.Name}
		}
		gpu := gm.GpuDataMap[v.ID]
		gpu.Temperature, _ = strconv.ParseFloat(v.Temperature, 64)
		gpu.MemoryUsed = bytesToMegabytes(memoryUsage)
		gpu.MemoryTotal = bytesToMegabytes(totalMemory)
		gpu.Usage += usage
		gpu.Power += power
		gpu.Count++
	}
	return true
}

// sums and resets the current GPU utilization data since the last update
func (gm *GPUManager) GetCurrentData() map[string]system.GPUData {
	gm.Lock()
	defer gm.Unlock()

	// check for GPUs with the same name
	nameCounts := make(map[string]int)
	for _, gpu := range gm.GpuDataMap {
		nameCounts[gpu.Name]++
	}

	// copy / reset the data
	gpuData := make(map[string]system.GPUData, len(gm.GpuDataMap))
	for id, gpu := range gm.GpuDataMap {
		// avoid division by zero
		count := max(gpu.Count, 1)

		// average the data
		gpuAvg := *gpu
		gpuAvg.Temperature = twoDecimals(gpu.Temperature)
		gpuAvg.Power = twoDecimals(gpu.Power / count)

		// intel gpu stats doesn't provide usage, memory used, or memory total
		if gpu.Engines != nil {
			maxEngineUsage := 0.0
			for name, engine := range gpu.Engines {
				gpuAvg.Engines[name] = twoDecimals(engine / count)
				maxEngineUsage = max(maxEngineUsage, engine/count)
			}
			gpuAvg.Usage = twoDecimals(maxEngineUsage)
		} else {
			gpuAvg.Usage = twoDecimals(gpu.Usage / count)
			gpuAvg.MemoryUsed = twoDecimals(gpu.MemoryUsed)
			gpuAvg.MemoryTotal = twoDecimals(gpu.MemoryTotal)
		}

		// reset accumulators in the original gpu data for next collection
		gpu.Usage, gpu.Power, gpu.Count = gpuAvg.Usage, gpuAvg.Power, 1
		gpu.Engines = gpuAvg.Engines

		// append id to the name if there are multiple GPUs with the same name
		if nameCounts[gpu.Name] > 1 {
			gpuAvg.Name = fmt.Sprintf("%s %s", gpu.Name, id)
		}
		gpuData[id] = gpuAvg
	}
	slog.Debug("GPU", "data", gpuData)
	return gpuData
}

// detectGPUs checks for the presence of GPU management tools (nvidia-smi, rocm-smi, tegrastats)
// in the system path. It sets the corresponding flags in the GPUManager struct if any of these
// tools are found. If none of the tools are found, it returns an error indicating that no GPU
// management tools are available.
func (gm *GPUManager) detectGPUs() error {
	if _, err := exec.LookPath(nvidiaSmiCmd); err == nil {
		gm.nvidiaSmi = true
	}
	if _, err := exec.LookPath(rocmSmiCmd); err == nil {
		gm.rocmSmi = true
	}
	if _, err := exec.LookPath(tegraStatsCmd); err == nil {
		gm.tegrastats = true
		gm.nvidiaSmi = false
	}
	if _, err := exec.LookPath(intelGpuStatsCmd); err == nil {
		gm.intelGpuStats = true
	}
	if gm.nvidiaSmi || gm.rocmSmi || gm.tegrastats || gm.intelGpuStats {
		return nil
	}
	return fmt.Errorf("no GPU found - install nvidia-smi, rocm-smi, tegrastats, or intel_gpu_top")
}

// startCollector starts the appropriate GPU data collector based on the command
func (gm *GPUManager) startCollector(command string) {
	collector := gpuCollector{
		name:    command,
		bufSize: 10 * 1024,
	}
	switch command {
	case intelGpuStatsCmd:
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
	case nvidiaSmiCmd:
		collector.cmdArgs = []string{
			"-l", nvidiaSmiInterval,
			"--query-gpu=index,name,temperature.gpu,memory.used,memory.total,utilization.gpu,power.draw",
			"--format=csv,noheader,nounits",
		}
		collector.parse = gm.parseNvidiaData
		go collector.start()
	case tegraStatsCmd:
		collector.cmdArgs = []string{"--interval", tegraStatsInterval}
		collector.parse = gm.getJetsonParser()
		go collector.start()
	case rocmSmiCmd:
		collector.cmdArgs = []string{"--showid", "--showtemp", "--showuse", "--showpower", "--showproductname", "--showmeminfo", "vram", "--json"}
		collector.parse = gm.parseAmdData
		go func() {
			failures := 0
			for {
				if err := collector.collect(); err != nil {
					failures++
					if failures > maxFailureRetries {
						break
					}
					slog.Warn("Error collecting AMD GPU data", "err", err)
				}
				time.Sleep(rocmSmiInterval)
			}
		}()
	}
}

// NewGPUManager creates and initializes a new GPUManager
func NewGPUManager() (*GPUManager, error) {
	if skipGPU, _ := GetEnv("SKIP_GPU"); skipGPU == "true" {
		return nil, nil
	}
	var gm GPUManager
	if err := gm.detectGPUs(); err != nil {
		return nil, err
	}
	gm.GpuDataMap = make(map[string]*system.GPUData)

	if gm.nvidiaSmi {
		gm.startCollector(nvidiaSmiCmd)
	}
	if gm.rocmSmi {
		gm.startCollector(rocmSmiCmd)
	}
	if gm.tegrastats {
		gm.startCollector(tegraStatsCmd)
	}
	if gm.intelGpuStats {
		gm.startCollector(intelGpuStatsCmd)
	}

	return &gm, nil
}
