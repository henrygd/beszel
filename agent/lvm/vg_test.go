//go:build testing

package lvm

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readPVFixture returns a fresh copy of the PV head that tests may modify.
func readPVFixture(t *testing.T) []byte {
	t.Helper()
	return pvFixture()
}

func TestPVFixtureMatchesCapture(t *testing.T) {
	le := binary.LittleEndian
	b := readPVFixture(t)
	assert.Equal(t, uint32(capturedLabelCsum), le.Uint32(b[sectorSize+16:]))
	text := []byte(pvMetadataText + "\x00")
	assert.Equal(t, uint32(capturedMetadataCsum), lvmCrc(lvmInitialCrc, text))
	// The header as captured pointed at the copy's original position.
	b = setMetadata(b, 0xff000, capturedMetadataOffset, text)
	assert.Equal(t, uint32(capturedMdaHeaderCsum), le.Uint32(b[fixtureMda:]))
}

func TestReadPVMetadataFixture(t *testing.T) {
	text, err := readPVMetadata(bytes.NewReader(readPVFixture(t)))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(text, "pve {"))

	vg, err := ParseVGMetadata(text)
	require.NoError(t, err)
	assert.Equal(t, VolumeGroup{
		UUID:    "Q34p3SP0kHuKfR0v1aRkFOK6cVBttXev",
		Name:    "pve",
		Seqno:   55,
		Size:    499029901312,
		Alloc:   499029901312 - 17184063488,
		Devices: []Device{{Name: "/dev/nvme0n1p3", State: "ONLINE"}},
	}, vg)
}

func TestReadPVMetadataWrapsAround(t *testing.T) {
	b := readPVFixture(t)
	text := []byte(strings.Repeat("x", 300) + strings.Repeat("y", 200))
	// A 1024-byte area holds 512 bytes of text space after its header; a copy
	// starting at 812 wraps after 212 bytes back to offset 512.
	b = setMetadata(b[:fixtureMda+1024], 1024, 812, text)
	copy(b[fixtureMda+812:], text[:212])
	copy(b[fixtureMda+mdaHeaderSize:], text[212:])

	got, err := readPVMetadata(bytes.NewReader(b))
	require.NoError(t, err)
	assert.Equal(t, string(text), got)
}

func TestReadPVMetadataInvalid(t *testing.T) {
	t.Run("no label", func(t *testing.T) {
		_, err := readPVMetadata(bytes.NewReader(make([]byte, 8192)))
		assert.ErrorIs(t, err, ErrInvalidPV)
	})

	t.Run("label checksum", func(t *testing.T) {
		b := readPVFixture(t)
		b[sectorSize+40]++ // inside the PV UUID
		_, err := readPVMetadata(bytes.NewReader(b))
		assert.ErrorIs(t, err, ErrInvalidPV)
	})

	t.Run("metadata area checksum", func(t *testing.T) {
		b := readPVFixture(t)
		b[fixtureMda+100]++
		_, err := readPVMetadata(bytes.NewReader(b))
		assert.ErrorIs(t, err, ErrInvalidPV)
	})

	t.Run("text checksum", func(t *testing.T) {
		b := readPVFixture(t)
		b[fixtureMda+mdaHeaderSize+10]++
		_, err := readPVMetadata(bytes.NewReader(b))
		assert.ErrorIs(t, err, ErrInvalidPV)
	})

	t.Run("location out of range", func(t *testing.T) {
		b := readPVFixture(t)
		b = setMetadata(b, 1024, 2048, []byte("pve {}"))
		_, err := readPVMetadata(bytes.NewReader(b))
		assert.ErrorIs(t, err, ErrInvalidPV)
	})

	t.Run("empty metadata area", func(t *testing.T) {
		b := setMetadata(readPVFixture(t), 0xff000, mdaHeaderSize, nil)
		_, err := readPVMetadata(bytes.NewReader(b))
		assert.ErrorIs(t, err, errNoMetadata)
	})
}

func TestParseVGMetadata(t *testing.T) {
	// Two PVs (one missing), a RAID1 LV whose images are counted through their
	// striped sub-LVs, a two-way stripe and a thin LV that allocates nothing.
	text := `# comment
data-vg {
	id = "AAAAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAAAA"
	seqno = 7
	extent_size = 8192 # 4 MiB
	physical_volumes {
		pv0 {
			device = "/dev/sda"
			status = ["ALLOCATABLE"]
			pe_count = 100
		}
		pv1 {
			status = ["ALLOCATABLE"]
			flags = ["MISSING"]
			pe_count = 100
		}
	}
	logical_volumes {
		mirror { segment1 { type = "raid1" extent_count = 10 raids = ["mirror_rmeta_0", "mirror_rimage_0"] } }
		mirror_rimage_0 { segment1 { type = "striped" extent_count = 10 stripes = ["pv0", 0] } }
		mirror_rimage_1 { segment1 { type = "striped" extent_count = 10 stripes = ["pv1", 0] } }
		wide {
			segment_count = 2
			segment1 { type = "striped" extent_count = 20 stripe_count = 2 stripes = ["pv0", 10, "pv1", 10] }
			segment2 { type = "striped" extent_count = 4 stripes = ["pv0", 30] }
		}
		thin { segment1 { type = "thin" extent_count = 1000 thin_pool = "pool" } }
		quoted { description = "a \"quoted\" } value" segment1 { type = "striped" extent_count = 1 stripes = ["pv0", 40] } }
	}
}
contents = "Text Format Volume Group"
version = 1
`
	vg, err := ParseVGMetadata(text)
	require.NoError(t, err)
	const extent = 4 << 20
	assert.Equal(t, "data-vg", vg.Name)
	assert.Equal(t, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", vg.UUID)
	assert.Equal(t, int64(7), vg.Seqno)
	assert.Equal(t, uint64(200*extent), vg.Size)
	assert.Equal(t, uint64((10+10+20+4+1)*extent), vg.Alloc)
	assert.Equal(t, []Device{{Name: "/dev/sda", State: "ONLINE"}, {Name: "pv1", State: "MISSING"}}, vg.Devices)
}

func TestParseVGMetadataInvalid(t *testing.T) {
	for name, text := range map[string]string{
		"empty":               "",
		"no volume group":     `contents = "Text Format Volume Group"`,
		"unterminated":        `vg { id = "AAAAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAAAA" extent_size = 8192`,
		"unterminated string": `vg { id = "AAAA`,
		"stray brace":         `}`,
		"bad uuid":            `vg { id = "short" extent_size = 8192 }`,
		"missing value":       `vg { id = }`,
	} {
		_, err := ParseVGMetadata(text)
		assert.Error(t, err, name)
	}
}
