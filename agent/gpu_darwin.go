//go:build darwin

package agent

import (
	"bufio"
	"bytes"
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
)

// startPowermetricsCollector runs powermetrics --samplers gpu_power in a loop and updates
// GPU usage and power. Requires root (sudo) on macOS. A single logical GPU is reported as id "0".
func (gm *GPUManager) startPowermetricsCollector() {
	// Ensure single GPU entry for Apple GPU
	const appleGPUID = "0"
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

	const appleGPUID = "0"
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
