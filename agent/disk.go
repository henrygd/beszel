package agent

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/disk"
)

// rootFsKey is the fsStats key of the root filesystem. Root is reported in the
// system stats rather than ExtraFs, so this key never reaches the hub and
// cannot collide with the name of an extra filesystem.
const rootFsKey = "\x00root"

// mountInfo identifies the filesystem behind a mountpoint. Only Linux fills it
// in; elsewhere discovery relies on device names and usage.
type mountInfo struct {
	id       string // shared by every mount of the same filesystem
	devName  string // kernel block device name, which is also its diskstats name
	stableID string // survives reboots and device renames
	overlay  bool   // overlay root of a container
}

// fsRegistrationContext holds the shared lookup state needed to resolve a
// filesystem into its I/O device.
type fsRegistrationContext struct {
	filesystem     string // device part of optional FILESYSTEM env var
	filesystemName string // optional custom name from FILESYSTEM=device__name
	efPath         string // path to extra filesystems (default "/extra-filesystems")
	diskIoCounters map[string]disk.IOCountersStat
}

// fsCandidate is an extra filesystem found during discovery, before it is
// deduplicated and given a key.
type fsCandidate struct {
	device     string // partition device or configured path, used to match I/O by name
	hint       string // device part of an /extra-filesystems folder name
	mountpoint string
	customName string // explicit name from "__name"
	label      string // folder name used as the key when there is no custom name
	ioDevice   string
	mount      mountInfo
}

// diskDiscovery groups the transient state for a single initializeDiskInfo run so
// helper methods can share the same partitions, mount paths, and lookup functions
type diskDiscovery struct {
	agent          *Agent
	rootMountPoint string
	partitions     []disk.PartitionStat
	usageFn        func(string) (*disk.UsageStat, error)
	ctx            fsRegistrationContext
	rootIDs        []string // filesystem identities of the root filesystem
	extras         []*fsCandidate
	keys           *fsKeyStore
}

// prevDisk stores previous per-device disk counters for a given cache interval
type prevDisk struct {
	readBytes  uint64
	writeBytes uint64
	readTime   uint64 // cumulative ms spent on reads (from ReadTime)
	writeTime  uint64 // cumulative ms spent on writes (from WriteTime)
	ioTime     uint64 // cumulative ms spent doing I/O (from IoTime)
	weightedIO uint64 // cumulative weighted ms (queue-depth × ms, from WeightedIO)
	readCount  uint64 // cumulative read operation count
	writeCount uint64 // cumulative write operation count
	at         time.Time
}

// prevDiskFromCounter creates a prevDisk snapshot from a disk.IOCountersStat at time t.
func prevDiskFromCounter(d disk.IOCountersStat, t time.Time) prevDisk {
	return prevDisk{
		readBytes:  d.ReadBytes,
		writeBytes: d.WriteBytes,
		readTime:   d.ReadTime,
		writeTime:  d.WriteTime,
		ioTime:     d.IoTime,
		weightedIO: d.WeightedIO,
		readCount:  d.ReadCount,
		writeCount: d.WriteCount,
		at:         t,
	}
}

// parseFilesystemEntry parses a filesystem entry in the format "device__customname"
// Returns the device/filesystem part and the custom name part
func parseFilesystemEntry(entry string) (device, customName string) {
	entry = strings.TrimSpace(entry)
	if parts := strings.SplitN(entry, "__", 2); len(parts) == 2 {
		device = strings.TrimSpace(parts[0])
		customName = strings.TrimSpace(parts[1])
	} else {
		device = entry
	}
	return device, customName
}

func isDockerSpecialMountpoint(mountpoint string) bool {
	switch mountpoint {
	case "/etc/hosts", "/etc/resolv.conf", "/etc/hostname":
		return true
	}
	return false
}

// resolveIoDevice returns the diskstats device for a filesystem. The kernel
// device behind the mount is authoritative; name matching on the partition
// device and the configured hint is the fallback for anonymous devices
// (btrfs, ZFS) and platforms without statMount.
func (d *diskDiscovery) resolveIoDevice(devName, device, hint string) string {
	counters := d.ctx.diskIoCounters
	if _, ok := counters[devName]; ok && devName != "" {
		return devName
	}
	if name := normalizeDeviceName(device); name != "" {
		if _, ok := counters[name]; ok {
			return name
		}
	}
	for _, name := range []string{hint, device} {
		if io, ok := findIoDevice(name, counters); ok {
			return io
		}
	}
	return ""
}

