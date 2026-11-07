//go:build linux

package lvm

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"github.com/henrygd/beszel/agent/utils"
	"golang.org/x/sys/unix"
)

var (
	sysClassBlock = "/sys/class/block"
	devPath       = "/dev"
	openDevice    = openDirect
)

var errSuspended = errors.New("device suspended")

// deviceReader reads a block device; *os.File satisfies it in tests.
type deviceReader interface {
	io.ReaderAt
	io.Closer
}

// blockDevice is a block device as described by sysfs.
type blockDevice struct {
	dev    string // kernel name, e.g. dm-4 or nvme0n1p3
	name   string // dm name, e.g. pve-data-tpool; empty for non-dm devices
	uuid   string // dm uuid, e.g. LVM-<vg><lv>-tpool
	dir    string
	slaves []string
}

func (d blockDevice) isLV() bool { return strings.HasPrefix(d.uuid, "LVM-") }

func (d blockDevice) suspended() bool {
	return utils.ReadStringFile(filepath.Join(d.dir, "dm", "suspended")) == "1"
}

// stat returns cumulative bytes read and written.
func (d blockDevice) stat() (read, written uint64) {
	if stat := strings.Fields(utils.ReadStringFile(filepath.Join(d.dir, "stat"))); len(stat) >= 7 {
		return parseUint(stat[2]) * 512, parseUint(stat[6]) * 512
	}
	return 0, 0
}

// Pools returns volume groups with at least one active LV, and active thin
// pools. Devices that cannot be read are skipped and logged; a suspended
// device fails the whole inventory so the previous one is kept.
func Pools() ([]Pool, error) {
	devices, err := blockDevices()
	if err != nil || len(devices) == 0 {
		return nil, err
	}
	vgs, err := volumeGroups(devices)
	if err != nil {
		return nil, err
	}
	thin, err := thinPools(devices)
	if err != nil {
		return nil, err
	}
	pools := append(vgs, thin...)
	slices.SortFunc(pools, func(a, b Pool) int { return strings.Compare(a.Name, b.Name) })
	return pools, nil
}

// IOCounters returns cumulative I/O for every VG with an active LV and every
// active thin pool. Unlike Pools it reads only sysfs, never the devices, so
// it is cheap enough to call on every stats collection.
func IOCounters() ([]Counters, error) {
	devices, err := blockDevices()
	if err != nil || len(devices) == 0 {
		return nil, err
	}
	// A VG's PVs are the non-LV slaves of its LVs, whose dm uuids start with
	// the VG UUID. This needs no metadata, so PVs without a metadata area
	// count too. A thin pool's members are its _tmeta and _tdata sub-LVs.
	members := make(map[string]map[string]blockDevice)
	add := func(uuid string, d blockDevice) {
		if members[uuid] == nil {
			members[uuid] = make(map[string]blockDevice)
		}
		members[uuid][d.dev] = d
	}
	for _, d := range devices {
		vg, ok := lvVGUUID(d.uuid)
		if !ok {
			continue
		}
		pool, isPool := lvmUUID(d.uuid, "-tpool")
		for _, name := range d.slaves {
			slave, ok := devices[name]
			switch {
			case !ok:
			case !slave.isLV():
				add(vg, slave)
			case isPool && isThinSubLV(slave):
				add(pool, slave)
			}
		}
	}
	counters := make([]Counters, 0, len(members))
	for uuid, devs := range members {
		c := Counters{UUID: uuid}
		for _, d := range devs {
			read, written := d.stat()
			c.NRead += read
			c.NWrite += written
		}
		counters = append(counters, c)
	}
	slices.SortFunc(counters, func(a, b Counters) int { return strings.Compare(a.UUID, b.UUID) })
	return counters, nil
}

// lvVGUUID returns the VG UUID that prefixes an LVM dm uuid.
func lvVGUUID(dmUUID string) (string, bool) {
	id, ok := strings.CutPrefix(dmUUID, "LVM-")
	if !ok || len(id) < 64 {
		return "", false
	}
	return id[:32], true
}

func isThinSubLV(d blockDevice) bool {
	_, isMeta := lvmUUID(d.uuid, "-tmeta")
	_, isData := lvmUUID(d.uuid, "-tdata")
	return isMeta || isData
}

func blockDevices() (map[string]blockDevice, error) {
	entries, err := os.ReadDir(sysClassBlock)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	devices := make(map[string]blockDevice, len(entries))
	for _, entry := range entries {
		dir := filepath.Join(sysClassBlock, entry.Name())
		d := blockDevice{
			dev:  entry.Name(),
			name: utils.ReadStringFile(filepath.Join(dir, "dm", "name")),
			uuid: utils.ReadStringFile(filepath.Join(dir, "dm", "uuid")),
			dir:  dir,
		}
		if slaves, err := os.ReadDir(filepath.Join(dir, "slaves")); err == nil {
			for _, slave := range slaves {
				d.slaves = append(d.slaves, slave.Name())
			}
		}
		devices[d.dev] = d
	}
	return devices, nil
}

