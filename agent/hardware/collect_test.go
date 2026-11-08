package hardware

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/hardware"
	"github.com/stretchr/testify/assert"
)

func TestStripIdentifiers(t *testing.T) {
	inv := hardware.Inventory{
		System:   hardware.System{Product: "X470", Serial: "s", UUID: "u"},
		Board:    hardware.Board{Serial: "s"},
		Chassis:  hardware.Chassis{Serial: "s"},
		Memory:   []hardware.MemoryModule{{PartNumber: "p", Serial: "s"}},
		USB:      []hardware.USBDevice{{Name: "kbd", Serial: "s"}},
		Disks:    []hardware.Disk{{Model: "m", Serial: "s"}},
		NICs:     []hardware.NIC{{Name: "eth0", MAC: "m"}},
		Displays: []hardware.Display{{Name: "d", Serial: "s"}},
	}
	inv.StripIdentifiers()
	assert.Equal(t, hardware.System{Product: "X470"}, inv.System)
	assert.Empty(t, inv.Board.Serial+inv.Chassis.Serial+inv.Memory[0].Serial+inv.USB[0].Serial+
		inv.Disks[0].Serial+inv.NICs[0].MAC+inv.Displays[0].Serial)
	assert.Equal(t, "p", inv.Memory[0].PartNumber, "non-identifying fields are kept")
}

func TestClean(t *testing.T) {
	assert.Equal(t, "", clean("To Be Filled By O.E.M."))
	assert.Equal(t, "", clean("00000000"), "all-zero serial")
	assert.Equal(t, "", clean("ffffffff"), "all-ones serial")
	assert.Equal(t, "A0000000", clean("A0000000"))
	assert.Equal(t, "Gigabyte", clean("  Gigabyte\x00\n"))
	assert.Len(t, clean(string(make([]byte, 1000))), 0)
}

func TestPlaceholderUUID(t *testing.T) {
	assert.Equal(t, "", clean("12345678-1234-5678-90AB-CDDEEFAABBCC"), "Insyde default UUID")
}
