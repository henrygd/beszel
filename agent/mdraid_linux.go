//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/smart"
)

// mdraidSysfsRoot is a test hook; production value is "/sys".
var mdraidSysfsRoot = "/sys"

type mdraidHealth struct {
	level          string
	arrayState     string
	degraded       uint64
	faultyDisks    uint64
	populatedDisks uint64
	raidDisks      uint64
	syncAction     string
	syncCompleted  string
	syncSpeed      string
	mismatchCnt    uint64
	capacity       uint64
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
		if !utils.FileExists(filepath.Join(mdDir, "array_state")) {
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
	if health.faultyDisks > 0 {
		attrs = append(attrs, &smart.SmartAttribute{Name: "FaultyDisks", RawValue: health.faultyDisks})
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
	arrayState, okState := utils.ReadStringFileOK(filepath.Join(mdDir, "array_state"))
	if !okState {
		return out, false
	}

	out.arrayState = arrayState
	out.level = utils.ReadStringFile(filepath.Join(mdDir, "level"))
	out.syncAction = utils.ReadStringFile(filepath.Join(mdDir, "sync_action"))
	out.syncCompleted = utils.ReadStringFile(filepath.Join(mdDir, "sync_completed"))
	out.syncSpeed = utils.ReadStringFile(filepath.Join(mdDir, "sync_speed"))

	if val, ok := utils.ReadUintFile(filepath.Join(mdDir, "raid_disks")); ok {
		out.raidDisks = val
	}
	if val, ok := utils.ReadUintFile(filepath.Join(mdDir, "degraded")); ok {
		out.degraded = val
	}
	out.faultyDisks, out.populatedDisks = countMdraidMemberStates(blockName, mdraidSysfsRoot)
	if val, ok := utils.ReadUintFile(filepath.Join(mdDir, "mismatch_cnt")); ok {
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
	// During rebuild/recovery, arrays are often temporarily degraded; report as
	// warning instead of hard failure while synchronization is in progress.
	syncAction := strings.ToLower(strings.TrimSpace(health.syncAction))
	switch syncAction {
	case "resync", "recover", "reshape":
		return "WARNING"
	}
	// Use actual faulty member count rather than the degraded counter, which
	// equals raid_disks minus active_disks. On QNAP systems raid_disks may be
	// set to a large value (e.g. 32) while only a few slots are ever used,
	// making degraded misleadingly large despite zero failed disks.
	if health.faultyDisks > 0 {
		return "FAILED"
	}
	if health.degraded > 0 {
		if isSparseSlotDegraded(health) {
			// A sysfs snapshot cannot distinguish reserved slots from a removed
			// member on sparse arrays, so report the ambiguity as a warning.
			return "WARNING"
		}
		return "FAILED"
	}
	if health.mismatchCnt > 0 {
		return "WARNING"
	}
	// "check" scans for consistency problems without repairing mismatches.
	// With no mismatches, keep it green while reporting progress attributes.
	switch syncAction {
	case "repair":
		return "WARNING"
	}
	switch state {
	case "clean", "active", "active-idle", "write-pending", "read-auto", "readonly":
		return "PASSED"
	}
	return "UNKNOWN"
}

// countMdraidMemberStates reads member device directories under
// block/<name>/md and returns how many are explicitly marked "faulty", plus
// how many are populated at all (regardless of state). populatedDisks lets
// callers distinguish RAID slots that were never used (QNAP reserves far
// more raid_disks than it ever populates) from members that went missing.
func countMdraidMemberStates(blockName, root string) (faultyDisks, populatedDisks uint64) {
	devDir := filepath.Join(root, "block", blockName, "md")
	entries, err := os.ReadDir(devDir)
	if err != nil {
		return 0, 0
	}
	for _, ent := range entries {
		if !strings.HasPrefix(ent.Name(), "dev-") {
			continue
		}
		populatedDisks++
		statePath := filepath.Join(devDir, ent.Name(), "state")
		state := utils.ReadStringFile(statePath)
		if strings.Contains(state, "faulty") {
			faultyDisks++
		}
	}
	return faultyDisks, populatedDisks
}

// isSparseSlotDegraded reports whether a non-zero "degraded" count may be
// explained by RAID slots that were never populated. QNAP configures system
// arrays with raid_disks set to a large fixed maximum (e.g. 32) far beyond the
// handful of slots it ever populates, so sparse slots outnumber populated ones.
func isSparseSlotDegraded(health mdraidHealth) bool {
	if health.populatedDisks == 0 || health.raidDisks <= health.populatedDisks {
		return false
	}
	sparseSlots := health.raidDisks - health.populatedDisks
	return sparseSlots > health.populatedDisks
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

	sizeStr, ok := utils.ReadStringFileOK(sizePath)
	if !ok {
		return 0, false
	}
	sectors, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil || sectors == 0 {
		return 0, false
	}

	logicalBlockSize := uint64(512)
	if lbsStr, ok := utils.ReadStringFileOK(lbsPath); ok {
		if parsed, err := strconv.ParseUint(lbsStr, 10, 64); err == nil && parsed > 0 {
			logicalBlockSize = parsed
		}
	}

	return sectors * logicalBlockSize, true
}
