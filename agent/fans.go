package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

// updateFans populates systemStats.Fans from the host's hwmon sysfs tree.
// No-op on platforms where hwmon isn't available (see fans_other.go).
func (a *Agent) updateFans(systemStats *system.Stats) {
	a.systemInfo.DashboardFan = 0
	if hwmonRoot == "" {
		return
	}
	fans, err := readHwmonFans(hwmonRoot)
	if err != nil {
		slog.Debug("Error reading fans", "err", err)
		return
	}
	if len(fans) == 0 {
		return
	}
	systemStats.Fans = fans
	// Compute the single "dashboard" value used by the FanSpeed alert.
	// Per-sensor RPMs live in Stats.Fans and drive the multi-line FanChart
	// in the UI; the alert path only needs one number to compare against
	// the user's threshold, so we use the highest RPM across all fans —
	// mirrors DashboardTemp semantics (max temp) so the alert direction
	// stays consistent across the app.
	for _, rpm := range fans {
		if rpm > a.systemInfo.DashboardFan {
			a.systemInfo.DashboardFan = rpm
		}
	}
}

// readHwmonFans walks the given hwmon root (typically /sys/class/hwmon) and
// returns a map of "<chip>_<label-or-fan-idx>" → RPM for every fan*_input
// file it finds. Zero RPM is retained because it can represent a real fan that
// has stopped; negative and malformed readings are ignored.
func readHwmonFans(root string) (map[string]uint16, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	fans := make(map[string]uint16)
	for _, entry := range entries {
		chipDir := filepath.Join(root, entry.Name())
		chipName := utils.ReadStringFile(filepath.Join(chipDir, "name"))
		if chipName == "" {
			chipName = entry.Name()
		}
		inputs, _ := filepath.Glob(filepath.Join(chipDir, "fan*_input"))
		for _, inputPath := range inputs {
			base := strings.TrimSuffix(filepath.Base(inputPath), "_input")
			rpm, ok := utils.ReadUintFile(inputPath)
			if !ok {
				continue
			}
			label := utils.ReadStringFile(filepath.Join(chipDir, base+"_label"))
			key := chipName + "_" + base
			if label != "" {
				key = chipName + "_" + label
			}
			fans[key] = uint16(rpm)
		}
	}
	return fans, nil
}
