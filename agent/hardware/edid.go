package hardware

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

var edidHeader = []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}

// parseEDID decodes the base EDID block (the first 128 bytes) of a monitor.
func parseEDID(b []byte) (hardware.Display, error) {
	var d hardware.Display
	if len(b) < 128 || !bytes.Equal(b[:8], edidHeader) {
		return d, errors.New("not an EDID block")
	}
	d.Manufacturer = pnpID(binary.BigEndian.Uint16(b[8:10]))
	d.ProductCode = fmt.Sprintf("%04x", binary.LittleEndian.Uint16(b[10:12]))
	if serial := binary.LittleEndian.Uint32(b[12:16]); serial != 0 {
		d.Serial = fmt.Sprintf("%d", serial)
	}
	if year := int(b[17]); year > 0 {
		d.Year = 1990 + year
	}
	d.WidthCM, d.HeightCM = int(b[21]), int(b[22])
	// Four 18-byte descriptors; display descriptors start with a zero pixel clock.
	for off := 54; off+18 <= 126; off += 18 {
		desc := b[off : off+18]
		if desc[0] != 0 || desc[1] != 0 {
			continue
		}
		switch desc[3] {
		case 0xFC:
			d.Name = descriptorText(desc[5:18])
		case 0xFF:
			if s := descriptorText(desc[5:18]); s != "" {
				d.Serial = s
			}
		}
	}
	return d, nil
}

// pnpID decodes the three-letter vendor ID packed as 5-bit letters.
func pnpID(v uint16) string {
	letters := []uint16{(v >> 10) & 0x1F, (v >> 5) & 0x1F, v & 0x1F}
	var sb strings.Builder
	for _, l := range letters {
		if l < 1 || l > 26 {
			return ""
		}
		sb.WriteByte(byte('A' + l - 1))
	}
	return sb.String()
}

func descriptorText(b []byte) string {
	if i := bytes.IndexByte(b, 0x0A); i >= 0 {
		b = b[:i]
	}
	return clean(string(b))
}
