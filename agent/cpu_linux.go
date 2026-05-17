//go:build linux

package agent

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
)

// getCpuFrequencies reads per-core current CPU frequency in GHz from sysfs.
// Returns nil if unavailable (no cpufreq driver or sysfs not mounted).
func getCpuFrequencies() []float64 {
	entries, err := os.ReadDir("/sys/devices/system/cpu")
	if err != nil {
		return nil
	}

	type cpuFreq struct {
		idx  int
		freq float64
	}

	var freqs []cpuFreq
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "cpu") {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(name, "cpu"))
		if err != nil {
			continue
		}
		kHz, ok := utils.ReadUintFile("/sys/devices/system/cpu/" + name + "/cpufreq/scaling_cur_freq")
		if !ok {
			continue
		}
		freqs = append(freqs, cpuFreq{idx: idx, freq: utils.TwoDecimals(float64(kHz) / 1e6)})
	}

	if len(freqs) == 0 {
		return nil
	}

	sort.Slice(freqs, func(i, j int) bool { return freqs[i].idx < freqs[j].idx })

	result := make([]float64, len(freqs))
	for i, f := range freqs {
		result[i] = f.freq
	}
	return result
}
