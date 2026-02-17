//go:build darwin

package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	// powermetricsSampleIntervalMs is the sampling interval passed to powermetrics (-i).
	powermetricsSampleIntervalMs = 500
	// powermetricsPollInterval is how often we run powermetrics to collect a new sample.
	powermetricsPollInterval = 2 * time.Second
	// macmonIntervalMs is the sampling interval passed to macmon pipe (-i), in milliseconds.
	macmonIntervalMs = 2500
)

const appleGPUID = "0"

// startPowermetricsCollector runs powermetrics --samplers gpu_power in a loop and updates
// GPU usage and power. Requires root (sudo) on macOS. A single logical GPU is reported as id "0".
func (gm *GPUManager) startPowermetricsCollector() {
	// Ensure single GPU entry for Apple GPU
	if _, ok := gm.GpuDataMap[appleGPUID]; !ok {
		gm.GpuDataMap[appleGPUID] = &system.GPUData{Name: "Apple GPU"}
	}

	go func() {
		failures := 0
		for {
			if err := gm.collectPowermetrics(); err != nil {
				failures++
				if failures > maxFailureRetries {
					slog.Warn("powermetrics GPU collector failed repeatedly, stopping", "err", err)
					break
				}
				slog.Warn("Error collecting macOS GPU data via powermetrics (may require sudo)", "err", err)
				time.Sleep(retryWaitTime)
				continue
			}
			failures = 0
			time.Sleep(powermetricsPollInterval)
		}
	}()
}

// collectPowermetrics runs powermetrics once and parses GPU usage and power from its output.
func (gm *GPUManager) collectPowermetrics() error {
	interval := strconv.Itoa(powermetricsSampleIntervalMs)
	cmd := exec.Command(powermetricsCmd, "--samplers", "gpu_power", "-i", interval, "-n", "1")
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	if !gm.parsePowermetricsData(out) {
		return errNoValidData
	}
	return nil
}

// parsePowermetricsData parses powermetrics gpu_power output and updates GpuDataMap["0"].
// Example output:
//
//	**** GPU usage ****
//	GPU HW active frequency: 444 MHz
//	GPU HW active residency:   0.97% (444 MHz: .97% ...
//	GPU idle residency:  99.03%
//	GPU Power: 4 mW
func (gm *GPUManager) parsePowermetricsData(output []byte) bool {
	var idleResidency, powerMW float64
	var gotIdle, gotPower bool

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "GPU idle residency:") {
			// "GPU idle residency:  99.03%"
			fields := strings.Fields(strings.TrimPrefix(line, "GPU idle residency:"))
			if len(fields) >= 1 {
				pct := strings.TrimSuffix(fields[0], "%")
				if v, err := strconv.ParseFloat(pct, 64); err == nil {
					idleResidency = v
					gotIdle = true
				}
			}
		} else if strings.HasPrefix(line, "GPU Power:") {
			// "GPU Power: 4 mW"
			fields := strings.Fields(strings.TrimPrefix(line, "GPU Power:"))
			if len(fields) >= 1 {
				if v, err := strconv.ParseFloat(fields[0], 64); err == nil {
					powerMW = v
					gotPower = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return false
	}
	if !gotIdle && !gotPower {
		return false
	}

	gm.Lock()
	defer gm.Unlock()

	if _, ok := gm.GpuDataMap[appleGPUID]; !ok {
		gm.GpuDataMap[appleGPUID] = &system.GPUData{Name: "Apple GPU"}
	}
	gpu := gm.GpuDataMap[appleGPUID]

	if gotIdle {
		// Usage = 100 - idle residency (e.g. 100 - 99.03 = 0.97%)
		gpu.Usage += 100 - idleResidency
	}
	if gotPower {
		// mW -> W
		gpu.Power += powerMW / milliwattsInAWatt
	}
	gpu.Count++
	return true
}

// startMacmonCollector runs `macmon pipe` in a loop and parses one JSON object per line.
// This collector does not require sudo. A single logical GPU is reported as id "0".
func (gm *GPUManager) startMacmonCollector() {
	if _, ok := gm.GpuDataMap[appleGPUID]; !ok {
		gm.GpuDataMap[appleGPUID] = &system.GPUData{Name: "Apple GPU"}
	}

	go func() {
		failures := 0
		for {
			if err := gm.collectMacmonPipe(); err != nil {
				failures++
				if failures > maxFailureRetries {
					slog.Warn("macmon GPU collector failed repeatedly, stopping", "err", err)
					break
				}
				slog.Warn("Error collecting macOS GPU data via macmon", "err", err)
				time.Sleep(retryWaitTime)
				continue
			}
			failures = 0
			// `macmon pipe` is long-running; if it returns, wait a bit before restarting.
			time.Sleep(retryWaitTime)
		}
	}()
}

type macmonTemp struct {
	GPUTempAvg float64 `json:"gpu_temp_avg"`
}

type macmonSample struct {
	GPUPower    float64    `json:"gpu_power"`     // watts (macmon reports fractional values)
	GPURAMPower float64    `json:"gpu_ram_power"` // watts
	GPUUsage    []float64  `json:"gpu_usage"`     // [freq_mhz, usage] where usage is typically 0..1
	Temp        macmonTemp `json:"temp"`
}

func (gm *GPUManager) collectMacmonPipe() (err error) {
	cmd := exec.Command(macmonCmd, "pipe", "-i", strconv.Itoa(macmonIntervalMs))
	// Avoid blocking if macmon writes to stderr.
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Ensure we always reap the child to avoid zombies on any return path and
	// propagate a non-zero exit code if no other error was set.
	defer func() {
		_ = stdout.Close()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
		if waitErr := cmd.Wait(); err == nil && waitErr != nil {
			err = waitErr
		}
	}()

	scanner := bufio.NewScanner(stdout)
	var hadSample bool
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if gm.parseMacmonLine(line) {
			hadSample = true
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return scanErr
	}
	if !hadSample {
		return errNoValidData
	}
	return nil
}

// parseMacmonLine parses a single macmon JSON line and updates Apple GPU metrics.
func (gm *GPUManager) parseMacmonLine(line []byte) bool {
	var sample macmonSample
	if err := json.Unmarshal(line, &sample); err != nil {
		return false
	}

	usage := 0.0
	if len(sample.GPUUsage) >= 2 {
		usage = sample.GPUUsage[1]
		// Heuristic: macmon typically reports 0..1; convert to percentage.
		if usage <= 1.0 {
			usage *= 100
		}
	}

	// Consider the line valid if it contains at least one GPU metric.
	if usage == 0 && sample.GPUPower == 0 && sample.Temp.GPUTempAvg == 0 {
		return false
	}

	gm.Lock()
	defer gm.Unlock()

	gpu, ok := gm.GpuDataMap[appleGPUID]
	if !ok {
		gpu = &system.GPUData{Name: "Apple GPU"}
		gm.GpuDataMap[appleGPUID] = gpu
	}
	gpu.Temperature = sample.Temp.GPUTempAvg
	gpu.Usage += usage
	// macmon reports power in watts; include VRAM power if present.
	gpu.Power += sample.GPUPower + sample.GPURAMPower
	gpu.Count++
	return true
}
