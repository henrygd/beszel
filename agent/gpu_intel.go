package agent

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	intelGpuStatsCmd      string = "intel_gpu_top"
	intelGpuStatsInterval string = "3300" // in milliseconds
)

type intelGpuStats struct {
	Power struct {
		GPU float64 `json:"GPU"`
	} `json:"power"`
	Engines map[string]struct {
		Busy float64 `json:"busy"`
	} `json:"engines"`
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

	if sample.Power.GPU > 0 {
		gpuData.Power += sample.Power.GPU
	}

	if gpuData.Engines == nil {
		gpuData.Engines = make(map[string]float64, len(sample.Engines))
	}
	for name, engine := range sample.Engines {
		gpuData.Engines[name] += engine.Busy
	}

	gpuData.Count++
	return true
}

// collectIntelStats executes intel_gpu_top in JSON mode and stream-decodes the array of samples
func (gm *GPUManager) collectIntelStats() error {
	cmd := exec.Command(intelGpuStatsCmd, "-s", intelGpuStatsInterval, "-J")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	dec := json.NewDecoder(stdout)

	// Expect a JSON array stream: [ { ... }, { ... }, ... ]
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("unexpected JSON start token: %v", tok)
	}

	var sample intelGpuStats
	for {
		if dec.More() {
			// Clear the engines map before decoding
			if sample.Engines != nil {
				for k := range sample.Engines {
					delete(sample.Engines, k)
				}
			}

			if err := dec.Decode(&sample); err != nil {
				return fmt.Errorf("decode intel gpu: %w", err)
			}
			gm.updateIntelFromStats(&sample)
			continue
		}
		// Attempt to read closing bracket (will only be present when process exits)
		tok, err = dec.Token()
		if err != nil {
			// When the process is still running, decoder will block in More/Decode; any error here is terminal
			return err
		}
		if delim, ok := tok.(json.Delim); ok && delim == ']' {
			break
		}
	}

	return cmd.Wait()
}