// addRootFs resolves the root filesystem from FILESYSTEM, the root mount
// itself, root-like partitions and, as a last resort, the busiest disk.
func (d *diskDiscovery) addRootFs() {
	counters := d.ctx.diskIoCounters
	mountpoint, device, ioDevice := d.rootMountPoint, "", ""
	configured := false

	if fs := d.ctx.filesystem; fs != "" {
		if i := slices.IndexFunc(d.partitions, func(p disk.PartitionStat) bool {
			return filesystemMatchesPartitionSetting(fs, p)
		}); i >= 0 {
			device, mountpoint = d.partitions[i].Device, d.partitions[i].Mountpoint
			configured = true
		} else if io, ok := findIoDevice(fs, counters); ok {
			// FILESYSTEM may name a physical disk absent from partitions (e.g. ZFS
			// lists dataset paths like zroot/ROOT/default, not block devices).
			ioDevice = io
			configured = true
		} else {
			slog.Warn("Partition details not found", "filesystem", fs)
		}
	}

	mount := statMount(mountpoint)
	d.rootIDs = []string{"mount:" + mountpoint}
	if mount.id != "" {
		d.rootIDs = append(d.rootIDs, mount.id)
	}
	// A container's root is an overlay whose usage is that of the filesystem
	// holding the container, which is also where Docker keeps /etc/hosts.
	if mount.overlay {
		if hosts := statMount("/etc/hosts"); hosts.id != "" {
			mount = hosts
			d.rootIDs = append(d.rootIDs, hosts.id)
		}
	}

	if ioDevice == "" {
		ioDevice = d.resolveIoDevice(mount.devName, device, d.ctx.filesystem)
	}
	if ioDevice == "" && !configured {
		for _, p := range d.partitions {
			if isRootFallbackPartition(p, d.rootMountPoint) {
				if io, ok := findIoDevice(p.Device, counters); ok {
					ioDevice = io
					break
				}
			}
		}
		if ioDevice == "" {
			if ioDevice = mostActiveIoDevice(counters); ioDevice != "" {
				slog.Warn("Using most active device for root I/O; set FILESYSTEM to override", "device", ioDevice)
			} else {
				slog.Warn("Root I/O device not detected; set FILESYSTEM to override")
			}
		}
	} else if ioDevice == "" {
		slog.Warn("Root I/O unmapped; set FILESYSTEM", "device", device, "mountpoint", mountpoint)
	}

	d.agent.fsStats[rootFsKey] = &system.FsStats{Root: true, Mountpoint: mountpoint, Name: d.ctx.filesystemName, IoDevice: ioDevice}
	slog.Info("Detected disk", "name", cmp.Or(d.ctx.filesystemName, "root"), "device", device, "mount", mountpoint, "io", ioDevice, "root", true)
}

func isRootFallbackPartition(p disk.PartitionStat, rootMountPoint string) bool {
	return p.Mountpoint == rootMountPoint ||
		(isDockerSpecialMountpoint(p.Mountpoint) && strings.HasPrefix(p.Device, "/dev"))
}

// addCandidate queues an extra filesystem and resolves its mount and I/O device.
func (d *diskDiscovery) addCandidate(c *fsCandidate) {
	c.mount = statMount(c.mountpoint)
	c.ioDevice = d.resolveIoDevice(c.mount.devName, c.device, c.hint)
	d.extras = append(d.extras, c)
}

// findPartitionByFilesystemSetting matches an EXTRA_FILESYSTEMS entry against a
// discovered partition either by mountpoint or by device suffix. Autofs
// placeholders are skipped: they share the mountpoint of the real filesystem
// but not its device (#1931).
func findPartitionByFilesystemSetting(filesystem string, partitions []disk.PartitionStat) (disk.PartitionStat, bool) {
	for _, p := range partitions {
		if p.Fstype == "autofs" {
			continue
		}
		if strings.HasSuffix(p.Device, filesystem) || p.Mountpoint == filesystem {
			return p, true
		}
	}
	return disk.PartitionStat{}, false
}

// addConfiguredExtraFsEntry resolves one EXTRA_FILESYSTEMS entry, preferring a
// discovered partition and falling back to any path that disk.Usage accepts.
func (d *diskDiscovery) addConfiguredExtraFsEntry(filesystem, customName string) {
	if p, found := findPartitionByFilesystemSetting(filesystem, d.partitions); found {
		d.addCandidate(&fsCandidate{device: p.Device, mountpoint: p.Mountpoint, customName: customName})
		return
	}
	if _, err := d.usageFn(filesystem); err != nil {
		slog.Error("Invalid filesystem", "name", filesystem, "err", err)
		return
	}
	d.addCandidate(&fsCandidate{device: filesystem, mountpoint: filesystem, customName: customName})
}

