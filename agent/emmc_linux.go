//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/smart"
)

// emmcSysfsRoot is a test hook; production value is "/sys".
var emmcSysfsRoot = "/sys"

type emmcHealth struct {
	model    string
	serial   string
	revision string
	capacity uint64
	preEOL   uint8
	lifeA    uint8
	lifeB    uint8
}

func scanEmmcDevices() []*DeviceInfo {
	blockDir := filepath.Join(emmcSysfsRoot, "class", "block")
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil
	}

	devices := make([]*DeviceInfo, 0, 2)
	for _, ent := range entries {
		name := ent.Name()
		if !isEmmcBlockName(name) {
			continue
		}

		deviceDir := filepath.Join(blockDir, name, "device")
		if !fileExists(filepath.Join(deviceDir, "pre_eol_info")) &&
			!fileExists(filepath.Join(deviceDir, "life_time")) &&
			!fileExists(filepath.Join(deviceDir, "device_life_time_est_typ_a")) &&
			!fileExists(filepath.Join(deviceDir, "device_life_time_est_typ_b")) {
			continue
		}

		devPath := filepath.Join("/dev", name)
		devices = append(devices, &DeviceInfo{
			Name:     devPath,
			Type:     "emmc",
			InfoName: devPath + " [eMMC]",
			Protocol: "MMC",
		})
	}

	return devices
}

func (sm *SmartManager) collectEmmcHealth(deviceInfo *DeviceInfo) (bool, error) {
	if deviceInfo == nil || deviceInfo.Name == "" {
		return false, nil
	}

	base := filepath.Base(deviceInfo.Name)
	if !isEmmcBlockName(base) && !strings.EqualFold(deviceInfo.Type, "emmc") && !strings.EqualFold(deviceInfo.Type, "mmc") {
		return false, nil
	}

	health, ok := readEmmcHealth(base)
	if !ok {
		return false, nil
	}

	// Normalize the device type to keep pruning logic stable across refreshes.
	deviceInfo.Type = "emmc"

	key := health.serial
	if key == "" {
		key = filepath.Join("/dev", base)
	}

	status := emmcSmartStatus(health.preEOL)

	attrs := []*smart.SmartAttribute{
		{
			Name:      "PreEOLInfo",
			RawValue:  uint64(health.preEOL),
			RawString: emmcPreEOLString(health.preEOL),
		},
		{
			Name:      "DeviceLifeTimeEstA",
			RawValue:  uint64(health.lifeA),
			RawString: emmcLifeTimeString(health.lifeA),
		},
		{
			Name:      "DeviceLifeTimeEstB",
			RawValue:  uint64(health.lifeB),
			RawString: emmcLifeTimeString(health.lifeB),
		},
	}

	sm.Lock()
	defer sm.Unlock()

	if _, exists := sm.SmartDataMap[key]; !exists {
		sm.SmartDataMap[key] = &smart.SmartData{}
	}

	data := sm.SmartDataMap[key]
	data.ModelName = health.model
	data.SerialNumber = health.serial
	data.FirmwareVersion = health.revision
	data.Capacity = health.capacity
	data.Temperature = 0
	data.SmartStatus = status
	data.DiskName = filepath.Join("/dev", base)
	data.DiskType = "emmc"
	data.Attributes = attrs
	sm.SmartDataMap[key] = data

	return true, nil
}

func readEmmcHealth(blockName string) (emmcHealth, bool) {
	var out emmcHealth

	if !isEmmcBlockName(blockName) {
		return out, false
	}

	deviceDir := filepath.Join(emmcSysfsRoot, "class", "block", blockName, "device")
	preEOL, okPre := readHexByteFile(filepath.Join(deviceDir, "pre_eol_info"))

	// Some kernels expose EXT_CSD lifetime via "life_time" (two bytes), others as
	// separate files. Support both.
	lifeA, lifeB, okLife := readLifeTime(deviceDir)

	if !okPre && !okLife {
		return out, false
	}

	out.preEOL = preEOL
	out.lifeA = lifeA
	out.lifeB = lifeB

	out.model = readStringFile(filepath.Join(deviceDir, "name"))
	out.serial = readStringFile(filepath.Join(deviceDir, "serial"))
	out.revision = readStringFile(filepath.Join(deviceDir, "prv"))

	if capBytes, ok := readBlockCapacityBytes(blockName); ok {
		out.capacity = capBytes
	}

	return out, true
}

func readLifeTime(deviceDir string) (uint8, uint8, bool) {
	if content, ok := readStringFileOK(filepath.Join(deviceDir, "life_time")); ok {
		a, b, ok := parseHexBytePair(content)
		return a, b, ok
	}

	a, okA := readHexByteFile(filepath.Join(deviceDir, "device_life_time_est_typ_a"))
	b, okB := readHexByteFile(filepath.Join(deviceDir, "device_life_time_est_typ_b"))
	if okA || okB {
		return a, b, true
	}
	return 0, 0, false
}

func readBlockCapacityBytes(blockName string) (uint64, bool) {
	sizePath := filepath.Join(emmcSysfsRoot, "class", "block", blockName, "size")
	lbsPath := filepath.Join(emmcSysfsRoot, "class", "block", blockName, "queue", "logical_block_size")

	sizeStr, ok := readStringFileOK(sizePath)
	if !ok {
		return 0, false
	}
	sectors, err := strconv.ParseUint(strings.TrimSpace(sizeStr), 10, 64)
	if err != nil || sectors == 0 {
		return 0, false
	}

	lbsStr, ok := readStringFileOK(lbsPath)
	logicalBlockSize := uint64(512)
	if ok {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(lbsStr), 10, 64); err == nil && parsed > 0 {
			logicalBlockSize = parsed
		}
	}

	return sectors * logicalBlockSize, true
}

func readHexByteFile(path string) (uint8, bool) {
	content, ok := readStringFileOK(path)
	if !ok {
		return 0, false
	}
	b, ok := parseHexOrDecByte(content)
	return b, ok
}

func readStringFile(path string) string {
	content, _ := readStringFileOK(path)
	return content
}

func readStringFileOK(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
