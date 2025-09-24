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
func (gm *GPUManager) collectIntelStats() error {
	cmd := exec.Command(intelGpuStatsCmd, "-s", intelGpuStatsInterval, "-l")
	// Avoid blocking if intel_gpu_top writes to stderr
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Ensure we always reap the child to avoid zombies on any return path.
	defer func() {
		// Best-effort close of the pipe (unblock the child if it writes)
		_ = stdout.Close()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	var header1 string
	var header2 string
	var engineNames []string
	var friendlyNames []string
	var preEngineCols int
	var powerIndex int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// first header line
		if header1 == "" {
			header1 = line
			continue
		}

		// second header line
		if header2 == "" {
			engineNames, friendlyNames, powerIndex, preEngineCols = gm.parseIntelHeaders(header1, line)
			header1, header2 = "x", "x" // don't need these anymore
			continue
		}

		// Data row
		sample := gm.parseIntelData(line, engineNames, friendlyNames, powerIndex, preEngineCols)
		gm.updateIntelFromStats(&sample)
	}
	if err := scanner.Err(); err != nil {
		return err
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
		key := strings.TrimRightFunc(col, func(r rune) bool { return r >= '0' && r <= '9' })
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

func (gm *GPUManager) parseIntelData(line string, engineNames []string, friendlyNames []string, powerIndex int, preEngineCols int) (sample intelGpuStats) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return sample
	}
	// Make sure row has enough columns for engines
	if need := preEngineCols + 3*len(engineNames); len(fields) < need {
		return sample
	}
	if powerIndex >= 0 && powerIndex < len(fields) {
		if v, perr := strconv.ParseFloat(fields[powerIndex], 64); perr == nil {
			sample.PowerGPU = v
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
	return sample
}
