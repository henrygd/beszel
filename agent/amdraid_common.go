package agent

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/smart"
)

// AMD RAIDXpert2 member drives are read through CrystalDiskInfo's AMD_RC2 DLL on
// Windows. The parsers live here so they can be tested on every platform.

const amdRaidPrefix = "amdraid"

// amdRc2Identify mirrors AMD_RC2_IDENTIFY from CrystalDiskInfo (256 bytes).
type amdRc2Identify struct {
	StructSize    uint32
	StructVersion uint32
	DiskNum       uint32
	PhysicalDrive int32
	DriveSize64   uint64
	DriveSize     uint32
	IsSSD         uint8
	IsNVMe        uint8
	_             [2]uint8
	Model         [41]byte
	Serial        [21]byte
	Firmware      [9]byte
	Speed         [60]byte
	_             [93]byte
}

// amdRaidIndex returns the DLL disk index for names like "amdraid0".
func amdRaidIndex(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, amdRaidPrefix)
	if !ok {
		return 0, false
	}
	idx, err := strconv.Atoi(rest)
	return idx, err == nil && idx >= 0
}

func amdRaidDeviceName(idx int) string {
	return fmt.Sprintf("%s%d", amdRaidPrefix, idx)
}

func cString(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

// parseAmdRc2Nvme parses a 512-byte NVMe SMART / Health Information log page.
func parseAmdRc2Nvme(buf []byte) (temp uint8, passed bool, attrs []*smart.SmartAttribute) {
	if len(buf) < 200 {
		return 0, false, nil
	}
	u64 := func(off int) uint64 { return binary.LittleEndian.Uint64(buf[off:]) } // low half of 128-bit counters
	kelvin := binary.LittleEndian.Uint16(buf[1:])
	if kelvin > 273 {
		temp = uint8(min(kelvin-273, 255))
	}
	critical := buf[0]
	attrs = []*smart.SmartAttribute{
		{Name: "CriticalWarning", RawValue: uint64(critical)},
		{Name: "Temperature", RawValue: uint64(temp)},
		{Name: "AvailableSpare", RawValue: uint64(buf[3])},
		{Name: "AvailableSpareThreshold", RawValue: uint64(buf[4])},
		{Name: "PercentageUsed", RawValue: uint64(buf[5])},
		{Name: "DataUnitsRead", RawValue: u64(32)},
		{Name: "DataUnitsWritten", RawValue: u64(48)},
		{Name: "HostReads", RawValue: u64(64)},
		{Name: "HostWrites", RawValue: u64(80)},
		{Name: "ControllerBusyTime", RawValue: u64(96)},
		{Name: "PowerCycles", RawValue: u64(112)},
		{Name: "PowerOnHours", RawValue: u64(128)},
		{Name: "UnsafeShutdowns", RawValue: u64(144)},
		{Name: "MediaErrors", RawValue: u64(160)},
		{Name: "NumErrLogEntries", RawValue: u64(176)},
		{Name: "WarningTempTime", RawValue: uint64(binary.LittleEndian.Uint32(buf[192:]))},
		{Name: "CriticalCompTime", RawValue: uint64(binary.LittleEndian.Uint32(buf[196:]))},
	}
	return temp, critical == 0, attrs
}

var ataAttributeNames = map[uint8]string{
	1:   "Raw_Read_Error_Rate",
	5:   "Reallocated_Sector_Ct",
	9:   "Power_On_Hours",
	12:  "Power_Cycle_Count",
	177: "Wear_Leveling_Count",
	187: "Reported_Uncorrect",
	188: "Command_Timeout",
	190: "Airflow_Temperature_Cel",
	194: "Temperature_Celsius",
	196: "Reallocated_Event_Count",
	197: "Current_Pending_Sector",
	198: "Offline_Uncorrectable",
	199: "UDMA_CRC_Error_Count",
	231: "SSD_Life_Left",
	233: "Media_Wearout_Indicator",
	241: "Total_LBAs_Written",
	242: "Total_LBAs_Read",
}

// parseAmdRc2Ata parses raw ATA SMART READ DATA and READ THRESHOLDS sectors:
// 30 entries of 12 bytes starting at offset 2.
func parseAmdRc2Ata(data, thresholds []byte) (temp uint8, passed bool, attrs []*smart.SmartAttribute) {
	const entries, size, start = 30, 12, 2
	if len(data) < start+entries*size {
		return 0, false, nil
	}
	thr := make(map[uint8]uint8, entries)
	if len(thresholds) >= start+entries*size {
		for i := range entries {
			off := start + i*size
			if id := thresholds[off]; id != 0 {
				thr[id] = thresholds[off+1]
			}
		}
	}

	passed = true
	var temp190 uint8
	for i := range entries {
		e := data[start+i*size : start+(i+1)*size]
		id := e[0]
		if id == 0 {
			continue
		}
		var raw uint64
		for b := 5; b >= 0; b-- {
			raw = raw<<8 | uint64(e[5+b])
		}
		name, ok := ataAttributeNames[id]
		if !ok {
			name = fmt.Sprintf("Attribute_%d", id)
		}
		attr := &smart.SmartAttribute{
			ID:        uint16(id),
			Name:      name,
			Value:     uint16(e[3]),
			Worst:     uint16(e[4]),
			Threshold: uint16(thr[id]),
			RawValue:  raw,
		}
		if thr[id] > 0 && e[3] <= thr[id] {
			attr.WhenFailed = "now"
			passed = false
		}
		switch id {
		case 194:
			temp = uint8(raw)
		case 190:
			temp190 = uint8(raw)
		}
		attrs = append(attrs, attr)
	}
	if temp == 0 {
		temp = temp190
	}
	return temp, passed && len(attrs) > 0, attrs
}
