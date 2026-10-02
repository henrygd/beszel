//go:build testing

package lvm

import "encoding/binary"

// The fixtures below rebuild, field by field, blocks captured from a Proxmox
// VE host with the default pve VG. Tests compare their checksums with the
// values stored on that host, so the encoders are checked against real LVM
// and kernel output without committing binary files.

// Checksums read from the captured blocks.
const (
	capturedSuperblockCsum = 0x140e93d7 // pve-data_tmeta block 0
	capturedLabelCsum      = 0x1bac6e40 // /dev/nvme0n1p3 label sector
	capturedMdaHeaderCsum  = 0x68c0eeb4 // metadata area header, copy at 0x2d400
	capturedMetadataCsum   = 0x525921c0 // metadata text below, including its NUL
	capturedMetadataOffset = 0x2d400
)

// thinSuperblockFixture is pve/data's metadata superblock: lvs reported
// 362777935872 bytes, Data% 3.88, Meta% 0.63.
func thinSuperblockFixture() []byte {
	le := binary.LittleEndian
	b := make([]byte, superblockSize)
	le.PutUint64(b[offMagic:], superblockMagic)
	le.PutUint32(b[40:], 2)  // version
	le.PutUint64(b[48:], 18) // transaction id
	// Space map roots: nr_blocks, nr_allocated, bitmap root, ref count root.
	for i, v := range []uint64{5535552, 214696, 43484, 60} {
		le.PutUint64(b[offDataSpaceMap+8*i:], v)
	}
	for i, v := range []uint64{903168, 1589, 43486, 2} {
		le.PutUint64(b[offMetadataSpaceMap+8*i:], v)
	}
	le.PutUint64(b[320:], 43480) // data mapping root
	le.PutUint64(b[328:], 43483) // device details root
	le.PutUint32(b[offDataBlockSize:], 128)
	le.PutUint32(b[340:], 8)      // metadata block size in sectors
	le.PutUint64(b[344:], 903168) // metadata blocks
	reseal(b)
	return b
}

// fixtureMda is the offset of the PV's metadata area.
const fixtureMda = 0x1000

// pvFixture is the head of /dev/nvme0n1p3 (VG pve: 499029901312 bytes,
// 17184063488 free per vgs) with the newest metadata copy moved to the start
// of the metadata area. The rest of the 1020 KiB area is not materialized.
func pvFixture() []byte {
	le := binary.LittleEndian
	text := []byte(pvMetadataText + "\x00")
	b := make([]byte, fixtureMda+mdaHeaderSize+len(text))

	label := b[sectorSize : 2*sectorSize]
	copy(label, labelID)
	le.PutUint64(label[8:], 1)   // sector of the label
	le.PutUint32(label[20:], 32) // offset of the pv header
	copy(label[24:], labelType)
	pv := label[32:]
	copy(pv, "yxhnW60DEToVIxSEGyGtZpsKd0F9ELSf")
	le.PutUint64(pv[32:], 0x7430b01e00) // device size
	le.PutUint64(pv[40:], 0x100000)     // data area at 1 MiB, rest of device
	le.PutUint64(pv[72:], fixtureMda)   // metadata area
	le.PutUint64(pv[80:], 0xff000)
	le.PutUint32(pv[104:], 2) // pv header extension version
	le.PutUint32(pv[108:], 1) // PV_EXT_USED
	le.PutUint32(label[16:], lvmCrc(lvmInitialCrc, label[labelCrcOffset:]))

	header := b[fixtureMda:]
	copy(header[4:], mdaMagic)
	le.PutUint32(header[20:], mdaVersion)
	le.PutUint64(header[24:], fixtureMda)
	copy(b[fixtureMda+mdaHeaderSize:], text)
	return setMetadata(b, 0xff000, mdaHeaderSize, text)
}

// setMetadata points raw_locn[0] at text and reseals the metadata area header.
func setMetadata(b []byte, mdaSize, offset uint64, text []byte) []byte {
	le := binary.LittleEndian
	header := b[fixtureMda : fixtureMda+mdaHeaderSize]
	le.PutUint64(header[32:], mdaSize)
	le.PutUint64(header[40:], offset)
	le.PutUint64(header[48:], uint64(len(text)))
	le.PutUint32(header[56:], lvmCrc(lvmInitialCrc, text))
	le.PutUint32(header, lvmCrc(lvmInitialCrc, header[4:]))
	return b
}

