//go:build darwin

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	// powermetricsSampleIntervalMs is the sampling interval passed to powermetrics (-i).
	powermetricsSampleIntervalMs = 500
	// powermetricsPollInterval is how often we run powermetrics to collect a new sample.
	powermetricsPollInterval = 2 * time.Second
	// powermetricsCommandTimeout bounds one finite powermetrics invocation.
	powermetricsCommandTimeout = 5 * time.Second

	// macmonIntervalMs is the sampling interval passed to macmon pipe (-i), in milliseconds.
	macmonIntervalMs = 2500
	// macmonHealthTimeout bounds the gap since the last valid macmon sample.
	macmonHealthTimeout = 10 * time.Second

	// appleGPUDataStaleAfter omits Apple GPU data when a collector has gone quiet.
	appleGPUDataStaleAfter = 15 * time.Second
)

const appleGPUID = "0"

// startPowermetricsCollector runs powermetrics --samplers gpu_power in a loop and updates
// GPU usage and power. Requires root (sudo) on macOS. A single logical GPU is reported as id "0".
func (gm *GPUManager) startPowermetricsCollector() {
	go gm.runPowermetricsCollector(context.Background(), powermetricsCommandTimeout, retryWaitTime)
}

func (gm *GPUManager) runPowermetricsCollector(ctx context.Context, commandTimeout, retryWait time.Duration) {
	// A stopped collector must not leave stale Apple GPU averages behind.
	defer gm.removeAppleGPUData()

	failures := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if err := gm.collectPowermetricsWithTimeout(ctx, commandTimeout); err != nil {
			failures++
			if failures > maxFailureRetries {
				slog.Warn("powermetrics GPU collector failed repeatedly, stopping", "err", err)
				return
			}
			slog.Warn("Error collecting macOS GPU data via powermetrics (may require sudo)", "err", err)
			if !waitForAppleCollector(ctx, retryWait) {
				return
			}
			continue
		}
		failures = 0
		if !waitForAppleCollector(ctx, powermetricsPollInterval) {
			return
		}
	}
}

// collectPowermetricsWithTimeout runs powermetrics once and parses GPU usage and power from its output.
func (gm *GPUManager) collectPowermetricsWithTimeout(parent context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	interval := strconv.Itoa(powermetricsSampleIntervalMs)
	cmd := exec.CommandContext(ctx, powermetricsCmd, "--samplers", "gpu_power", "-i", interval, "-n", "1")
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 250 * time.Millisecond
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return fmt.Errorf("powermetrics execution: %w", ctx.Err())
	}
	if err != nil {
		return err
	}
	if !gm.parsePowermetricsData(out) {
		return errNoValidData
	}
	return nil
}

// parsePowermetricsData parses powermetrics gpu_power output and updates GpuDataMap["0"].
// Both GPU usage and GPU power must be present and valid. Example output:
//
//	**** GPU usage ****
//	GPU HW active residency:   0.00% (444 MHz: .00% 612 MHz:   0%)
//	GPU idle residency:  100.00%
//	GPU Power: 0 mW
func (gm *GPUManager) parsePowermetricsData(output []byte) bool {
	var usage, powerWatts float64
	var usageFound, powerFound, usageValid, powerValid bool

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "GPU HW active residency:") {
			usageFound = true
			usage, usageValid = parsePowermetricsUsage(line)
		} else if strings.HasPrefix(line, "GPU Power:") {
			powerFound = true
			powerWatts, powerValid = parsePowermetricsPower(line)
		}
	}
	if scanner.Err() != nil || !usageFound || !powerFound || !usageValid || !powerValid {
		return false
	}

	gm.Lock()
	defer gm.Unlock()

	gpu, ok := gm.GpuDataMap[appleGPUID]
	if !ok {
		gpu = &system.GPUData{Name: "Apple GPU"}
		gm.GpuDataMap[appleGPUID] = gpu
	}
	gpu.Usage += usage
	gpu.Power += powerWatts
	gpu.Count++
	gm.noteAppleGPUSampleLocked(time.Now())
	return true
}

func parsePowermetricsUsage(line string) (float64, bool) {
	fields := strings.Fields(strings.TrimPrefix(line, "GPU HW active residency:"))
	if len(fields) == 0 || !strings.HasSuffix(fields[0], "%") {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSuffix(fields[0], "%"), 64)
	if err != nil || !isFiniteMetric(value) || value < 0 || value > 100 {
		return 0, false
	}
	return value, true
}

func parsePowermetricsPower(line string) (float64, bool) {
	fields := strings.Fields(strings.TrimPrefix(line, "GPU Power:"))
	if len(fields) < 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || !isFiniteMetric(value) || value < 0 {
		return 0, false
	}
	switch strings.ToLower(fields[1]) {
	case "w":
	case "mw":
		value /= milliwattsInAWatt
	case "kw":
		value *= 1000
	default:
		return 0, false
	}
	return value, isFiniteMetric(value)
}

// startMacmonCollector runs `macmon pipe` in a loop and parses one JSON object per line.
// This collector does not require sudo. A single logical GPU is reported as id "0".
func (gm *GPUManager) startMacmonCollector() {
	go gm.runMacmonCollector(context.Background(), macmonHealthTimeout, retryWaitTime)
}

func (gm *GPUManager) runMacmonCollector(ctx context.Context, healthTimeout, retryWait time.Duration) {
	defer gm.removeAppleGPUData()

	failures := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if err := gm.collectMacmonPipeWithHealthTimeout(ctx, healthTimeout); err != nil {
			failures++
			if failures > maxFailureRetries {
				slog.Warn("macmon GPU collector failed repeatedly, stopping", "err", err)
				return
			}
			slog.Warn("Error collecting macOS GPU data via macmon", "err", err)
			if !waitForAppleCollector(ctx, retryWait) {
				return
			}
			continue
		}
		failures = 0
		// `macmon pipe` is long-running; if it returns, wait a bit before restarting.
		if !waitForAppleCollector(ctx, retryWait) {
			return
		}
	}
}

