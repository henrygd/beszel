package agent

import (
	"bufio"
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

// GPUManager manages data collection for GPUs (either Nvidia or AMD)
type GPUManager struct {
	nvidiaSmi  bool
	rocmSmi    bool
	tegrastats bool
	GpuDataMap map[string]*system.GPUData
	mutex      sync.Mutex
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
	name  string
	cmd   *exec.Cmd
	parse func([]byte) bool // returns true if valid data was found
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
			time.Sleep(time.Second * 5)
			continue
		}
	}
}

// collect executes the command, parses output with the assigned parser function
func (c *gpuCollector) collect() error {
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := c.cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 8*1024)
	scanner.Buffer(buf, bufio.MaxScanTokenSize)

	for scanner.Scan() {
		hasValidData := c.parse(scanner.Bytes())
		if !hasValidData {
			return errNoValidData
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return c.cmd.Wait()
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

	return func(output []byte) bool {
		gm.mutex.Lock()
		defer gm.mutex.Unlock()
		// we get gpu name from the intitial run of nvidia-smi, so return if it hasn't been initialized
		gpuData, ok := gm.GpuDataMap["0"]
		if !ok {
			return true
		}
		data := string(output)
		// Parse RAM usage
		ramMatches := ramPattern.FindStringSubmatch(data)
		if ramMatches != nil {
			gpuData.MemoryUsed, _ = strconv.ParseFloat(ramMatches[1], 64)
			gpuData.MemoryTotal, _ = strconv.ParseFloat(ramMatches[2], 64)
		}
		// Parse GR3D (GPU) usage
		gr3dMatches := gr3dPattern.FindStringSubmatch(data)
		if gr3dMatches != nil {
			gpuData.Usage, _ = strconv.ParseFloat(gr3dMatches[1], 64)
		}
		// Parse temperature
		tempMatches := tempPattern.FindStringSubmatch(data)
		if tempMatches != nil {
			gpuData.Temperature, _ = strconv.ParseFloat(tempMatches[1], 64)
		}
		// Parse power usage
		powerMatches := powerPattern.FindStringSubmatch(data)
		if powerMatches != nil {
			power, _ := strconv.ParseFloat(powerMatches[1], 64)
			gpuData.Power = power / 1000
		}
		gpuData.Count++
		return true
	}
}

// parseNvidiaData parses the output of nvidia-smi and updates the GPUData map
func (gm *GPUManager) parseNvidiaData(output []byte) bool {
	fields := strings.Split(string(output), ", ")
	if len(fields) < 7 {
		return false
	}
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line != "" {
			fields := strings.Split(line, ", ")
			if len(fields) >= 7 {
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
					// check if tegrastats is active - if so we will only use nvidia-smi to get gpu name
					// - nvidia-smi does not provide metrics for tegra / jetson devices
					// this will end the nvidia-smi collector
					if gm.tegrastats {
						return false
					}
				}
				// update gpu data
				gpu := gm.GpuDataMap[id]
				gpu.Temperature = temp
				gpu.MemoryUsed = memoryUsage / 1.024
				gpu.MemoryTotal = totalMemory / 1.024
				gpu.Usage += usage
				gpu.Power += power
				gpu.Count++
			}
		}
	}
	return true
}

// parseAmdData parses the output of rocm-smi and updates the GPUData map
func (gm *GPUManager) parseAmdData(output []byte) bool {
	var rocmSmiInfo map[string]RocmSmiJson
	if err := json.Unmarshal(output, &rocmSmiInfo); err != nil || len(rocmSmiInfo) == 0 {
		return false
	}
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
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
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	// check for GPUs with the same name
	nameCounts := make(map[string]int)
	for _, gpu := range gm.GpuDataMap {
		nameCounts[gpu.Name]++
	}

	// copy / reset the data
	gpuData := make(map[string]system.GPUData, len(gm.GpuDataMap))
	for id, gpu := range gm.GpuDataMap {
		// sum the data
		gpu.Temperature = twoDecimals(gpu.Temperature)
		gpu.MemoryUsed = twoDecimals(gpu.MemoryUsed)
		gpu.MemoryTotal = twoDecimals(gpu.MemoryTotal)
		gpu.Usage = twoDecimals(gpu.Usage / gpu.Count)
		gpu.Power = twoDecimals(gpu.Power / gpu.Count)
		// reset the count
		gpu.Count = 1
		// dereference to avoid overwriting anything else
		gpuCopy := *gpu
		// append id to the name if there are multiple GPUs with the same name
		if nameCounts[gpu.Name] > 1 {
			gpuCopy.Name = fmt.Sprintf("%s %s", gpu.Name, id)
		}
		gpuData[id] = gpuCopy
	}
	return gpuData
}

// detectGPUs checks for the presence of GPU management tools (nvidia-smi, rocm-smi, tegrastats)
// in the system path. It sets the corresponding flags in the GPUManager struct if any of these
// tools are found. If none of the tools are found, it returns an error indicating that no GPU
// management tools are available.
func (gm *GPUManager) detectGPUs() error {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		gm.nvidiaSmi = true
	}
	if _, err := exec.LookPath("rocm-smi"); err == nil {
		gm.rocmSmi = true
	}
	if _, err := exec.LookPath("tegrastats"); err == nil {
		gm.tegrastats = true
	}
	if gm.nvidiaSmi || gm.rocmSmi || gm.tegrastats {
		return nil
	}
	return fmt.Errorf("no GPU found - install nvidia-smi, rocm-smi, or tegrastats")
}

// startCollector starts the appropriate GPU data collector based on the command
func (gm *GPUManager) startCollector(command string) {
	switch command {
	case "nvidia-smi":
		nvidia := gpuCollector{
			name: "nvidia-smi",
			cmd: exec.Command("nvidia-smi", "-l", "4",
				"--query-gpu=index,name,temperature.gpu,memory.used,memory.total,utilization.gpu,power.draw",
				"--format=csv,noheader,nounits"),
			parse: gm.parseNvidiaData,
		}
		go nvidia.start()
	case "rocm-smi":
		amdCollector := gpuCollector{
			name: "rocm-smi",
			cmd: exec.Command("/bin/sh", "-c",
				"while true; do rocm-smi --showid --showtemp --showuse --showpower --showproductname --showmeminfo vram --json; sleep 4.3; done"),
			parse: gm.parseAmdData,
		}
		go amdCollector.start()
	case "tegrastats":
		jetsonCollector := gpuCollector{
			name:  "tegrastats",
			cmd:   exec.Command("tegrastats", "--interval", "3000"),
			parse: gm.getJetsonParser(),
		}
		go jetsonCollector.start()
	}
}

// NewGPUManager creates and initializes a new GPUManager
func NewGPUManager() (*GPUManager, error) {
	var gm GPUManager
	if err := gm.detectGPUs(); err != nil {
		return nil, err
	}
	gm.GpuDataMap = make(map[string]*system.GPUData, 1)

	if gm.nvidiaSmi {
		gm.startCollector("nvidia-smi")
	}
	if gm.rocmSmi {
		gm.startCollector("rocm-smi")
	}
	if gm.tegrastats {
		gm.startCollector("tegrastats")
	}

	return &gm, nil
}