// pvMetadataText is VG pve's metadata as LVM 2.03.31 wrote it, verbatim.
const pvMetadataText = `pve {
id = "Q34p3S-P0kH-uKfR-0v1a-RkFO-K6cV-BttXev"
seqno = 55
format = "lvm2"
status = ["RESIZEABLE", "READ", "WRITE"]
flags = []
extent_size = 8192
max_lv = 0
max_pv = 0
metadata_copies = 0

physical_volumes {

pv0 {
id = "yxhnW6-0DET-oVIx-SEGy-GtZp-sKd0-F9ELSf"
device = "/dev/nvme0n1p3"

status = ["ALLOCATABLE"]
flags = []
dev_size = 974673935
pe_start = 2048
pe_count = 118978
}
}

logical_volumes {

swap {
id = "21qd7q-oAcW-GynS-sJ0z-HW1s-Hk2E-WMsjcY"
status = ["READ", "WRITE", "VISIBLE"]
flags = []
creation_time = 1770636091
creation_host = "proxmox"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 2048

type = "striped"
stripe_count = 1

stripes = [
"pv0", 0
]
}
}

root {
id = "yhUCOw-USjT-eriw-croT-N6a7-WcU0-1cH1vB"
status = ["READ", "WRITE", "VISIBLE"]
flags = []
creation_time = 1770636091
creation_host = "proxmox"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 24576

type = "striped"
stripe_count = 1

stripes = [
"pv0", 2048
]
}
}

data {
id = "fFUT2V-moS6-HouZ-7XEs-Sgza-15P7-uQPpeE"
status = ["READ", "WRITE", "VISIBLE"]
flags = []
creation_time = 1770636091
creation_host = "proxmox"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 86493

type = "thin-pool"
metadata = "data_tmeta"
pool = "data_tdata"
transaction_id = 18
chunk_size = 128
discards = "passdown"
zero_new_blocks = 1
}
}

vm-100-disk-0 {
id = "O7aIJy-LY81-ejO8-8U2w-tUi0-Zvt8-bkFAy1"
status = ["READ", "WRITE", "VISIBLE"]
flags = ["NOAUTOACTIVATE"]
creation_time = 1790846770
creation_host = "pve"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 8192

type = "thin"
thin_pool = "data"
transaction_id = 12
device_id = 1
}
}

vm-100-disk-1 {
id = "5D73bK-Euit-tXcm-hn0j-yuic-8NXs-ptsY2Q"
status = ["READ", "WRITE", "VISIBLE"]
flags = ["NOAUTOACTIVATE"]
creation_time = 1790846801
creation_host = "pve"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 512

type = "thin"
thin_pool = "data"
transaction_id = 13
device_id = 2
}
}

vm-100-disk-2 {
id = "6NVsrr-e3v8-kAuA-VyN3-Movy-TaeI-Mm9mEb"
status = ["READ", "WRITE", "VISIBLE"]
flags = ["NOAUTOACTIVATE"]
creation_time = 1790846946
creation_host = "pve"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 512

type = "thin"
thin_pool = "data"
transaction_id = 14
device_id = 3
}
}

vm-101-disk-0 {
id = "qGGVqr-FzsN-WcXF-m56C-hEhj-kN6B-xR6z1S"
status = ["READ", "WRITE", "VISIBLE"]
flags = ["NOAUTOACTIVATE"]
creation_time = 1790847397
creation_host = "pve"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 8192

type = "thin"
thin_pool = "data"
transaction_id = 15
device_id = 4
}
}

vm-102-disk-0 {
id = "qFwf3s-oOHa-zlXX-Xrlb-evtn-5B0Z-3oSTCU"
status = ["READ", "WRITE", "VISIBLE"]
flags = ["NOAUTOACTIVATE"]
creation_time = 1790875445
creation_host = "pve"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 4096

type = "thin"
thin_pool = "data"
transaction_id = 16
device_id = 5
}
}

vm-103-disk-0 {
id = "ByQ2YM-lEV9-yvYz-bSnH-HcpJ-Gey8-kLZPrE"
status = ["READ", "WRITE", "VISIBLE"]
flags = ["NOAUTOACTIVATE"]
creation_time = 1790878770
creation_host = "pve"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 8192

type = "thin"
thin_pool = "data"
transaction_id = 17
device_id = 6
}
}

data_tmeta {
id = "0y6ARL-9e0l-exk2-VdyX-meGZ-gpHH-8jT1Gc"
status = ["READ", "WRITE"]
flags = []
creation_time = 1770636091
creation_host = "proxmox"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 882

type = "striped"
stripe_count = 1

stripes = [
"pv0", 113117
]
}
}

lvol0_pmspare {
id = "lyWHo2-r9es-KtaJ-JW0b-U1zA-QEbr-YT10yl"
status = ["READ", "WRITE"]
flags = []
creation_time = 1770636095
creation_host = "proxmox"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 882

type = "striped"
stripe_count = 1

stripes = [
"pv0", 113999
]
}
}

data_tdata {
id = "0a9i4D-c9nd-n4Pe-Aq5h-V25O-vwx2-K2JJ6a"
status = ["READ", "WRITE"]
flags = []
creation_time = 1770636100
creation_host = "proxmox"
segment_count = 1

segment1 {
start_extent = 0
extent_count = 86493

type = "striped"
stripe_count = 1

stripes = [
"pv0", 26624
]
}
}
}

}
# Generated by LVM2 version 2.03.31(2) (2025-02-27): Thu Oct  1 20:19:30 2026

contents = "Text Format Volume Group"
version = 1

description = "Write from /sbin/lvchange --setautoactivation n pve/vm-103-disk-0."

creation_host = "pve"	# Linux pve 7.0.14-20-pve #1 SMP PREEMPT_DYNAMIC PMX 7.0.14-20 (2026-09-24T10:43Z) x86_64
creation_time = 1790878770	# Thu Oct  1 20:19:30 2026

`