// addConfiguredExtraFilesystems parses and registers the comma-separated
// EXTRA_FILESYSTEMS env var entries and reports whether disks should also be
// discovered automatically. That is the default when no entries are given; a
// list monitors only what it names unless it includes "auto", and "none"
// monitors no extra filesystems from partitions at all.
func (d *diskDiscovery) addConfiguredExtraFilesystems(extraFilesystems string) (auto bool) {
	listed := false
	for fsEntry := range strings.SplitSeq(extraFilesystems, ",") {
		filesystem, customName := parseFilesystemEntry(fsEntry)
		if filesystem == "" && customName == "" {
			continue
		}
		listed = true
		switch {
		case strings.EqualFold(filesystem, "auto") && customName == "":
			auto = true
		case strings.EqualFold(filesystem, "none") && customName == "":
		default:
			d.addConfiguredExtraFsEntry(filesystem, customName)
		}
	}
	return auto || !listed
}

// addExtraFilesystemFolders registers the folders in /extra-filesystems. The
// folder name is the key, so it stays the same when the device behind it is
// renamed; its device part is only a hint for matching I/O when the mount
// itself does not reveal the device.
func (d *diskDiscovery) addExtraFilesystemFolders(folderNames []string) {
	for _, folderName := range folderNames {
		mountpoint := filepath.Join(d.ctx.efPath, folderName)
		hint, customName := parseFilesystemEntry(folderName)
		c := &fsCandidate{hint: hint, mountpoint: mountpoint, customName: customName, label: hint}
		if i := slices.IndexFunc(d.partitions, func(p disk.PartitionStat) bool { return p.Mountpoint == mountpoint }); i >= 0 {
			c.device = d.partitions[i].Device
		}
		d.addCandidate(c)
	}
}

// autoExcludedFstypes are pseudo or image filesystems that disk discovery skips.
var autoExcludedFstypes = map[string]bool{
	"autofs": true, "devtmpfs": true, "iso9660": true, "overlay": true,
	"ramfs": true, "squashfs": true, "tmpfs": true, "udf": true,
}

// autoExcludedMounts are system and container paths that disk discovery skips.
var autoExcludedMounts = []string{
	"/boot/efi", "/efi", // EFI system partition
	"/dev", "/proc", "/run", "/snap", "/sys", "/System/Volumes",
	"/var/lib/containers", "/var/lib/docker", "/var/lib/kubelet",
}

// isAutoFilesystem reports whether disk discovery monitors partition p:
// a directory mount of a real block device outside system and container paths.
// ZFS datasets are left to the storage pool monitoring.
func (d *diskDiscovery) isAutoFilesystem(p disk.PartitionStat) bool {
	if _, isVolume := windowsVolumeName(p.Device); !isVolume && !strings.HasPrefix(p.Device, "/dev/") {
		return false
	}
	if autoExcludedFstypes[strings.ToLower(p.Fstype)] || p.Mountpoint == d.rootMountPoint || isUnderPath(p.Mountpoint, d.ctx.efPath) {
		return false
	}
	for _, prefix := range autoExcludedMounts {
		if isUnderPath(p.Mountpoint, prefix) {
			return false
		}
	}
	// Containers bind-mount single files such as /etc/hosts or /etc/localtime.
	info, err := os.Stat(p.Mountpoint)
	return err == nil && info.IsDir()
}

// isUnderPath reports whether path is dir or inside it.
func isUnderPath(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+"/")
}

// addAutoFilesystems adds every mounted block-device filesystem (#1928). It is
// the default when EXTRA_FILESYSTEMS is unset, or opted into with "auto". It
// runs after the explicit entries so those win the dedupe, and mounts of one
// filesystem collapse to a single entry.
func (d *diskDiscovery) addAutoFilesystems() {
	for _, p := range d.partitions {
		if d.isAutoFilesystem(p) {
			d.addCandidate(&fsCandidate{device: p.Device, mountpoint: p.Mountpoint})
		}
	}
}

