package hardware

import (
	"fmt"
	"strings"
)

// SMBIOS 3.x enumerations (DSP0134), limited to values seen on real hardware.
var memoryTypes = map[byte]string{
	0x0F: "SDRAM", 0x12: "DDR", 0x13: "DDR2", 0x18: "DDR3", 0x1A: "DDR4",
	0x1B: "LPDDR", 0x1C: "LPDDR2", 0x1D: "LPDDR3", 0x1E: "LPDDR4",
	0x22: "DDR5", 0x23: "LPDDR5",
}

var formFactors = map[byte]string{
	0x03: "SIMM", 0x09: "DIMM", 0x0D: "SODIMM", 0x0F: "FB-DIMM", 0x10: "Die", 0x11: "CAMM",
}

var chassisTypes = map[byte]string{
	0x03: "Desktop", 0x04: "Low Profile Desktop", 0x06: "Mini Tower", 0x07: "Tower",
	0x08: "Portable", 0x09: "Laptop", 0x0A: "Notebook", 0x0D: "All in One",
	0x0E: "Sub Notebook", 0x11: "Main Server Chassis", 0x17: "Rack Mount Chassis",
	0x1E: "Tablet", 0x1F: "Convertible", 0x20: "Detachable", 0x23: "Mini PC", 0x24: "Stick PC",
}

// placeholders are vendor filler strings that carry no information.
var placeholders = map[string]bool{
	"to be filled by o.e.m.": true, "default string": true, "not specified": true,
	"not applicable": true, "none": true, "n/a": true, "system product name": true,
	"system manufacturer": true, "system version": true, "system serial number": true,
	"base board serial number": true, "chassis serial number": true, "unknown": true,
	"0123456789": true, "123456789": true, "0000000000": true, "o.e.m.": true,
	// Insyde firmware default UUID, left unchanged by some vendors (e.g. Synology).
	"12345678-1234-5678-90ab-cddeefaabbcc": true,
}

// clean trims whitespace and drops placeholder and non-printable values.
func clean(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s))
	if placeholders[strings.ToLower(s)] || strings.Trim(s, "0") == "" || strings.Trim(strings.ToUpper(s), "F") == "" {
		return ""
	}
	if len(s) > 256 {
		s = strings.ToValidUTF8(s[:256], "")
	}
	return s
}

// jedecManufacturers maps JEP106 codes of common memory module makers, keyed by
// bank (1-based) and manufacturer ID with the parity bit cleared.
var jedecManufacturers = map[[2]byte]string{
	{1, 0x2C}: "Micron", {1, 0x2D}: "SK hynix", {1, 0x4E}: "Samsung", {1, 0x7E}: "Elpida",
	{2, 0x18}: "Kingston", {3, 0x1E}: "Corsair", {5, 0x4B}: "ADATA", {5, 0x4D}: "G.Skill",
	{5, 0x6F}: "Team Group", {6, 0x1B}: "Crucial",
}

// jedecManufacturer decodes the SMBIOS module manufacturer ID: the low byte is
// the number of continuation codes (bank - 1), the high byte the ID, each with
// an odd-parity bit 7.
func jedecManufacturer(id uint16) string {
	if id == 0 {
		return ""
	}
	bank, code := byte(id&0x7F)+1, byte(id>>8)&0x7F
	if name, ok := jedecManufacturers[[2]byte{bank, code}]; ok {
		return name
	}
	return fmt.Sprintf("JEDEC bank %d 0x%02X", bank, code)
}
