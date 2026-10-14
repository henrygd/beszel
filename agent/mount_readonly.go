package agent

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/disk"
)

// mountReadOnlyState holds optional host mountinfo path and one-shot warnings.
type mountReadOnlyState struct {
	path         string
	warnOnce     sync.Once
	seenWritable map[string]bool // fsStats key → observed rw at least once
}

func newMountReadOnlyState() *mountReadOnlyState {
	path, _ := utils.GetEnv("HOST_MOUNTINFO")
	if path == "" {
		if _, err := os.Stat("/host/proc/1/mountinfo"); err == nil {
			path = "/host/proc/1/mountinfo"
		}
	}
	return &mountReadOnlyState{
		path:         path,
		seenWritable: make(map[string]bool),
	}
}

// isReadOnlyOpts reports whether mount option list indicates a read-only mount.
// The last ro/rw token wins when both appear.
func isReadOnlyOpts(opts []string) bool {
	ro := false
	for _, opt := range opts {
		switch strings.ToLower(strings.TrimSpace(opt)) {
		case "ro":
			ro = true
		case "rw":
			ro = false
		}
	}
	return ro
}

// parseMountinfoReadOnly maps mountpoint and device basenames to read-only state.
func parseMountinfoReadOnly(content string) (byMountpoint map[string]bool, byDevice map[string]bool) {
	byMountpoint = make(map[string]bool)
	byDevice = make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// mountinfo: ... mountpoint opts [optional fields] - fstype source superOpts
		sep := -1
		for i, f := range fields {
			if f == "-" {
				sep = i
				break
			}
		}
		if sep < 6 || sep+2 >= len(fields) {
			continue
		}
		mountpoint := fields[4]
		opts := strings.Split(fields[5], ",")
		ro := isReadOnlyOpts(opts)
		byMountpoint[mountpoint] = ro

		source := fields[sep+2]
		base := filepath.Base(source)
		if base != "" && base != "/" && base != "." {
			// Prefer the root mountpoint when a device appears more than once.
			if _, exists := byDevice[base]; !exists || mountpoint == "/" {
				byDevice[base] = ro
			}
		}
	}
	return byMountpoint, byDevice
}

func loadHostMountReadOnly(path string) (byMountpoint map[string]bool, byDevice map[string]bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	byMountpoint, byDevice = parseMountinfoReadOnly(string(data))
	return byMountpoint, byDevice, nil
}

func loadLocalPartitionReadOnly() (byMountpoint map[string]bool, byDevice map[string]bool, err error) {
	partitions, err := disk.PartitionsWithContext(context.Background(), true)
	if err != nil {
		return nil, nil, err
	}
	byMountpoint = make(map[string]bool, len(partitions))
	byDevice = make(map[string]bool, len(partitions))
	for _, p := range partitions {
		ro := isReadOnlyOpts(p.Opts)
		if p.Mountpoint != "" {
			byMountpoint[p.Mountpoint] = ro
		}
		base := filepath.Base(p.Device)
		if base != "" && base != "/" && base != "." {
			if p.Mountpoint == "/" || p.Mountpoint == "" {
				byDevice[base] = ro
			} else if _, exists := byDevice[base]; !exists {
				byDevice[base] = ro
			}
		}
	}
	return byMountpoint, byDevice, nil
}

func lookupReadOnly(byMountpoint, byDevice map[string]bool, mountpoint, deviceKey string) (ro bool, ok bool) {
	if byMountpoint != nil {
		if v, found := byMountpoint[mountpoint]; found {
			return v, true
		}
	}
	if byDevice != nil && deviceKey != "" {
		base := filepath.Base(deviceKey)
		if v, found := byDevice[base]; found {
			return v, true
		}
	}
	return false, false
}

// updateDiskReadOnly sets ReadOnly on monitored filesystems using host mountinfo
// when available, otherwise the local partition table (binary / host network ns).
func (a *Agent) updateDiskReadOnly() {
	if a.mountRO == nil {
		return
	}
	var (
		byMountpoint map[string]bool
		byDevice     map[string]bool
		err          error
	)
	if a.mountRO.path != "" {
		byMountpoint, byDevice, err = loadHostMountReadOnly(a.mountRO.path)
		if err != nil {
			a.mountRO.warnOnce.Do(func() {
				slog.Warn("HOST_MOUNTINFO unreadable; filesystem read-only detection disabled", "path", a.mountRO.path, "err", err)
			})
			return
		}
	} else {
		byMountpoint, byDevice, err = loadLocalPartitionReadOnly()
		if err != nil {
			a.mountRO.warnOnce.Do(func() {
				slog.Warn("Cannot list partitions for read-only detection", "err", err)
			})
			return
		}
	}

	for key, stats := range a.fsStats {
		if stats == nil {
			continue
		}
		deviceKey := key
		ro, found := lookupReadOnly(byMountpoint, byDevice, stats.Mountpoint, deviceKey)
		if !found {
			stats.ReadOnly = false
			continue
		}
		if !ro {
			a.mountRO.seenWritable[key] = true
			stats.ReadOnly = false
			continue
		}
		// Currently read-only: always report for root; for extras only after we've
		// seen them writable (avoids permanent intentional RO and cold-start noise).
		if stats.Root || a.mountRO.seenWritable[key] {
			stats.ReadOnly = true
		} else {
			stats.ReadOnly = false
		}
	}
}

// applyReadOnlyInfo copies RO flags into Info / root Stats for the hub and UI.
func (a *Agent) applyReadOnlyInfo(data *system.CombinedData) {
	data.Info.ExtraFsReadOnly = make(map[string]bool)
	var names []string
	for name, stats := range a.fsStats {
		if stats == nil || !stats.ReadOnly {
			continue
		}
		if stats.Root {
			data.Info.RootReadOnly = true
			label := stats.Name
			if label == "" {
				label = "root"
			}
			names = append(names, label)
			continue
		}
		key := name
		if stats.Name != "" {
			key = stats.Name
		}
		data.Info.ExtraFsReadOnly[key] = true
		names = append(names, key)
	}
	if len(data.Info.ExtraFsReadOnly) == 0 {
		data.Info.ExtraFsReadOnly = nil
	}
	if len(names) > 0 {
		data.Info.ReadOnlyFilesystems = names
	}
}