// registerExtraFilesystems dedupes the extra filesystem candidates, picks
// their keys and adds them to fsStats.
func (d *diskDiscovery) registerExtraFilesystems() {
	kept := d.dedupeExtraFilesystems()
	for i, key := range d.assignKeys(kept) {
		c := kept[i]
		d.agent.fsStats[key] = &system.FsStats{Mountpoint: c.mountpoint, Name: c.customName, IoDevice: c.ioDevice}
		slog.Info("Detected disk", "name", key, "device", c.device, "mount", c.mountpoint, "io", c.ioDevice)
	}
}

// dedupeExtraFilesystems keeps one candidate per filesystem, dropping those
// that repeat the root filesystem or an earlier candidate. Filesystems are
// compared by identity where statMount provides one, so two disks with equal
// usage are never merged (#2103). Elsewhere they are compared by mountpoint
// and by usage against root (#1428).
func (d *diskDiscovery) dedupeExtraFilesystems() []*fsCandidate {
	root := d.agent.fsStats[rootFsKey]
	owners := make(map[string]string, len(d.extras)+len(d.rootIDs))
	for _, id := range d.rootIDs {
		owners[id] = rootFsKey
	}
	var rootUsage *disk.UsageStat
	rootUsageLoaded := false

	kept := make([]*fsCandidate, 0, len(d.extras))
	for _, c := range d.extras {
		owner, duplicate := owners["mount:"+c.mountpoint]
		if !duplicate && c.mount.id != "" {
			owner, duplicate = owners[c.mount.id]
		}
		if !duplicate && c.mount.id == "" && root != nil {
			if !rootUsageLoaded {
				rootUsage, _ = d.usageFn(root.Mountpoint)
				rootUsageLoaded = true
			}
			if usage, err := d.usageFn(c.mountpoint); err == nil && hasSameDiskUsage(rootUsage, usage) {
				owner, duplicate = rootFsKey, true
			}
		}
		if duplicate {
			sameAs := owner
			if owner == rootFsKey {
				sameAs = "root"
				if root != nil && root.Name == "" && c.customName != "" {
					// Keep the user's label for the disk on the root chart.
					root.Name = c.customName
				}
			}
			slog.Info("Ignoring duplicate FS", "name", cmp.Or(c.customName, c.label, c.device), "mount", c.mountpoint, "same as", sameAs)
			continue
		}
		owners["mount:"+c.mountpoint] = c.mountpoint
		if c.mount.id != "" {
			owners[c.mount.id] = c.mountpoint
		}
		kept = append(kept, c)
	}
	return kept
}

// assignKeys picks the key of each filesystem, which the hub uses as its chart
// name and history key. Explicit names come first, then keys pinned to stable
// filesystem IDs, then the I/O device name, which is pinned for the next run
// so it survives the device being renamed after a reboot (#294).
func (d *diskDiscovery) assignKeys(cands []*fsCandidate) []string {
	keys := make([]string, len(cands))
	used := map[string]bool{rootFsKey: true}
	for i, c := range cands {
		if name := cmp.Or(c.customName, c.label); name != "" {
			keys[i] = uniqueFsKey(name, c.mountpoint, used)
		}
	}
	for i, c := range cands {
		if keys[i] != "" || c.mount.stableID == "" {
			continue
		}
		if key, ok := d.keys.key(c.mount.stableID); ok && !used[key] {
			keys[i] = key
			used[key] = true
		}
	}
	for i, c := range cands {
		if keys[i] != "" {
			continue
		}
		keys[i] = uniqueFsKey(cmp.Or(c.ioDevice, normalizeDeviceName(c.device), filepath.Base(c.mountpoint)), c.mountpoint, used)
		if c.mount.stableID != "" {
			d.keys.pin(c.mount.stableID, keys[i])
		}
	}
	return keys
}

// uniqueFsKey returns key, or a variant of it when another filesystem already
// uses it, and marks the result as used. A taken key never drops a filesystem.
func uniqueFsKey(key, mountpoint string, used map[string]bool) string {
	if !used[key] {
		used[key] = true
		return key
	}
	unique := filepath.Base(mountpoint)
	if unique == "." || unique == string(filepath.Separator) || used[unique] {
		for n := 2; ; n++ {
			if unique = fmt.Sprintf("%s-%d", key, n); !used[unique] {
				break
			}
		}
	}
	slog.Warn("Disk name already in use", "name", key, "mount", mountpoint, "using", unique)
	used[unique] = true
	return unique
}

