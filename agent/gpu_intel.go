package agent

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
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
	id := "i0" // prefix with i to avoid conflicts with nvidia card ids
	gpuData, ok := gm.GpuDataMap[id]
	if !ok {
		gpuData = &system.GPUData{Name: "GPU", Engines: make(map[string]float64)}
		gm.GpuDataMap[id] = gpuData
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

// collectIntelStats executes intel_gpu_top in JSON mode (-J) and parses the output.
func (gm *GPUManager) collectIntelStats() (err error) {
	// Build command arguments, optionally selecting a device via -d
	args := []string{"-s", intelGpuStatsInterval, "-J"}
	if dev, ok := utils.GetEnv("INTEL_GPU_DEVICE"); ok && dev != "" {
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

	// intel_gpu_top JSON mode wraps all samples in a single array:
	// "[", then one object per sample (comma separated), and "]" only
	// when the process exits. Since the process usually runs until it is
	// killed, the array is never fully read; each sample object is decoded
	// individually as it becomes available.
	dec := json.NewDecoder(stdout)
	var hadDataRow bool
	if _, err := dec.Token(); err != nil { // opening "[" of the sample array
		if err == io.EOF {
			return errNoValidData
		}
		return err
	}
	// Each sample is a single JSON object. More() reports false once the
	// closing "]" (or EOF, when the process is killed before the array is
	// closed) is reached; Decode() reads exactly one object and transparently
	// skips the commas the encoder emits between array elements.
	for dec.More() {
		var sample intelGpuJSONSample
		if err := dec.Decode(&sample); err != nil {
			// The process can be killed while a sample is being written; the
			// trailing truncated object is unusable but all complete samples
			// before it were already consumed.
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return err
		}
		stats := parseIntelJSONSample(sample)
		// skip first data row because it sometimes has erroneous data
		if !hadDataRow {
			hadDataRow = true
			continue
		}
		gm.updateIntelFromStats(&stats)
	}
	if !hadDataRow {
		return errNoValidData
	}
	return nil
}

// intelGpuJSONSample is a single sample from intel_gpu_top -J output.
// Only the fields we need are mapped; everything else (period, frequency,
// interrupts, rc6, imc, clients, ...) is ignored by encoding/json.
type intelGpuJSONSample struct {
	Power *struct {
		GPU     float64
		Package float64
	} `json:"power"`
	Engines map[string]struct {
		Busy float64 `json:"busy"`
	} `json:"engines"`
}

// parseIntelJSONSample maps one intel_gpu_top JSON sample into intelGpuStats.
// The engines object keys are engine class short names (RCS, BCS, VCS, VECS,
// CCS) in the default class view; physical-engine keys such as "RCS/0" are
// handled by stripping the instance suffix. Unknown keys are kept as-is so
// engines on newer/dedicated GPUs still feed the usage calculation instead of
// being silently dropped.
func parseIntelJSONSample(sample intelGpuJSONSample) (stats intelGpuStats) {
	if sample.Power != nil {
		stats.PowerGPU = sample.Power.GPU
		stats.PowerPkg = sample.Power.Package
	}
	if len(sample.Engines) > 0 {
		stats.Engines = make(map[string]float64, len(sample.Engines))
		for key, engine := range sample.Engines {
			// Physical-engine keys look like "RCS/0"; strip the instance
			// suffix so they map to the same class as in the class view.
			name := key
			if idx := strings.IndexByte(key, '/'); idx >= 0 {
				if rest := key[idx+1:]; rest != "" && isDigits(rest) {
					name = key[:idx]
				}
			}
			switch name {
			case "RCS":
				stats.Engines["Render/3D"] += engine.Busy
			case "BCS":
				stats.Engines["Blitter"] += engine.Busy
			case "VCS":
				stats.Engines["Video"] += engine.Busy
			case "VECS":
				stats.Engines["VideoEnhance"] += engine.Busy
			case "CCS":
				stats.Engines["Compute"] += engine.Busy
			default:
				stats.Engines[name] += engine.Busy
			}
		}
	}
	return stats
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