type macmonTemp struct {
	GPUTempAvg float64 `json:"gpu_temp_avg"`
}

type macmonSample struct {
	GPUPower    *float64    `json:"gpu_power"`     // watts (macmon reports fractional values)
	GPURAMPower *float64    `json:"gpu_ram_power"` // watts
	GPUUsage    *[]*float64 `json:"gpu_usage"`     // [freq_mhz, usage] where usage is 0..1
	Temp        macmonTemp  `json:"temp"`
}

type macmonReadResult struct {
	line []byte
	err  error
	done bool
}

func (gm *GPUManager) collectMacmonPipeWithHealthTimeout(ctx context.Context, healthTimeout time.Duration) (err error) {
	macmonPath, err := utils.LookPathHomebrew(macmonCmd)
	if err != nil {
		return err
	}
	cmd := exec.Command(macmonPath, "pipe", "-i", strconv.Itoa(macmonIntervalMs))
	// Avoid blocking if macmon writes to stderr.
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 250 * time.Millisecond
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	readerStop := make(chan struct{})
	defer func() {
		close(readerStop)
		_ = stdout.Close()
		// Always kill and reap the child, including timeout and malformed-output paths.
		_ = cmd.Process.Kill()
		waitErr := cmd.Wait()
		if err == nil && waitErr != nil {
			err = waitErr
		}
	}()

	results := make(chan macmonReadResult)
	go func() {
		defer close(results)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			result := macmonReadResult{line: bytes.Clone(scanner.Bytes())}
			select {
			case results <- result:
			case <-readerStop:
				return
			}
		}
		result := macmonReadResult{err: scanner.Err(), done: true}
		select {
		case results <- result:
		case <-readerStop:
		}
	}()

	timer := time.NewTimer(healthTimeout)
	defer timer.Stop()
	var hadValidSample bool
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("macmon health timeout: %w", errNoValidData)
		case result, ok := <-results:
			if !ok {
				if !hadValidSample {
					return errNoValidData
				}
				return nil
			}
			if result.done {
				if result.err != nil {
					return result.err
				}
				if !hadValidSample {
					return errNoValidData
				}
				return nil
			}
			if gm.parseMacmonLine(result.line) {
				hadValidSample = true
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(healthTimeout)
			}
		}
	}
}

// parseMacmonLine parses a single macmon JSON line and updates Apple GPU metrics.
// gpu_usage and gpu_power are required; both are valid at zero. GPU usage is a
// real 0..1 ratio and is never guessed to be a percentage.
func (gm *GPUManager) parseMacmonLine(line []byte) bool {
	var sample macmonSample
	if err := json.Unmarshal(line, &sample); err != nil {
		return false
	}
	if sample.GPUUsage == nil || len(*sample.GPUUsage) < 2 {
		return false
	}
	frequency := (*sample.GPUUsage)[0]
	usageRatioPtr := (*sample.GPUUsage)[1]
	if frequency == nil || usageRatioPtr == nil || !isFiniteMetric(*frequency) || !isFiniteMetric(*usageRatioPtr) {
		return false
	}
	usageRatio := *usageRatioPtr
	if usageRatio < 0 || usageRatio > 1 {
		return false
	}
	if sample.GPUPower == nil || !isFiniteMetric(*sample.GPUPower) || *sample.GPUPower < 0 {
		return false
	}
	power := *sample.GPUPower
	if sample.GPURAMPower != nil {
		if !isFiniteMetric(*sample.GPURAMPower) || *sample.GPURAMPower < 0 {
			return false
		}
		power += *sample.GPURAMPower
	}
	if !isFiniteMetric(power) {
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
	gpu.Usage += usageRatio * 100
	// macmon reports power in watts. gpu_ram_power is memory-subsystem power, not VRAM capacity.
	gpu.Power += power
	gpu.Count++
	gm.noteAppleGPUSampleLocked(time.Now())
	return true
}

func (gm *GPUManager) noteAppleGPUSampleLocked(at time.Time) {
	gm.appleGPUHaveSample = true
	gm.appleGPULastSample = at
}

// removeAppleGPUData removes the Apple GPU and all per-cache-key stale state.
func (gm *GPUManager) removeAppleGPUData() {
	gm.Lock()
	defer gm.Unlock()
	gm.removeAppleGPUDataLocked()
}

func (gm *GPUManager) removeAppleGPUDataLocked() {
	delete(gm.GpuDataMap, appleGPUID)
	delete(gm.lastAvgData, appleGPUID)
	for _, snapshots := range gm.lastSnapshots {
		delete(snapshots, appleGPUID)
	}
	gm.appleGPUHaveSample = false
	gm.appleGPULastSample = time.Time{}
}

// pruneAppleGPUData omits Apple GPU data after a long gap without a valid sample.
func (gm *GPUManager) pruneAppleGPUData() {
	gm.Lock()
	defer gm.Unlock()
	if !gm.appleGPUHaveSample || time.Since(gm.appleGPULastSample) < appleGPUDataStaleAfter {
		return
	}
	gm.removeAppleGPUDataLocked()
}

func (gm *GPUManager) isAppleGPU(name string) bool {
	return name == "Apple GPU"
}

func isFiniteMetric(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func waitForAppleCollector(ctx context.Context, wait time.Duration) bool {
	if wait <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
