package agent

import (
	"bufio"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	intelGpuStatsCmd      string = "intel_gpu_top"
	intelGpuStatsInterval string = "3300" // in milliseconds
)

type intelGpuStats struct {
	PowerGPU float64
	PowerPkg float64
	Engines  map[string]float64
}

// updateIntelFromStats updates aggregated GPU data from a single intelGpuStats sample
func (gm *GPUManager) updateIntelFromStats(sample *intelGpuStats) bool {
	gm.Lock()
	defer gm.Unlock()

	// only one gpu for now - cmd doesn't provide all by default
	gpuData, ok := gm.GpuDataMap["0"]
	if !ok {
		gpuData = &system.GPUData{Name: "GPU", Engines: make(map[string]float64)}
		gm.GpuDataMap["0"] = gpuData
	}

	gpuData.Power += sample.PowerGPU
	gpuData.PowerPkg += sample.PowerPkg

	if gpuData.Engines == nil {
		gpuData.Engines = make(map[string]float64, len(sample.Engines))
	}
	for name, engine := range sample.Engines {
		gpuData.Engines[name] += engine
	}

	gpuData.Count++
	return true
}

// collectIntelStats executes intel_gpu_top in text mode (-l) and parses the output
func (gm *GPUManager) collectIntelStats() (err error) {
	// Build command arguments, optionally selecting a device via -d
	args := []string{"-s", intelGpuStatsInterval, "-l"}
	if dev, ok := GetEnv("INTEL_GPU_DEVICE"); ok && dev != "" {
		args = append(args, "-d", dev)
	}
	cmd := exec.Command(intelGpuStatsCmd, args...)
	// Avoid blocking if intel_gpu_top writes to stderr
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
		// Best-effort close of the pipe (unblock the child if it writes)
		_ = stdout.Close()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
		if waitErr := cmd.Wait(); err == nil && waitErr != nil {
			err = waitErr
		}
	}()

	scanner := bufio.NewScanner(stdout)
	var header1 string
	var engineNames []string
	var friendlyNames []string
	var preEngineCols int
	var powerIndex int
	var hadDataRow bool
	// skip first data row because it sometimes has erroneous data
	var skippedFirstDataRow bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// first header line
		if strings.HasPrefix(line, "Freq") {
			header1 = line
			continue
		}

		// second header line
		if strings.HasPrefix(line, "req") {
			engineNames, friendlyNames, powerIndex, preEngineCols = gm.parseIntelHeaders(header1, line)
			continue
		}

		// Data row
		if !skippedFirstDataRow {
			skippedFirstDataRow = true
			continue
		}
		sample, err := gm.parseIntelData(line, engineNames, friendlyNames, powerIndex, preEngineCols)
		if err != nil {
			return err
		}
		hadDataRow = true
		gm.updateIntelFromStats(&sample)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return scanErr
	}
	if !hadDataRow {
		return errNoValidData
	}
	return nil
}

func (gm *GPUManager) parseIntelHeaders(header1 string, header2 string) (engineNames []string, friendlyNames []string, powerIndex int, preEngineCols int) {
	// Build indexes
	h1 := strings.Fields(header1)
	h2 := strings.Fields(header2)
	powerIndex = -1 // Initialize to -1, will be set to actual index if found
	// Collect engine names from header1
	for _, col := range h1 {
		key := strings.TrimRightFunc(col, func(r rune) bool {
			return (r >= '0' && r <= '9') || r == '/'
		})
		var friendly string
		switch key {
		case "RCS":
			friendly = "Render/3D"
		case "BCS":
			friendly = "Blitter"
		case "VCS":
			friendly = "Video"
		case "VECS":
			friendly = "VideoEnhance"
		case "CCS":
			friendly = "Compute"
		default:
			continue
		}
		engineNames = append(engineNames, key)
		friendlyNames = append(friendlyNames, friendly)
	}
	// find power gpu index among pre-engine columns
	if n := len(engineNames); n > 0 {
		preEngineCols = max(len(h2)-3*n, 0)
		limit := min(len(h2), preEngineCols)
		for i := range limit {
			if strings.EqualFold(h2[i], "gpu") {
				powerIndex = i
				break
			}
		}
	}
	return engineNames, friendlyNames, powerIndex, preEngineCols
}

func (gm *GPUManager) parseIntelData(line string, engineNames []string, friendlyNames []string, powerIndex int, preEngineCols int) (sample intelGpuStats, err error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return sample, errNoValidData
	}
	// Make sure row has enough columns for engines
	if need := preEngineCols + 3*len(engineNames); len(fields) < need {
		return sample, errNoValidData
	}
	if powerIndex >= 0 && powerIndex < len(fields) {
		if v, perr := strconv.ParseFloat(fields[powerIndex], 64); perr == nil {
			sample.PowerGPU = v
		}
		if v, perr := strconv.ParseFloat(fields[powerIndex+1], 64); perr == nil {
			sample.PowerPkg = v
		}
	}
	if len(engineNames) > 0 {
		sample.Engines = make(map[string]float64, len(engineNames))
		for k := range engineNames {
			base := preEngineCols + 3*k
			if base < len(fields) {
				busy := 0.0
				if v, e := strconv.ParseFloat(fields[base], 64); e == nil {
					busy = v
				}
				cur := sample.Engines[friendlyNames[k]]
				sample.Engines[friendlyNames[k]] = cur + busy
			} else {
				sample.Engines[friendlyNames[k]] = 0
			}
		}
	}
	return sample, nil
}
