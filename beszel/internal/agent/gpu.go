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

type GPUManager struct {
	nvidiaSmi  bool
	rocmSmi    bool
	GpuDataMap map[string]*system.GPUData
	mutex      sync.Mutex
}

type RocmSmiJson struct {
	ID          string `json:"Device ID"`
	Name        string `json:"Card series"`
	Temperature string `json:"Temperature (Sensor edge) (C)"`
	MemoryUsed  string `json:"VRAM Total Used Memory (B)"`
	MemoryTotal string `json:"VRAM Total Memory (B)"`
	Usage       string `json:"GPU use (%)"`
	Power       string `json:"Current Socket Graphics Package Power (W)"`
}

// startNvidiaCollector oversees collectNvidiaStats and restarts nvidia-smi if it fails
func (gm *GPUManager) startNvidiaCollector() error {
	for {
		if err := gm.collectNvidiaStats(); err != nil {
			slog.Warn("Restarting nvidia-smi", "err", err)
			time.Sleep(time.Second) // Wait before retrying
			continue
		}
	}
}

// collectNvidiaStats runs nvidia-smi in a loop and passes the output to parseNvidiaData
func (gm *GPUManager) collectNvidiaStats() error {
	// Set up the command
	cmd := exec.Command("nvidia-smi", "-l", "4", "--query-gpu=index,name,temperature.gpu,memory.used,memory.total,utilization.gpu,power.draw", "--format=csv,noheader,nounits")
	// Set up a pipe to capture stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// Start the command
	if err := cmd.Start(); err != nil {
		return err
	}
	// Use a scanner to read each line of output
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024) // 64KB buffer
	scanner.Buffer(buf, bufio.MaxScanTokenSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		gm.parseNvidiaData(line) // Run your function on each new line
	}
	// Check for any errors encountered during scanning
	if err := scanner.Err(); err != nil {
		return err
	}
	// Wait for the command to complete
	return cmd.Wait()
}

// parseNvidiaData parses the output of nvidia-smi and updates the GPUData map
func (gm *GPUManager) parseNvidiaData(output []byte) {
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
}

// startAmdCollector oversees collectAmdStats and restarts rocm-smi if it fails
func (gm *GPUManager) startAmdCollector() {
	for {
		if err := gm.collectAmdStats(); err != nil {
			slog.Warn("Restarting rocm-smi", "err", err)
			time.Sleep(time.Second) // Wait before retrying
			continue
		} else {
			// break if no error (command runs but no card found)
			break
		}
	}
}

// collectAmdStats runs rocm-smi in a loop and passes the output to parseAmdData
func (gm *GPUManager) collectAmdStats() error {
	cmd := exec.Command("/bin/sh", "-c", "while true; do rocm-smi --showid --showtemp --showuse --showpower --showproductname --showmeminfo vram --json; sleep 3.7; done")
	// Set up a pipe to capture stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// Start the command
	if err := cmd.Start(); err != nil {
		return err
	}
	// Use a scanner to read each line of output
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024) // 64KB buffer
	scanner.Buffer(buf, bufio.MaxScanTokenSize)
	for scanner.Scan() {
		var rocmSmiInfo map[string]RocmSmiJson
		if err := json.Unmarshal(scanner.Bytes(), &rocmSmiInfo); err != nil {
			return err
		}
		if len(rocmSmiInfo) > 0 {
			// slog.Info("rocm-smi", "data", rocmSmiInfo)
			gm.parseAmdData(&rocmSmiInfo)
		} else {
			slog.Warn("rocm-smi returned no GPU")
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return cmd.Wait()
}

// parseAmdData parses the output of rocm-smi and updates the GPUData map
func (gm *GPUManager) parseAmdData(rocmSmiInfo *map[string]RocmSmiJson) {
	for _, v := range *rocmSmiInfo {
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

// detectGPU returns the GPU brand (nvidia or amd) or an error if none is found
// todo: make sure there's actually a GPU, not just if the command exists
func (gm *GPUManager) detectGPU() error {
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

// NewGPUManager returns a new GPUManager
func NewGPUManager() (*GPUManager, error) {
	var gm GPUManager
	if err := gm.detectGPU(); err != nil {
		return nil, err
	}
	gm.GpuDataMap = make(map[string]*system.GPUData, 1)
	if gm.nvidiaSmi {
		go gm.startNvidiaCollector()
	}
	if gm.rocmSmi {
		go gm.startAmdCollector()
	}
	return &gm, nil
}
