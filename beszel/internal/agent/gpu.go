package agent

import (
	"beszel/internal/entities/system"
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slog"
)

// GPUManager manages data collection for GPUs (either Nvidia or AMD)
type GPUManager struct {
	nvidiaSmi  bool
	rocmSmi    bool
	GpuDataMap map[string]*system.GPUData
	mutex      sync.Mutex
}

// RocmSmiJson represents the JSON structure of rocm-smi output
type RocmSmiJson struct {
	ID          string `json:"Device ID"`
	Name        string `json:"Card series"`
	Temperature string `json:"Temperature (Sensor edge) (C)"`
	MemoryUsed  string `json:"VRAM Total Used Memory (B)"`
	MemoryTotal string `json:"VRAM Total Memory (B)"`
	Usage       string `json:"GPU use (%)"`
	Power       string `json:"Current Socket Graphics Package Power (W)"`
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

	hasValidData := false
	for scanner.Scan() {
		if c.parse(scanner.Bytes()) {
			hasValidData = true
		}
	}

	if !hasValidData {
		return errNoValidData
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return c.cmd.Wait()
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
		temp, _ := strconv.ParseFloat(v.Temperature, 64)
		memoryUsage, _ := strconv.ParseFloat(v.MemoryUsed, 64)
		totalMemory, _ := strconv.ParseFloat(v.MemoryTotal, 64)
		usage, _ := strconv.ParseFloat(v.Usage, 64)
		power, _ := strconv.ParseFloat(v.Power, 64)
		memoryUsage = bytesToMegabytes(memoryUsage)
		totalMemory = bytesToMegabytes(totalMemory)

		if _, ok := gm.GpuDataMap[v.ID]; !ok {
			gm.GpuDataMap[v.ID] = &system.GPUData{Name: v.Name}
		}
		gpu := gm.GpuDataMap[v.ID]
		gpu.Temperature = temp
		gpu.MemoryUsed = memoryUsage
		gpu.MemoryTotal = totalMemory
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
	// copy / reset the data
	gpuData := make(map[string]system.GPUData, len(gm.GpuDataMap))
	for id, gpu := range gm.GpuDataMap {
		// sum the data
		gpu.Temperature = twoDecimals(gpu.Temperature)
		gpu.MemoryUsed = twoDecimals(gpu.MemoryUsed)
		gpu.MemoryTotal = twoDecimals(gpu.MemoryTotal)
		gpu.Usage = twoDecimals(gpu.Usage / gpu.Count)
		gpu.Power = twoDecimals(gpu.Power / gpu.Count)
		gpuData[id] = *gpu
		// reset the count
		gpu.Count = 1
	}
	return gpuData
}

// detectGPUs returns the GPU brand (nvidia or amd) or an error if none is found
// todo: make sure there's actually a GPU, not just if the command exists
func (gm *GPUManager) detectGPUs() error {
	if err := exec.Command("nvidia-smi").Run(); err == nil {
		gm.nvidiaSmi = true
	}
	if err := exec.Command("rocm-smi").Run(); err == nil {
		gm.rocmSmi = true
	}
	if gm.nvidiaSmi || gm.rocmSmi {
		return nil
	}
	return fmt.Errorf("no GPU found - install nvidia-smi or rocm-smi")
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

	return &gm, nil
}
