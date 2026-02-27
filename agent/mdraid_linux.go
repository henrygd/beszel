//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/smart"
)

// mdraidSysfsRoot is a test hook; production value is "/sys".
var mdraidSysfsRoot = "/sys"

type mdraidHealth struct {
	level         string
	arrayState    string
	degraded      uint64
	raidDisks     uint64
	syncAction    string
	syncCompleted string
	syncSpeed     string
	mismatchCnt   uint64
	capacity      uint64
}

// scanMdraidDevices discovers Linux md arrays exposed in sysfs.
func scanMdraidDevices() []*DeviceInfo {
	blockDir := filepath.Join(mdraidSysfsRoot, "block")
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil
	}

	devices := make([]*DeviceInfo, 0, 2)
	for _, ent := range entries {
		name := ent.Name()
		if !isMdraidBlockName(name) {
			continue
		}
		mdDir := filepath.Join(blockDir, name, "md")
		if !fileExists(filepath.Join(mdDir, "array_state")) {
			continue
		}

		devPath := filepath.Join("/dev", name)
		devices = append(devices, &DeviceInfo{
			Name:     devPath,
			Type:     "mdraid",
			InfoName: devPath + " [mdraid]",
			Protocol: "MD",
		})
	}

	return devices
}

// collectMdraidHealth reads mdraid health and stores it in SmartDataMap.
func (sm *SmartManager) collectMdraidHealth(deviceInfo *DeviceInfo) (bool, error) {
	if deviceInfo == nil || deviceInfo.Name == "" {
		return false, nil
	}

	base := filepath.Base(deviceInfo.Name)
	if !isMdraidBlockName(base) && !strings.EqualFold(deviceInfo.Type, "mdraid") {
		return false, nil
	}

	health, ok := readMdraidHealth(base)
	if !ok {
		return false, nil
	}

	deviceInfo.Type = "mdraid"
	key := fmt.Sprintf("mdraid:%s", base)
	status := mdraidSmartStatus(health)

	attrs := make([]*smart.SmartAttribute, 0, 10)
	if health.arrayState != "" {
		attrs = append(attrs, &smart.SmartAttribute{Name: "ArrayState", RawString: health.arrayState})
	}
	if health.level != "" {
		attrs = append(attrs, &smart.SmartAttribute{Name: "RaidLevel", RawString: health.level})
	}
	if health.raidDisks > 0 {
		attrs = append(attrs, &smart.SmartAttribute{Name: "RaidDisks", RawValue: health.raidDisks})
	}
	if health.degraded > 0 {
		attrs = append(attrs, &smart.SmartAttribute{Name: "Degraded", RawValue: health.degraded})
	}
	if health.syncAction != "" {
		attrs = append(attrs, &smart.SmartAttribute{Name: "SyncAction", RawString: health.syncAction})
	}
	if health.syncCompleted != "" {
		attrs = append(attrs, &smart.SmartAttribute{Name: "SyncCompleted", RawString: health.syncCompleted})
	}
	if health.syncSpeed != "" {
		attrs = append(attrs, &smart.SmartAttribute{Name: "SyncSpeed", RawString: health.syncSpeed})
	}
	if health.mismatchCnt > 0 {
		attrs = append(attrs, &smart.SmartAttribute{Name: "MismatchCount", RawValue: health.mismatchCnt})
	}

	sm.Lock()
	defer sm.Unlock()

	if _, exists := sm.SmartDataMap[key]; !exists {
		sm.SmartDataMap[key] = &smart.SmartData{}
	}

	data := sm.SmartDataMap[key]
	data.ModelName = "Linux MD RAID"
	if health.level != "" {
		data.ModelName = "Linux MD RAID (" + health.level + ")"
	}
	data.Capacity = health.capacity
	data.SmartStatus = status
	data.DiskName = filepath.Join("/dev", base)
	data.DiskType = "mdraid"
	data.Attributes = attrs

	return true, nil
}

// readMdraidHealth reads md array health fields from sysfs.
func readMdraidHealth(blockName string) (mdraidHealth, bool) {
	var out mdraidHealth

	if !isMdraidBlockName(blockName) {
		return out, false
	}

	mdDir := filepath.Join(mdraidSysfsRoot, "block", blockName, "md")
	arrayState, okState := readStringFileOK(filepath.Join(mdDir, "array_state"))
	if !okState {
		return out, false
	}

	out.arrayState = arrayState
	out.level = readStringFile(filepath.Join(mdDir, "level"))
	out.syncAction = readStringFile(filepath.Join(mdDir, "sync_action"))
	out.syncCompleted = readStringFile(filepath.Join(mdDir, "sync_completed"))
	out.syncSpeed = readStringFile(filepath.Join(mdDir, "sync_speed"))

	if val, ok := readUintFile(filepath.Join(mdDir, "raid_disks")); ok {
		out.raidDisks = val
	}
	if val, ok := readUintFile(filepath.Join(mdDir, "degraded")); ok {
		out.degraded = val
	}
	if val, ok := readUintFile(filepath.Join(mdDir, "mismatch_cnt")); ok {
		out.mismatchCnt = val
	}

	if capBytes, ok := readMdraidBlockCapacityBytes(blockName, mdraidSysfsRoot); ok {
		out.capacity = capBytes
	}

	return out, true
}

// mdraidSmartStatus maps md state/sync signals to a SMART-like status.
func mdraidSmartStatus(health mdraidHealth) string {
	state := strings.ToLower(strings.TrimSpace(health.arrayState))
	switch state {
	case "inactive", "faulty", "broken", "stopped":
		return "FAILED"
	}
	if health.degraded > 0 {
		return "FAILED"
	}
	switch strings.ToLower(strings.TrimSpace(health.syncAction)) {
	case "resync", "recover", "reshape", "check", "repair":
		return "WARNING"
	}
	switch state {
	case "clean", "active", "active-idle", "write-pending", "read-auto", "readonly":
		return "PASSED"
	}
	return "UNKNOWN"
}

// isMdraidBlockName matches /dev/mdN-style block device names.
func isMdraidBlockName(name string) bool {
	if !strings.HasPrefix(name, "md") {
		return false
	}
	suffix := strings.TrimPrefix(name, "md")
	if suffix == "" {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// readMdraidBlockCapacityBytes converts block size metadata into bytes.
func readMdraidBlockCapacityBytes(blockName, root string) (uint64, bool) {
	sizePath := filepath.Join(root, "block", blockName, "size")
	lbsPath := filepath.Join(root, "block", blockName, "queue", "logical_block_size")

	sizeStr, ok := readStringFileOK(sizePath)
	if !ok {
		return 0, false
	}
	sectors, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil || sectors == 0 {
		return 0, false
	}

	logicalBlockSize := uint64(512)
	if lbsStr, ok := readStringFileOK(lbsPath); ok {
		if parsed, err := strconv.ParseUint(lbsStr, 10, 64); err == nil && parsed > 0 {
			logicalBlockSize = parsed
		}
	}

	return sectors * logicalBlockSize, true
}
