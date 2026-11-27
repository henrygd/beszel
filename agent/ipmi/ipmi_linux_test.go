//go:build testing && linux && (amd64 || arm64 || arm || 386 || riscv64 || loong64 || s390x)

package ipmi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDevTransportRefusesUnlistedCommands(t *testing.T) {
	// The check runs before the device is touched, so any file works.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	d := &devTransport{f: f}

	forbidden := [][2]uint8{
		{0x00, 0x02}, // Chassis Control (power off, reset)
		{0x06, 0x45}, // Set User Name
		{0x06, 0x47}, // Set User Password
		{0x0a, 0x24}, // Add SDR
		{0x04, 0x2e}, // Set Sensor Type
		{0x30, 0x70}, // OEM (often fan control)
	}
	for _, c := range forbidden {
		_, err := d.send(c[0], c[1], 0, nil)
		assert.ErrorIs(t, err, errCommandNotAllowed, "netfn 0x%02x cmd 0x%02x", c[0], c[1])
	}
}