// Sets up the filesystems to monitor for disk usage and I/O.
func (a *Agent) initializeDiskInfo() {
	filesystemRaw, _ := utils.GetEnv("FILESYSTEM")
	filesystem, filesystemName := parseFilesystemEntry(filesystemRaw)

	partitions, err := disk.PartitionsWithContext(context.Background(), true)
	if err != nil {
		slog.Error("Error getting disk partitions", "err", err)
	}
	slog.Debug("Disk", "partitions", partitions)

	// trim trailing backslash for Windows devices (#1361)
	if runtime.GOOS == "windows" {
		for i, p := range partitions {
			partitions[i].Device = strings.TrimSuffix(p.Device, "\\")
		}
	}

	diskIoCounters, err := disk.IOCounters()
	if err != nil {
		slog.Error("Error getting diskstats", "err", err)
	}
	slog.Debug("Disk I/O", "diskstats", diskIoCounters)

	discovery := diskDiscovery{
		agent:          a,
		rootMountPoint: a.getRootMountPoint(),
		partitions:     partitions,
		usageFn:        disk.Usage,
		keys:           loadFsKeyStore(a.dataDir),
		ctx: fsRegistrationContext{
			filesystem:     filesystem,
			filesystemName: filesystemName,
			diskIoCounters: diskIoCounters,
			efPath:         "/extra-filesystems",
		},
	}

	discovery.addRootFs()
	extraFilesystems, _ := utils.GetEnv("EXTRA_FILESYSTEMS")
	auto := discovery.addConfiguredExtraFilesystems(extraFilesystems)
	if folders, err := os.ReadDir(discovery.ctx.efPath); err == nil {
		folderNames := make([]string, 0, len(folders))
		for _, folder := range folders {
			if folder.IsDir() {
				folderNames = append(folderNames, folder.Name())
			}
		}
		discovery.addExtraFilesystemFolders(folderNames)
	}
	if auto {
		discovery.addAutoFilesystems()
	}
	discovery.registerExtraFilesystems()
	discovery.keys.save()

	a.initializeDiskIoStats(diskIoCounters)
}

// hasSameDiskUsage compares root/extra usage with a small byte tolerance.
func hasSameDiskUsage(a, b *disk.UsageStat) bool {
	if a == nil || b == nil || a.Total == 0 || b.Total == 0 {
		return false
	}
	// Allow minor drift between sequential disk usage calls.
	const toleranceBytes uint64 = 16 * 1024 * 1024
	return withinUsageTolerance(a.Total, b.Total, toleranceBytes) &&
		withinUsageTolerance(a.Used, b.Used, toleranceBytes)
}

// withinUsageTolerance reports whether two byte values differ by at most tolerance.
func withinUsageTolerance(a, b, tolerance uint64) bool {
	if a >= b {
		return a-b <= tolerance
	}
	return b-a <= tolerance
}

type ioMatchCandidate struct {
	name  string
	bytes uint64
	ops   uint64
}

