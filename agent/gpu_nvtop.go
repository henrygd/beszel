package agent

import (
	"encoding/json"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

type nvtopSnapshot struct {
	DeviceName string  `json:"device_name"`
	Temp       *string `json:"temp"`
	PowerDraw  *string `json:"power_draw"`
	GpuUtil    *string `json:"gpu_util"`
	MemTotal   *string `json:"mem_total"`
	MemUsed    *string `json:"mem_used"`
}

// parseNvtopNumber parses nvtop numeric strings with units (C/W/%).
func parseNvtopNumber(raw string) float64 {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimSuffix(cleaned, "C")
	cleaned = strings.TrimSuffix(cleaned, "W")
	cleaned = strings.TrimSuffix(cleaned, "%")
	val, _ := strconv.ParseFloat(cleaned, 64)
	return val
}

// parseNvtopData parses a single nvtop JSON snapshot payload.
func (gm *GPUManager) parseNvtopData(output []byte) bool {
	var snapshots []nvtopSnapshot
	if err := json.Unmarshal(output, &snapshots); err != nil || len(snapshots) == 0 {
		return false
	}
	return gm.updateNvtopSnapshots(snapshots)
}

// updateNvtopSnapshots applies one decoded nvtop snapshot batch to GPU accumulators.
func (gm *GPUManager) updateNvtopSnapshots(snapshots []nvtopSnapshot) bool {
	gm.Lock()
	defer gm.Unlock()

	valid := false
	usedIDs := make(map[string]struct{}, len(snapshots))
	for i, sample := range snapshots {
		if sample.DeviceName == "" {
			continue
		}
		indexID := "n" + strconv.Itoa(i)
		id := indexID

		// nvtop ordering can change, so prefer reusing an existing slot with matching device name.
		if existingByIndex, ok := gm.GpuDataMap[indexID]; ok && existingByIndex.Name != "" && existingByIndex.Name != sample.DeviceName {
			for existingID, gpu := range gm.GpuDataMap {
				if !strings.HasPrefix(existingID, "n") {
					continue
				}
				if _, taken := usedIDs[existingID]; taken {
					continue
				}
				if gpu.Name == sample.DeviceName {
					id = existingID
					break
				}
			}
		}

		if _, ok := gm.GpuDataMap[id]; !ok {
			gm.GpuDataMap[id] = &system.GPUData{Name: sample.DeviceName}
		}
		gpu := gm.GpuDataMap[id]
		gpu.Name = sample.DeviceName

		if sample.Temp != nil {
			gpu.Temperature = parseNvtopNumber(*sample.Temp)
		}
		if sample.MemUsed != nil {
			gpu.MemoryUsed = bytesToMegabytes(parseNvtopNumber(*sample.MemUsed))
		}
		if sample.MemTotal != nil {
			gpu.MemoryTotal = bytesToMegabytes(parseNvtopNumber(*sample.MemTotal))
		}
		if sample.GpuUtil != nil {
			gpu.Usage += parseNvtopNumber(*sample.GpuUtil)
		}
		if sample.PowerDraw != nil {
			gpu.Power += parseNvtopNumber(*sample.PowerDraw)
		}
		gpu.Count++
		usedIDs[id] = struct{}{}
		valid = true
	}
	return valid
}

// collectNvtopStats runs nvtop loop mode and continuously decodes JSON snapshots.
func (gm *GPUManager) collectNvtopStats(interval string) error {
	cmd := exec.Command(nvtopCmd, "-lP", "-d", interval)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = stdout.Close()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	decoder := json.NewDecoder(stdout)
	foundValid := false
	for {
		var snapshots []nvtopSnapshot
		if err := decoder.Decode(&snapshots); err != nil {
			if err == io.EOF {
				if foundValid {
					return nil
				}
				return errNoValidData
			}
			return err
		}
		if gm.updateNvtopSnapshots(snapshots) {
			foundValid = true
		}
	}
}

// startNvtopCollector starts nvtop collection with retry or fallback callback handling.
func (gm *GPUManager) startNvtopCollector(interval string, onFailure func()) {
	go func() {
		failures := 0
		for {
			if err := gm.collectNvtopStats(interval); err != nil {
				if onFailure != nil {
					slog.Warn("Error collecting GPU data via nvtop", "err", err)
					onFailure()
					return
				}
				failures++
				if failures > maxFailureRetries {
					break
				}
				slog.Warn("Error collecting GPU data via nvtop", "err", err)
				time.Sleep(retryWaitTime)
				continue
			}
		}
	}()
}
