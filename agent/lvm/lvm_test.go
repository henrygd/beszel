//go:build testing

package lvm

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readFixture returns a fresh copy of the superblock that tests may modify.
func readFixture(t *testing.T) []byte {
	t.Helper()
	return thinSuperblockFixture()
}

// reseal recomputes the checksum after a test mutates the superblock.
func reseal(b []byte) {
	sum := ^crc32.Checksum(b[4:superblockSize], castagnoli) ^ superblockCsumXor
	binary.LittleEndian.PutUint32(b, sum)
}

func TestThinSuperblockFixtureMatchesCapture(t *testing.T) {
	assert.Equal(t, uint32(capturedSuperblockCsum), binary.LittleEndian.Uint32(readFixture(t)))
}

func TestParseThinSuperblock(t *testing.T) {
	sb, err := ParseThinSuperblock(readFixture(t))
	require.NoError(t, err)

	assert.False(t, sb.NeedsCheck)
	assert.Equal(t, uint64(65536), sb.DataBlockSize)
	assert.Equal(t, uint64(362777935872), sb.DataSize)
	assert.Equal(t, uint64(214696*65536), sb.DataUsed)
	assert.InDelta(t, 3.88, float64(sb.DataUsed)/float64(sb.DataSize)*100, 0.005)
	assert.Equal(t, uint64(903168*4096), sb.MetadataSize)
	assert.Equal(t, uint64((1589+4096)*4096), sb.MetadataUsed)
	assert.InDelta(t, 0.63, float64(sb.MetadataUsed)/float64(sb.MetadataSize)*100, 0.005)
}

func TestParseThinSuperblockNeedsCheck(t *testing.T) {
	b := readFixture(t)
	binary.LittleEndian.PutUint32(b[offFlags:], needsCheckFlag)
	reseal(b)

	sb, err := ParseThinSuperblock(b)
	require.NoError(t, err)
	assert.True(t, sb.NeedsCheck)
}

func TestParseThinSuperblockSmallMetadataReserve(t *testing.T) {
	b := readFixture(t)
	// 1000 metadata blocks reserve 100, not the 4096 cap.
	binary.LittleEndian.PutUint64(b[offMetadataSpaceMap:], 1000)
	binary.LittleEndian.PutUint64(b[offMetadataSpaceMap+8:], 950)
	reseal(b)

	sb, err := ParseThinSuperblock(b)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000*4096), sb.MetadataSize)
	assert.Equal(t, sb.MetadataSize, sb.MetadataUsed, "reserve is clamped to capacity")
}

func TestParseThinSuperblockInvalid(t *testing.T) {
	t.Run("short read", func(t *testing.T) {
		_, err := ParseThinSuperblock(readFixture(t)[:512])
		assert.ErrorIs(t, err, ErrInvalidSuperblock)
	})

	t.Run("bad magic", func(t *testing.T) {
		b := readFixture(t)
		binary.LittleEndian.PutUint64(b[offMagic:], 1)
		reseal(b)
		_, err := ParseThinSuperblock(b)
		assert.ErrorIs(t, err, ErrInvalidSuperblock)
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		b := readFixture(t)
		b[offDataSpaceMap+8]++
		_, err := ParseThinSuperblock(b)
		assert.ErrorIs(t, err, ErrInvalidSuperblock)
	})

	t.Run("zeroed block", func(t *testing.T) {
		_, err := ParseThinSuperblock(make([]byte, superblockSize))
		assert.ErrorIs(t, err, ErrInvalidSuperblock)
	})
}
