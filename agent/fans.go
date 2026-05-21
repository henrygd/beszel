package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/system"
)

// updateFans populates systemStats.Fans from the host's hwmon sysfs tree.
// No-op on platforms where hwmon isn't available (see fans_other.go).
func (a *Agent) updateFans(systemStats *system.Stats) {
	if hwmonRoot == "" {
		return
	}
	fans, err := readHwmonFans(hwmonRoot)
	if err != nil {
		slog.Debug("Error reading fans", "err", err)
		return
	}
	if len(fans) > 0 {
		systemStats.Fans = fans
	}
}

// readHwmonFans walks the given hwmon root (typically /sys/class/hwmon) and
// returns a map of "<chip>_<label-or-fan-idx>" → RPM for every fan*_input
// file it finds. Entries reporting 0 RPM are skipped — that's how the kernel
// reports unpopulated fan headers.
func readHwmonFans(root string) (map[string]float64, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	fans := make(map[string]float64)
	for _, entry := range entries {
		chipDir := filepath.Join(root, entry.Name())
		chipName := readSysfsString(filepath.Join(chipDir, "name"))
		if chipName == "" {
			chipName = entry.Name()
		}
		inputs, _ := filepath.Glob(filepath.Join(chipDir, "fan*_input"))
		for _, inputPath := range inputs {
			base := strings.TrimSuffix(filepath.Base(inputPath), "_input")
			rpm, err := strconv.ParseFloat(readSysfsString(inputPath), 64)
			if err != nil || rpm <= 0 {
				continue
			}
			label := readSysfsString(filepath.Join(chipDir, base+"_label"))
			key := chipName + "_" + base
			if label != "" {
				key = chipName + "_" + label
			}
			fans[key] = rpm
		}
	}
	return fans, nil
}

// readSysfsString reads a single-line sysfs file and trims trailing newline.
// Returns "" on any read error.
func readSysfsString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
