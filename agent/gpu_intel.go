package agent

import (
	"encoding/json"
	"log/slog"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	intelGpuStatsCmd      string = "intel_gpu_top"
	intelGpuStatsInterval string = "3800" // in milliseconds
)

type intelGpuStats struct {
	Power struct {
		GPU float64 `json:"gpu"`
	} `json:"power"`
	Engines map[string]struct {
		Busy float64 `json:"busy"`
	} `json:"engines"`
}

func (gm *GPUManager) parseIntelData(output []byte) bool {
	slog.Info("Parsing Intel GPU stats")
	var intelGpuStats intelGpuStats
	if err := json.Unmarshal(output, &intelGpuStats); err != nil {
		slog.Error("Error parsing Intel GPU stats", "err", err)
		return false
	}
	gm.Lock()
	defer gm.Unlock()

	// only one gpu for now - cmd doesn't provide all by default
	gpuData, ok := gm.GpuDataMap["0"]
	if !ok {
		gpuData = &system.GPUData{Name: "GPU", Engines: make(map[string]float64, len(intelGpuStats.Engines))}
		gm.GpuDataMap["0"] = gpuData
	}

	if intelGpuStats.Power.GPU > 0 {
		gpuData.Power += intelGpuStats.Power.GPU
	}

	for name, engine := range intelGpuStats.Engines {
		gpuData.Engines[name] += engine.Busy
	}

	gpuData.Count++

	slog.Info("GPU Data", "gpuData", gpuData)
	return true
}