// volumeGroups reads the metadata of every PV underneath an active LV. PVs
// are not scanned beyond that, to avoid waking unrelated idle disks.
func volumeGroups(devices map[string]blockDevice) ([]Pool, error) {
	candidates := make(map[string]blockDevice)
	for _, d := range devices {
		if !d.isLV() {
			continue
		}
		for _, name := range d.slaves {
			if slave, ok := devices[name]; ok && !slave.isLV() {
				candidates[name] = slave
			}
		}
	}

	newest := make(map[string]VolumeGroup)
	for _, pv := range candidates {
		if pv.suspended() {
			return nil, fmt.Errorf("%s: %w", pv.dev, errSuspended)
		}
		vg, err := readVolumeGroup(filepath.Join(devPath, pv.dev))
		if errors.Is(err, errNoMetadata) {
			continue
		}
		if err != nil {
			slog.Debug("LVM physical volume unreadable", "device", pv.dev, "err", err)
			continue
		}
		if current, ok := newest[vg.UUID]; !ok || vg.Seqno > current.Seqno {
			newest[vg.UUID] = vg
		}
	}

	pools := make([]Pool, 0, len(newest))
	for uuid, vg := range newest {
		pool := Pool{UUID: uuid, Name: vg.Name, Raw: true, Health: "ONLINE", Size: vg.Size, Alloc: vg.Alloc, Devices: vg.Devices}
		for _, dev := range vg.Devices {
			if dev.State == "MISSING" {
				pool.Health = "DEGRADED"
			}
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

func readVolumeGroup(path string) (VolumeGroup, error) {
	f, err := openDevice(path)
	if err != nil {
		return VolumeGroup{}, err
	}
	defer f.Close()
	return readPVVolumeGroup(f)
}

func thinPools(devices map[string]blockDevice) ([]Pool, error) {
	var pools []Pool
	for _, d := range devices {
		// The -pool device is LVM's read-only wrapper with the same LV UUID;
		// only -tpool carries the thin-pool target.
		uuid, ok := lvmUUID(d.uuid, "-tpool")
		if !ok {
			continue
		}
		pool, err := readThinPool(d, uuid, devices)
		if errors.Is(err, errSuspended) {
			// Transient (e.g. lvextend): fail the inventory so the previous
			// one is kept instead of dropping the pool.
			return nil, fmt.Errorf("%s: %w", d.name, err)
		}
		if err != nil {
			slog.Debug("LVM thin pool unavailable", "pool", d.name, "err", err)
			continue
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

func readThinPool(d blockDevice, uuid string, devices map[string]blockDevice) (Pool, error) {
	pool := Pool{UUID: uuid, Name: decodeName(strings.TrimSuffix(d.name, "-tpool")), Health: "ONLINE"}
	var meta blockDevice
	for _, name := range d.slaves {
		slave, ok := devices[name]
		if !ok || !isThinSubLV(slave) {
			continue
		}
		if _, isMeta := lvmUUID(slave.uuid, "-tmeta"); isMeta {
			meta = slave
		}
		state := "ONLINE"
		if slave.suspended() {
			state = "SUSPENDED"
		}
		pool.Devices = append(pool.Devices, Device{Name: decodeName(slave.name), State: state})
	}
	if meta.dev == "" {
		return pool, errors.New("no metadata device")
	}
	// Reads from a suspended device block until it resumes (e.g. lvextend).
	if meta.suspended() {
		return pool, errSuspended
	}
	f, err := openDevice(filepath.Join(devPath, meta.dev))
	if err != nil {
		return pool, err
	}
	defer f.Close()
	b := make([]byte, superblockSize)
	if _, err := f.ReadAt(b, 0); err != nil {
		return pool, err
	}
	sb, err := ParseThinSuperblock(b)
	if err != nil {
		return pool, err
	}
	pool.Size, pool.Alloc = sb.DataSize, sb.DataUsed
	pool.MetadataSize, pool.MetadataUsed = sb.MetadataSize, sb.MetadataUsed
	switch {
	case sb.DataUsed >= sb.DataSize || sb.MetadataUsed >= sb.MetadataSize:
		pool.Health = "FULL"
	case sb.NeedsCheck:
		pool.Health = "DEGRADED"
	}
	return pool, nil
}

// lvmUUID returns the VG+LV UUID of an LVM dm uuid with the given suffix.
func lvmUUID(dmUUID, suffix string) (string, bool) {
	id, ok := strings.CutPrefix(dmUUID, "LVM-")
	if !ok {
		return "", false
	}
	id, ok = strings.CutSuffix(id, suffix)
	if !ok || len(id) != 64 {
		return "", false
	}
	return id, true
}

// decodeName turns a dm name into vg/lv. Device-mapper joins VG and LV with a
// single '-' and escapes '-' inside either name as "--".
func decodeName(dmName string) string {
	for i := 0; i < len(dmName); i++ {
		if dmName[i] != '-' {
			continue
		}
		if i+1 < len(dmName) && dmName[i+1] == '-' {
			i++
			continue
		}
		vg, lv := dmName[:i], dmName[i+1:]
		return strings.ReplaceAll(vg, "--", "-") + "/" + strings.ReplaceAll(lv, "--", "-")
	}
	return strings.ReplaceAll(dmName, "--", "-")
}

// directAlign covers the largest logical block size O_DIRECT may require.
const directAlign = 4096

// directFile reads with O_DIRECT. dm-thin writes metadata through dm-bufio
// and LVM writes PV metadata with O_DIRECT, so the block device's page cache
// can hold a stale copy.
type directFile struct{ fd int }

func openDirect(path string) (deviceReader, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECT|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return directFile{fd}, nil
}

// ReadAt widens the read to aligned offsets and an aligned buffer.
func (f directFile) ReadAt(p []byte, off int64) (int, error) {
	start := off &^ (directAlign - 1)
	end := (off + int64(len(p)) + directAlign - 1) &^ (directAlign - 1)
	raw := make([]byte, end-start+directAlign)
	shift := int(-uintptr(unsafe.Pointer(&raw[0])) & (directAlign - 1))
	buf := raw[shift : shift+int(end-start)]
	n, err := unix.Pread(f.fd, buf, start)
	if err != nil {
		return 0, err
	}
	skip := int(off - start)
	if n < skip+len(p) {
		return max(n-skip, 0), io.ErrUnexpectedEOF
	}
	return copy(p, buf[skip:]), nil
}

func (f directFile) Close() error { return unix.Close(f.fd) }

func parseUint(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
