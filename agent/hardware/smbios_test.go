package hardware

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smbiosStruct builds one structure: a formatted area of the given length
// (header included) with fields set by the caller, followed by its strings.
func smbiosStruct(typ, length byte, set func(f []byte), strs ...string) []byte {
	f := make([]byte, length)
	f[0], f[1] = typ, length
	if set != nil {
		set(f)
	}
	out := f
	for _, s := range strs {
		out = append(out, append([]byte(s), 0)...)
	}
	if len(strs) == 0 {
		out = append(out, 0)
	}
	return append(out, 0)
}

func sampleTable() []byte {
	var t []byte
	t = append(t, smbiosStruct(0, 0x18, func(f []byte) { f[4], f[5], f[8] = 1, 2, 3 },
		"American Megatrends", "F66c", "08/18/2026")...)
	t = append(t, smbiosStruct(1, 0x1B, func(f []byte) {
		f[4], f[5], f[7] = 1, 2, 3
		copy(f[8:], []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	}, "Gigabyte", "X470", "SN123")...)
	t = append(t, smbiosStruct(2, 0x0F, func(f []byte) { f[4], f[5], f[7] = 1, 2, 3 },
		"Gigabyte", "X470 AORUS", "Default string")...)
	t = append(t, smbiosStruct(3, 0x15, func(f []byte) { f[4], f[5] = 1, 0x03 }, "NZXT")...)
	t = append(t, smbiosStruct(17, 0x28, func(f []byte) {
		binary.LittleEndian.PutUint16(f[0x0C:], 16384)
		f[0x0E], f[0x10], f[0x11], f[0x12] = 0x09, 1, 2, 0x1A
		binary.LittleEndian.PutUint16(f[0x15:], 3200)
		f[0x17], f[0x18], f[0x1A] = 3, 4, 5
		binary.LittleEndian.PutUint16(f[0x20:], 2800)
	}, "DIMM_A2", "BANK 1", "G.Skill", "0000AB12", "F4-3200C16-16GVK")...)
	t = append(t, smbiosStruct(17, 0x28, func(f []byte) { f[0x10] = 1 }, "DIMM_A1")...)
	return append(t, smbiosStruct(127, 4, nil)...)
}

func TestParseSMBIOS(t *testing.T) {
	d, err := parseSMBIOS(sampleTable())
	require.NoError(t, err)

	assert.Equal(t, "F66c", d.BIOS.Version)
	assert.Equal(t, "SN123", d.System.Serial)
	assert.Equal(t, "00112233-4455-6677-8899-aabbccddeeff", d.System.UUID)
	assert.Equal(t, "", d.Board.Serial, "placeholder serial is dropped")
	assert.Equal(t, "Desktop", d.Chassis.Type)
	require.Len(t, d.Memory, 2)
	m := d.Memory[0]
	assert.Equal(t, uint64(16384), m.SizeMB)
	assert.Equal(t, "DDR4", m.Type)
	assert.Equal(t, "DIMM", m.FormFactor)
	assert.Equal(t, 2800, m.ConfiguredMTs)
	assert.Equal(t, "F4-3200C16-16GVK", m.PartNumber)
	assert.Equal(t, uint64(0), d.Memory[1].SizeMB, "empty slot")
}

func TestParseSMBIOSMalformed(t *testing.T) {
	_, err := parseSMBIOS(nil)
	assert.Error(t, err)
	_, err = parseSMBIOS([]byte{17, 2, 0, 0}) // length below header size
	assert.Error(t, err)
	_, err = parseSMBIOS(make([]byte, maxSMBIOSTable+1))
	assert.Error(t, err)
	// A string index beyond the string set resolves to "", not a panic.
	d, err := parseSMBIOS(smbiosStruct(0, 0x18, func(f []byte) { f[4] = 9 }, "only"))
	require.NoError(t, err)
	assert.Equal(t, "", d.BIOS.Vendor)
}

func FuzzParseSMBIOS(f *testing.F) {
	f.Add(sampleTable())
	f.Add([]byte{})
	f.Add([]byte{17, 0x28, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := parseSMBIOS(b)
		if err != nil {
			return
		}
		assert.LessOrEqual(t, len(d.Memory), maxSMBIOSStructures)
	})
}