// findIoDevice prefers exact device/label matches, then falls back to a
// prefix-related candidate with the highest recent activity.
func findIoDevice(filesystem string, diskIoCounters map[string]disk.IOCountersStat) (string, bool) {
	filesystem = normalizeDeviceName(filesystem)
	if filesystem == "" {
		return "", false
	}

	candidates := []ioMatchCandidate{}

	for _, d := range diskIoCounters {
		if normalizeDeviceName(d.Name) == filesystem || (d.Label != "" && normalizeDeviceName(d.Label) == filesystem) {
			return d.Name, true
		}
		if prefixRelated(normalizeDeviceName(d.Name), filesystem) ||
			(d.Label != "" && prefixRelated(normalizeDeviceName(d.Label), filesystem)) {
			candidates = append(candidates, ioMatchCandidate{
				name:  d.Name,
				bytes: d.ReadBytes + d.WriteBytes,
				ops:   d.ReadCount + d.WriteCount,
			})
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.bytes > best.bytes ||
			(c.bytes == best.bytes && c.ops > best.ops) ||
			(c.bytes == best.bytes && c.ops == best.ops && c.name < best.name) {
			best = c
		}
	}

	slog.Info("Using disk I/O fallback", "requested", filesystem, "selected", best.name)
	return best.name, true
}

// mostActiveIoDevice returns the device with the highest I/O activity,
// or "" if diskIoCounters is empty.
func mostActiveIoDevice(diskIoCounters map[string]disk.IOCountersStat) string {
	var best ioMatchCandidate
	for _, d := range diskIoCounters {
		c := ioMatchCandidate{
			name:  d.Name,
			bytes: d.ReadBytes + d.WriteBytes,
			ops:   d.ReadCount + d.WriteCount,
		}
		if best.name == "" || c.bytes > best.bytes ||
			(c.bytes == best.bytes && c.ops > best.ops) ||
			(c.bytes == best.bytes && c.ops == best.ops && c.name < best.name) {
			best = c
		}
	}
	return best.name
}

// prefixRelated reports whether either identifier is a prefix of the other.
func prefixRelated(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// filesystemMatchesPartitionSetting checks whether a FILESYSTEM env var value
// matches a partition by mountpoint, exact device name, or prefix relationship
// (e.g. FILESYSTEM=ada0 matches partition /dev/ada0p2).
func filesystemMatchesPartitionSetting(filesystem string, p disk.PartitionStat) bool {
	filesystem = strings.TrimSpace(filesystem)
	if filesystem == "" {
		return false
	}
	if p.Mountpoint == filesystem {
		return true
	}

	fsName := normalizeDeviceName(filesystem)
	partName := normalizeDeviceName(p.Device)
	if fsName == "" || partName == "" {
		return false
	}
	if fsName == partName {
		return true
	}
	return prefixRelated(partName, fsName)
}

// normalizeDeviceName canonicalizes device strings for comparisons.
func normalizeDeviceName(value string) string {
	name := strings.TrimSpace(value)
	if volume, ok := windowsVolumeName(name); ok {
		return volume
	}
	name = filepath.Base(name)
	if name == "." {
		return ""
	}
	return name
}

// windowsVolumeName returns the canonical form of a bare Windows volume
// specifier, so that "C:", "c:", `C:\` and "C:/" all name the same drive.
// Drive letters are case-insensitive on Windows, so the letter is uppercased.
//
// filepath.Base cannot do this. On Windows it treats "C:" as a volume name
// with no path element to take the base of and returns "\", so every drive
// letter normalizes to the same key. findIoDevice then returns whichever
// counter the map happened to yield first, which registers the root
// filesystem under a random drive (#2417).
func windowsVolumeName(value string) (string, bool) {
	if len(value) < 2 || value[1] != ':' {
		return "", false
	}
	if c := value[0]; !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z') {
		return "", false
	}
	// Only separators may follow the specifier. "C:data" is a drive-relative
	// path, not a volume.
	for i := 2; i < len(value); i++ {
		if value[i] != '\\' && value[i] != '/' {
			return "", false
		}
	}
	return strings.ToUpper(value[:2]), true
}

// Sets start values for disk I/O stats.
func (a *Agent) initializeDiskIoStats(diskIoCounters map[string]disk.IOCountersStat) {
	a.fsNames = a.fsNames[:0]
	now := time.Now()
	// ZFS datasets have no /proc/diskstats entry, so they are excluded from
	// I/O tracking instead of warning about a missing device (#1541).
	var zfsMountpoints map[string]bool
	if a.storagePoolManager != nil {
		zfsMountpoints = a.storagePoolManager.ZfsMountpoints()
	}
	for _, stats := range a.fsStats {
		device := stats.IoDevice
		if device == "" || zfsMountpoints[stats.Mountpoint] {
			continue
		}
		// skip if not in diskIoCounters
		d, exists := diskIoCounters[device]
		if !exists {
			slog.Warn("Device not found in diskstats", "name", device)
			continue
		}
		// populate initial values
		stats.TotalRead = d.ReadBytes
		stats.TotalWrite = d.WriteBytes
		// add to list of valid io device names, which filesystems may share
		if !slices.Contains(a.fsNames, device) {
			a.setDiskBaseline(device, prevDiskFromCounter(d, now))
			a.fsNames = append(a.fsNames, device)
		}
	}
}

// Updates disk usage statistics for all monitored filesystems
func (a *Agent) updateDiskUsage(systemStats *system.Stats) {
	// Check if we should skip extra filesystem collection to avoid waking sleeping disks.
	// Root filesystem is always updated since it can't be sleeping while the agent runs.
	// Always collect on first call (lastDiskUsageUpdate is zero) or if caching is disabled.
	cacheExtraFs := a.diskUsageCacheDuration > 0 &&
		!a.lastDiskUsageUpdate.IsZero() &&
		time.Since(a.lastDiskUsageUpdate) < a.diskUsageCacheDuration

	// ZFS dataset mountpoints use `zfs list` values because statfs(2) reports
	// dataset-level usage that excludes child datasets (#1541).
	var zfsUsage map[string]zfsDatasetUsage
	if a.storagePoolManager != nil {
		zfsUsage = a.storagePoolManager.DatasetUsage()
	}

	// disk usage
	for _, stats := range a.fsStats {
		// Skip non-root filesystems if caching is active
		if cacheExtraFs && !stats.Root {
			continue
		}
		var total, used uint64
		var usedPct float64
		if u, ok := zfsUsage[stats.Mountpoint]; ok {
			total = u.used + u.avail
			used = u.used
			if total > 0 {
				usedPct = float64(used) / float64(total) * 100
			}
		} else if d, err := disk.Usage(stats.Mountpoint); err == nil {
			total = d.Total
			used = d.Used
			usedPct = d.UsedPercent
		} else {
			// reset stats if error (likely unmounted)
			slog.Error("Error getting disk stats", "name", stats.Mountpoint, "err", err)
			stats.DiskTotal = 0
			stats.DiskUsed = 0
			stats.TotalRead = 0
			stats.TotalWrite = 0
			continue
		}
		stats.DiskTotal = utils.BytesToGigabytes(total)
		stats.DiskUsed = utils.BytesToGigabytes(used)
		if stats.Root {
			systemStats.DiskTotal = stats.DiskTotal
			systemStats.DiskUsed = stats.DiskUsed
			systemStats.DiskPct = utils.TwoDecimals(usedPct)
		}
	}

	// Update the last disk usage update time when we've collected extra filesystems
	if !cacheExtraFs {
		a.lastDiskUsageUpdate = time.Now()
	}
}

// Updates disk I/O statistics for all monitored filesystems
func (a *Agent) updateDiskIo(cacheTimeMs uint16, systemStats *system.Stats) {
	// disk i/o (cache-aware per interval)
	if ioCounters, err := disk.IOCounters(a.fsNames...); err == nil {
		// Ensure map for this interval exists
		if _, ok := a.diskPrev[cacheTimeMs]; !ok {
			a.diskPrev[cacheTimeMs] = make(map[string]prevDisk)
		}
		now := time.Now()
		for name, d := range ioCounters {

			// Previous snapshot for this interval and device
			prev, ok := a.diskPrev[cacheTimeMs][name]
			firstSample := !ok
			if firstSample {
				// Seed from the latest counters of any interval, else seed from current
				if prev, ok = a.diskBaseline[name]; !ok {
					prev = prevDiskFromCounter(d, now)
				}
			}

			msElapsed := uint64(now.Sub(prev.at).Milliseconds())

			// Update per-interval snapshot
			a.diskPrev[cacheTimeMs][name] = prevDiskFromCounter(d, now)

			// Avoid division by zero or clock issues
			if msElapsed < 100 {
				continue
			}
			// The first sample of an interval must span at least half the interval.
			// Right after agent start the baseline is only a second or so old, and a
			// burst of startup I/O would be recorded as the rate for the whole interval.
			if firstSample && msElapsed < uint64(cacheTimeMs)/2 {
				continue
			}

			diskIORead := (d.ReadBytes - prev.readBytes) * 1000 / msElapsed
			diskIOWrite := (d.WriteBytes - prev.writeBytes) * 1000 / msElapsed
			readMbPerSecond := utils.BytesToMegabytes(float64(diskIORead))
			writeMbPerSecond := utils.BytesToMegabytes(float64(diskIOWrite))

			// validate values
			if readMbPerSecond > 50_000 || writeMbPerSecond > 50_000 {
				slog.Warn("Invalid disk I/O. Resetting.", "name", d.Name, "read", readMbPerSecond, "write", writeMbPerSecond)
				// also refresh agent baseline to avoid future negatives
				a.initializeDiskIoStats(ioCounters)
				continue
			}

			// These properties are calculated differently on different platforms,
			// but generally represent cumulative time spent doing reads/writes on the device.
			// This can surpass 100% if there are multiple concurrent I/O operations.
			// Linux kernel docs:
			// This is the total number of milliseconds spent by all reads (as
			// measured from __make_request() to end_that_request_last()).
			// https://www.kernel.org/doc/Documentation/iostats.txt (fields 4, 8)
			deltaReadTime := ioTimeDelta(d.ReadTime, prev.readTime)
			deltaWriteTime := ioTimeDelta(d.WriteTime, prev.writeTime)
			diskReadTime := utils.TwoDecimals(float64(deltaReadTime) / float64(msElapsed) * 100)
			diskWriteTime := utils.TwoDecimals(float64(deltaWriteTime) / float64(msElapsed) * 100)

			// I/O utilization %: fraction of wall time the device had any I/O in progress (0-100).
			diskIoUtilPct := utils.TwoDecimals(float64(ioTimeDelta(d.IoTime, prev.ioTime)) / float64(msElapsed) * 100)

			// Weighted I/O: queue-depth weighted I/O time, normalized to interval (can exceed 100%).
			// Linux kernel field 11: incremented by iops_in_progress × ms_since_last_update.
			// Used to display queue depth. Multipled by 100 to increase accuracy of digit truncation (divided by 100 in UI).
			diskWeightedIO := utils.TwoDecimals(float64(ioTimeDelta(d.WeightedIO, prev.weightedIO)) / float64(msElapsed) * 100)

			// r_await / w_await: average time per read/write operation in milliseconds.
			// Equivalent to r_await and w_await in iostat.
			var rAwait, wAwait float64
			if deltaReadCount := d.ReadCount - prev.readCount; deltaReadCount > 0 {
				rAwait = utils.TwoDecimals(float64(deltaReadTime) / float64(deltaReadCount))
			}
			if deltaWriteCount := d.WriteCount - prev.writeCount; deltaWriteCount > 0 {
				wAwait = utils.TwoDecimals(float64(deltaWriteTime) / float64(deltaWriteCount))
			}

			// Update the baseline that seeds new intervals
			a.setDiskBaseline(name, prevDiskFromCounter(d, now))
			diskIoStats := [6]float64{diskReadTime, diskWriteTime, diskIoUtilPct, rAwait, wAwait, diskWeightedIO}
			for _, stats := range a.fsStats {
				if stats.IoDevice != d.Name {
					continue
				}
				stats.TotalRead = d.ReadBytes
				stats.TotalWrite = d.WriteBytes
				stats.DiskReadPs = readMbPerSecond
				stats.DiskWritePs = writeMbPerSecond
				stats.DiskReadBytes = diskIORead
				stats.DiskWriteBytes = diskIOWrite
				stats.DiskIoStats = diskIoStats

				if stats.Root {
					systemStats.DiskReadPs = stats.DiskReadPs
					systemStats.DiskWritePs = stats.DiskWritePs
					systemStats.DiskIO[0] = diskIORead
					systemStats.DiskIO[1] = diskIOWrite
					systemStats.DiskIOTotal[0] = d.ReadBytes
					systemStats.DiskIOTotal[1] = d.WriteBytes
					systemStats.DiskIoStats = diskIoStats
				}
			}
		}
	}
}

// setDiskBaseline stores the latest counters of a device. A cache interval
// without its own snapshot measures its first sample from them.
func (a *Agent) setDiskBaseline(name string, d prevDisk) {
	if a.diskBaseline == nil {
		a.diskBaseline = make(map[string]prevDisk)
	}
	a.diskBaseline[name] = d
}

// ioTimeDelta returns the increase of a cumulative millisecond counter from
// the disk I/O stats. Linux prints these fields of /proc/diskstats as 32-bit
// unsigned ints, so they wrap to zero at 2^32. A busy disk reaches that in
// days for the weighted I/O time. Other platforms report 64-bit counters,
// so a lower value there is a reset.
func ioTimeDelta(current, previous uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	if runtime.GOOS == "linux" && previous <= math.MaxUint32 {
		return current + (math.MaxUint32 + 1 - previous)
	}
	return 0
}

// getRootMountPoint returns the appropriate root mount point for the system.
// On Windows it returns the system drive (e.g. "C:").
// For immutable systems like Fedora Silverblue, it returns /sysroot instead of /
func (a *Agent) getRootMountPoint() string {
	if runtime.GOOS == "windows" {
		if sd := os.Getenv("SystemDrive"); sd != "" {
			return sd
		}
		return "C:"
	}

	// 1. Check if /etc/os-release contains indicators of an immutable system
	if osReleaseContent, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(osReleaseContent)
		if strings.Contains(content, "fedora") && strings.Contains(content, "silverblue") ||
			strings.Contains(content, "coreos") ||
			strings.Contains(content, "flatcar") ||
			strings.Contains(content, "rhel-atomic") ||
			strings.Contains(content, "centos-atomic") {
			// Verify that /sysroot exists before returning it
			if _, err := os.Stat("/sysroot"); err == nil {
				return "/sysroot"
			}
		}
	}

	// 2. Check if /run/ostree is present (ostree-based systems like Silverblue)
	if _, err := os.Stat("/run/ostree"); err == nil {
		// Verify that /sysroot exists before returning it
		if _, err := os.Stat("/sysroot"); err == nil {
			return "/sysroot"
		}
	}

	return "/"
}
