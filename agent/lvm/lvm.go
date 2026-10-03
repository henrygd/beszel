// Package lvm reads LVM volume group and thin pool state from sysfs and
// on-disk metadata, without the LVM tools or root.
package lvm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// Thin-pool metadata superblock layout (drivers/md/dm-thin-metadata.c).
const (
	superblockSize      = 4096
	superblockMagic     = 27022010
	superblockCsumXor   = 160774
	needsCheckFlag      = 1 << 0
	offFlags            = 4
	offMagic            = 32
	offDataSpaceMap     = 64  // disk_sm_root: nr_blocks, nr_allocated
	offMetadataSpaceMap = 192 // disk_sm_root: nr_blocks, nr_allocated
	offDataBlockSize    = 336 // in 512-byte sectors
	metadataBlockSize   = 4096
	maxMetadataReserve  = 4096 // blocks (16 MiB), matching the kernel
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// ErrInvalidSuperblock is returned when a block is not a valid thin-pool
// metadata superblock.
var ErrInvalidSuperblock = errors.New("invalid thin-pool superblock")

// Pool is an LVM volume group or thin pool.
type Pool struct {
	UUID         string // VG UUID, or VG + LV UUID for a thin pool; stable across renames
	Name         string // vg, or vg/lv for a thin pool
	Raw          bool   // VG extent allocation rather than written data
	Health       string // ONLINE, DEGRADED (missing PV or metadata needs check) or FULL
	Size         uint64 // capacity in bytes
	Alloc        uint64 // allocated bytes
	MetadataSize uint64 // thin-pool metadata capacity in bytes
	MetadataUsed uint64 // thin-pool metadata bytes, including the kernel reserve
	NRead        uint64 // cumulative bytes read from the member devices
	NWrite       uint64 // cumulative bytes written to the member devices
	Devices      []Device
}

// Device is a PV of a volume group, or a hidden sub-LV (_tdata, _tmeta) of a
// thin pool.
type Device struct {
	Name  string // /dev/sda2 or vg/lv_tdata
	State string // ONLINE, MISSING or SUSPENDED
}

// ThinSuperblock is the space accounting of a thin pool as of its last
// metadata commit.
type ThinSuperblock struct {
	NeedsCheck    bool   // metadata flagged for thin_check
	DataSize      uint64 // pool data capacity in bytes
	DataUsed      uint64 // allocated data bytes
	MetadataSize  uint64 // metadata capacity in bytes
	MetadataUsed  uint64 // allocated metadata bytes, including the kernel reserve
	DataBlockSize uint64 // data block size in bytes
}

// ParseThinSuperblock decodes block 0 of a thin pool's metadata device.
func ParseThinSuperblock(b []byte) (ThinSuperblock, error) {
	if len(b) < superblockSize {
		return ThinSuperblock{}, fmt.Errorf("%w: short read of %d bytes", ErrInvalidSuperblock, len(b))
	}
	b = b[:superblockSize]
	le := binary.LittleEndian
	if magic := le.Uint64(b[offMagic:]); magic != superblockMagic {
		return ThinSuperblock{}, fmt.Errorf("%w: bad magic %d", ErrInvalidSuperblock, magic)
	}
	// The kernel seeds crc32c with ~0 and skips the final inversion.
	if sum := ^crc32.Checksum(b[4:], castagnoli) ^ superblockCsumXor; sum != le.Uint32(b) {
		return ThinSuperblock{}, fmt.Errorf("%w: checksum mismatch", ErrInvalidSuperblock)
	}

	dataBlockSize := uint64(le.Uint32(b[offDataBlockSize:])) * 512
	dataBlocks := le.Uint64(b[offDataSpaceMap:])
	dataAllocated := le.Uint64(b[offDataSpaceMap+8:])
	metaBlocks := le.Uint64(b[offMetadataSpaceMap:])
	metaAllocated := le.Uint64(b[offMetadataSpaceMap+8:])
	if dataBlockSize == 0 || dataBlocks == 0 || metaBlocks == 0 {
		return ThinSuperblock{}, fmt.Errorf("%w: empty space map", ErrInvalidSuperblock)
	}

	// dm-thin reports a slice of metadata as used so the pool can still commit
	// when nearly full; lvs Meta% includes it, so match that.
	metaUsed := min(metaAllocated+min(maxMetadataReserve, metaBlocks/10), metaBlocks)

	return ThinSuperblock{
		NeedsCheck:    le.Uint32(b[offFlags:])&needsCheckFlag != 0,
		DataSize:      dataBlocks * dataBlockSize,
		DataUsed:      min(dataAllocated, dataBlocks) * dataBlockSize,
		MetadataSize:  metaBlocks * metadataBlockSize,
		MetadataUsed:  metaUsed * metadataBlockSize,
		DataBlockSize: dataBlockSize,
	}, nil
}
