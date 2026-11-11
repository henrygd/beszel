//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/henrygd/beszel/agent/utils"
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

// scanEmmcDevices returns the /dev paths of eMMC devices that expose health
// fields in sysfs.
func scanEmmcDevices() []string {
	blockDir := filepath.Join(emmcSysfsRoot, "class", "block")
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil
	}

	var devices []string
	for _, ent := range entries {
		name := ent.Name()
		if !isEmmcBlockName(name) {
			continue
		}
		if !hasEmmcHealthFiles(filepath.Join(blockDir, name, "device")) {
			continue
		}
		devices = append(devices, filepath.Join("/dev", name))
	}
	return devices
}

// readEmmcSmartData reads eMMC wear / EOL indicators from sysfs. It returns the
// data map key (serial, or the device path when there is none).
func readEmmcSmartData(path string) (string, *smart.SmartData, bool) {
	base := filepath.Base(path)
	health, ok := readEmmcHealth(base)
	if !ok {
		return "", nil, false
	}

	devPath := filepath.Join("/dev", base)
	key := health.serial
	if key == "" {
		key = devPath
	}

	return key, &smart.SmartData{
		ModelName:       health.model,
		SerialNumber:    health.serial,
		FirmwareVersion: health.revision,
		Capacity:        health.capacity,
		SmartStatus:     emmcSmartStatus(health.preEOL),
		DiskName:        devPath,
		DiskType:        "emmc",
		Attributes: []*smart.SmartAttribute{
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
		},
	}, true
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

	out.model = utils.ReadStringFile(filepath.Join(deviceDir, "name"))
	out.serial = utils.ReadStringFile(filepath.Join(deviceDir, "serial"))
	out.revision = utils.ReadStringFile(filepath.Join(deviceDir, "prv"))

	if capBytes, ok := readBlockCapacityBytes(blockName); ok {
		out.capacity = capBytes
	}

	return out, true
}

func readLifeTime(deviceDir string) (uint8, uint8, bool) {
	if content, ok := utils.ReadStringFileOK(filepath.Join(deviceDir, "life_time")); ok {
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

	sizeStr, ok := utils.ReadStringFileOK(sizePath)
	if !ok {
		return 0, false
	}
	sectors, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil || sectors == 0 {
		return 0, false
	}

	lbsStr, ok := utils.ReadStringFileOK(lbsPath)
	logicalBlockSize := uint64(512)
	if ok {
		if parsed, err := strconv.ParseUint(lbsStr, 10, 64); err == nil && parsed > 0 {
			logicalBlockSize = parsed
		}
	}

	return sectors * logicalBlockSize, true
}

func readHexByteFile(path string) (uint8, bool) {
	content, ok := utils.ReadStringFileOK(path)
	if !ok {
		return 0, false
	}
	b, ok := parseHexOrDecByte(content)
	return b, ok
}

func hasEmmcHealthFiles(deviceDir string) bool {
	entries, err := os.ReadDir(deviceDir)
	if err != nil {
		return false
	}
	for _, ent := range entries {
		switch ent.Name() {
		case "pre_eol_info", "life_time", "device_life_time_est_typ_a", "device_life_time_est_typ_b":
			return true
		}
	}
	return false
}
