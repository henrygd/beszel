package hardware

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

// SMBIOS limits. The table comes from firmware and is treated as untrusted input.
const (
	maxSMBIOSTable      = 1 << 20 // bytes; real tables are a few KiB
	maxSMBIOSStructures = 4096
)

// smbiosData is everything the inventory takes from the SMBIOS table.
type smbiosData struct {
	BIOS    hardware.BIOS
	System  hardware.System
	Board   hardware.Board
	Chassis hardware.Chassis
	Memory  []hardware.MemoryModule
}

// structure is one SMBIOS structure: its formatted area and its string set.
type structure struct {
	typ       byte
	formatted []byte
	strings   []string
}

// byteAt returns the formatted-area byte at off, or 0 when the structure is too short.
func (s structure) byteAt(off int) byte {
	if off < len(s.formatted) {
		return s.formatted[off]
	}
	return 0
}

func (s structure) word(off int) uint16 {
	if off+2 <= len(s.formatted) {
		return binary.LittleEndian.Uint16(s.formatted[off:])
	}
	return 0
}

func (s structure) dword(off int) uint32 {
	if off+4 <= len(s.formatted) {
		return binary.LittleEndian.Uint32(s.formatted[off:])
	}
	return 0
}

// str resolves the string reference stored at formatted offset off.
func (s structure) str(off int) string {
	idx := int(s.byteAt(off))
	if idx == 0 || idx > len(s.strings) {
		return ""
	}
	return clean(s.strings[idx-1])
}

// splitStructures walks the raw table. It stops at the end-of-table marker, at
// the first malformed structure, or at the structure cap; it never reads past b.
func splitStructures(b []byte) ([]structure, error) {
	if len(b) > maxSMBIOSTable {
		return nil, fmt.Errorf("smbios table too large: %d bytes", len(b))
	}
	var out []structure
	for off := 0; off+4 <= len(b) && len(out) < maxSMBIOSStructures; {
		typ, length := b[off], int(b[off+1])
		if length < 4 || off+length > len(b) {
			break
		}
		s := structure{typ: typ, formatted: b[off : off+length]}
		// The string set follows the formatted area and ends with a double NUL.
		end := bytes.Index(b[off+length:], []byte{0, 0})
		if end < 0 {
			break
		}
		if end > 0 {
			s.strings = splitStrings(b[off+length : off+length+end])
		}
		out = append(out, s)
		if typ == 127 {
			break
		}
		off += length + end + 2
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no smbios structures found")
	}
	return out, nil
}

func splitStrings(b []byte) []string {
	parts := bytes.Split(b, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, string(p))
	}
	return out
}

// parseSMBIOS decodes the structure types the inventory uses (0, 1, 2, 3, 17).
func parseSMBIOS(b []byte) (smbiosData, error) {
	var d smbiosData
	structures, err := splitStructures(b)
	if err != nil {
		return d, err
	}
	for _, s := range structures {
		switch s.typ {
		case 0:
			d.BIOS = hardware.BIOS{Vendor: s.str(0x04), Version: s.str(0x05), Date: s.str(0x08)}
		case 1:
			d.System = hardware.System{
				Vendor: s.str(0x04), Product: s.str(0x05), Version: s.str(0x06),
				Serial: s.str(0x07), UUID: smbiosUUID(s), SKU: s.str(0x19), Family: s.str(0x1A),
			}
		case 2:
			d.Board = hardware.Board{Vendor: s.str(0x04), Product: s.str(0x05), Version: s.str(0x06), Serial: s.str(0x07)}
		case 3:
			d.Chassis = hardware.Chassis{Vendor: s.str(0x04), Type: chassisType(s.byteAt(0x05) & 0x7f), Serial: s.str(0x07)}
		case 17:
			d.Memory = append(d.Memory, memoryModule(s))
		}
	}
	return d, nil
}

func memoryModule(s structure) hardware.MemoryModule {
	m := hardware.MemoryModule{
		Locator: s.str(0x10), Bank: s.str(0x11),
		FormFactor: lookup(formFactors, s.byteAt(0x0E)), Type: lookup(memoryTypes, s.byteAt(0x12)),
		SpeedMTs: int(s.word(0x15)), ConfiguredMTs: int(s.word(0x20)),
		Manufacturer: s.str(0x17), Serial: s.str(0x18), PartNumber: s.str(0x1A),
	}
	// Many modules leave the manufacturer string as "Unknown"; SMBIOS 3.2+ also
	// carries the JEDEC manufacturer code.
	if m.Manufacturer == "" {
		m.Manufacturer = jedecManufacturer(s.word(0x2A))
	}
	switch size := s.word(0x0C); {
	case size == 0 || size == 0xFFFF: // empty slot / unknown
	case size == 0x7FFF:
		m.SizeMB = uint64(s.dword(0x1C) & 0x7FFFFFFF)
	case size&0x8000 != 0:
		m.SizeMB = uint64(size&0x7FFF) / 1024 // KiB granularity
	default:
		m.SizeMB = uint64(size)
	}
	return m
}

// smbiosUUID formats the type 1 UUID (first three fields little-endian since 2.6).
func smbiosUUID(s structure) string {
	if len(s.formatted) < 0x18 {
		return ""
	}
	u := s.formatted[0x08:0x18]
	if bytes.Equal(u, make([]byte, 16)) || bytes.Equal(u, bytes.Repeat([]byte{0xFF}, 16)) {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%x-%x", binary.LittleEndian.Uint32(u[0:]),
		binary.LittleEndian.Uint16(u[4:]), binary.LittleEndian.Uint16(u[6:]), u[8:10], u[10:16])
}

func lookup(table map[byte]string, v byte) string {
	if name, ok := table[v]; ok {
		return name
	}
	if v <= 2 { // 0 = absent, 1 = Other, 2 = Unknown
		return ""
	}
	return "0x" + strconv.FormatUint(uint64(v), 16)
}

func chassisType(v byte) string { return lookup(chassisTypes, v) }
