package hardware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleEDID() []byte {
	b := make([]byte, 128)
	copy(b, edidHeader)
	b[8], b[9] = 0x04, 0x72   // "ACR"
	b[10], b[11] = 0x23, 0x06 // product 0x0623
	b[17] = 29                // 2019
	b[21], b[22] = 60, 34
	name := b[72:90]
	name[3] = 0xFC
	copy(name[5:], "KG271U\n     ")
	serial := b[90:108]
	serial[3] = 0xFF
	copy(serial[5:], "T6XEE0018521\n")
	return b
}

func TestParseEDID(t *testing.T) {
	d, err := parseEDID(sampleEDID())
	require.NoError(t, err)
	assert.Equal(t, "ACR", d.Manufacturer)
	assert.Equal(t, "0623", d.ProductCode)
	assert.Equal(t, "KG271U", d.Name)
	assert.Equal(t, "T6XEE0018521", d.Serial)
	assert.Equal(t, 2019, d.Year)
	assert.Equal(t, 60, d.WidthCM)

	_, err = parseEDID(make([]byte, 127))
	assert.Error(t, err)
	_, err = parseEDID(make([]byte, 128)) // bad header
	assert.Error(t, err)
}

func FuzzParseEDID(f *testing.F) {
	f.Add(sampleEDID())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = parseEDID(b)
	})
}
