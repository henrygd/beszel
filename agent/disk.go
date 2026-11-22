package agent

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
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

// diskDiscovery holds the state of one initializeDiskInfo run.
type diskDiscovery struct {
	agent          *Agent
	rootMountPoint string
	partitions     []disk.PartitionStat
	ioCounters     map[string]disk.IOCountersStat
	usageFn        func(string) (*disk.UsageStat, error)
	isWindows      bool
	isDarwin       bool
	efPath         string // folder whose subfolders are extra filesystems ("/extra-filesystems")
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

// evalSymlinks resolves device symlinks; it is a seam so tests can fake the
// /dev topology (e.g. /dev/vg/lv -> /dev/dm-N created by udev for LVM).
var evalSymlinks = filepath.EvalSymlinks

// apfsPhysicalDisk returns the physical disk backing an APFS volume
// (e.g. disk13s1 -> disk12), or "" when it can't be resolved. It is
// implemented with diskutil on darwin. Also acts as a seam so tests can fake the
// APFS volume -> physical disk topology on any platform.
var apfsPhysicalDisk = diskutilPhysicalDisk

// apfsIoDevice returns the physical disk I/O key for an APFS volume on
// darwin, if it has I/O counters.
func (d *diskDiscovery) apfsIoDevice(device string) (string, bool) {
	if !d.isDarwin || !filepath.IsAbs(device) {
		return "", false
	}
	physical := apfsPhysicalDisk(device)
	if physical == "" {
		return "", false
	}
	_, ok := d.ioCounters[physical]
	return physical, ok
}

// ioKey returns the diskstats device a filesystem's I/O is read from. Hints
// are device names the user gave (the FILESYSTEM value or an
// /extra-filesystems folder name) and win over guesses from the device itself.
// When nothing matches, ok is false and key is the device's base name.
func (d *diskDiscovery) ioKey(device string, hints ...string) (key string, ok bool) {
	key = device
	if !d.isWindows {
		key = filepath.Base(device)
	}
	if _, ok := d.ioCounters[key]; ok {
		return key, true
	}
	// APFS volumes sit on a synthesized container disk (disk13s1 in container
	// disk13) that IOKit reports no counters for, and whose number is
	// unrelated to the physical disk (disk12). Resolve the physical store
	// before the prefix fallback can pick an unrelated disk (disk1 is a prefix
	// of disk13s1).
	if physical, ok := d.apfsIoDevice(device); ok {
		return physical, true
	}
	// A hint also covers mapper paths (e.g. a luks UUID) that obscure the
	// underlying block device.
	for _, hint := range hints {
		if match, ok := findIoDevice(hint, d.ioCounters); ok {
			return match, true
		}
	}
	if match, ok := findIoDevice(device, d.ioCounters); ok {
		return match, true
	}
	// Device-mapper mounts appear as symlinked paths like /dev/vg/lv whose
	// base name matches neither the diskstats name (dm-N) nor the dm label
	// (vg-lv); the resolved target's base is one of those. Bare names (folder
	// devices, ZFS datasets) are skipped because they would resolve relative
	// to the agent's working directory.
	if !d.isWindows && filepath.IsAbs(device) {
		if resolved, err := evalSymlinks(device); err == nil {
			if match, ok := findIoDevice(resolved, d.ioCounters); ok {
				return match, true
			}
		}
	}
	return key, false
}

// register tracks a filesystem by its mountpoint and reports whether it was
// added. A root filesystem on an already tracked mountpoint (e.g. the root
// drive also listed in EXTRA_FILESYSTEMS) promotes that entry.
func (d *diskDiscovery) register(ioDevice, mountpoint string, root bool, name string) bool {
	if fs, exists := d.agent.fsStats[mountpoint]; exists {
		if !root {
			return false
		}
		fs.Root = true
		fs.IoDevice = ioDevice
		if name != "" {
			fs.Name = name
		}
		return true
	}
	d.agent.fsStats[mountpoint] = &system.FsStats{Root: root, Mountpoint: mountpoint, Name: name, IoDevice: ioDevice}
	slog.Info("Detected disk", "name", cmp.Or(name, ioDevice), "mount", mountpoint, "io", ioDevice, "root", root)
	return true
}

// addConfiguredRootFs resolves FILESYSTEM against partitions first, then falls
// back to direct diskstats matching for setups like ZFS where partitions do not
// expose the physical device name.
func (d *diskDiscovery) addConfiguredRootFs(filesystem, name string) bool {
	if filesystem == "" {
		return false
	}
	if p, found := findPartition(filesystem, d.partitions); found {
		key, ok := d.ioKey(p.Device, filesystem)
		if !ok {
			slog.Warn("Root I/O device not found in diskstats", "filesystem", filesystem, "device", p.Device)
		}
		return d.register(key, p.Mountpoint, true, name)
	}
	// FILESYSTEM may name a physical disk absent from partitions (e.g. ZFS lists
	// dataset paths like zroot/ROOT/default, not block devices).
	if key, ok := findIoDevice(filesystem, d.ioCounters); ok {
		return d.register(key, d.rootMountPoint, true, name)
	}
	slog.Warn("Partition details not found", "filesystem", filesystem)
	return false
}

func isRootFallbackPartition(p disk.PartitionStat, rootMountPoint string) bool {
	return p.Mountpoint == rootMountPoint ||
		(isDockerSpecialMountpoint(p.Mountpoint) && strings.HasPrefix(p.Device, "/dev"))
}

// addPartitionRootFs registers a partition that looks like the root mount if
// its device maps to an I/O device. Without a match the caller tries the next
// candidate and finally addLastResortRootFs.
func (d *diskDiscovery) addPartitionRootFs(device, mountpoint string) bool {
	key, ok := d.ioKey(device)
	if !ok {
		return false
	}
	return d.register(key, mountpoint, true, "")
}

// addLastResortRootFs is only used when neither FILESYSTEM nor partition-based
// heuristics can identify root, so it picks the busiest I/O device as a final
// fallback and preserves the root mountpoint for usage collection.
func (d *diskDiscovery) addLastResortRootFs() {
	key := mostActiveIoDevice(d.ioCounters)
	if key != "" {
		slog.Warn("Using most active device for root I/O; set FILESYSTEM to override", "device", key)
	} else {
		slog.Warn("Root I/O device not detected; set FILESYSTEM to override")
	}
	d.register(key, d.rootMountPoint, true, "")
}

// isDeviceSetting reports whether a setting can name a device rather than only
// a mountpoint: a bare name (sda1, mapper/vg-lv), a /dev path or a Windows
// volume. /mnt/sda1 must not match device sda1 by its base name.
func isDeviceSetting(setting string) bool {
	if _, ok := windowsVolumeName(setting); ok {
		return true
	}
	return strings.HasPrefix(setting, "/dev/") || !strings.HasPrefix(setting, "/") && !filepath.IsAbs(setting)
}

// findPartition returns the partition a FILESYSTEM or EXTRA_FILESYSTEMS value
// refers to. A mount at that path wins, then an exact device name, then a
// prefix-related one (FILESYSTEM=ada0 matches /dev/ada0p2), so sda1 never
// resolves to sda10. When several mounts share a path the last one is the
// visible one, and autofs placeholders are skipped (#1931).
func findPartition(setting string, partitions []disk.PartitionStat) (disk.PartitionStat, bool) {
	setting = strings.TrimSpace(setting)
	if setting == "" {
		return disk.PartitionStat{}, false
	}
	name := ""
	if isDeviceSetting(setting) {
		name = normalizeDeviceName(setting)
	}
	byMount, byDevice, byPrefix := -1, -1, -1
	for i, p := range partitions {
		if p.Fstype == "autofs" {
			continue
		}
		if p.Mountpoint == setting {
			byMount = i
			continue
		}
		if name == "" {
			continue
		}
		device := normalizeDeviceName(p.Device)
		if device == name {
			if byDevice < 0 {
				byDevice = i
			}
		} else if byPrefix < 0 && prefixRelated(device, name) {
			byPrefix = i
		}
	}
	for _, i := range []int{byMount, byDevice, byPrefix} {
		if i >= 0 {
			return partitions[i], true
		}
	}
	return disk.PartitionStat{}, false
}

// addConfiguredExtraFilesystems registers the comma-separated
// EXTRA_FILESYSTEMS entries, preferring a matching partition and falling back
// to any path that disk.Usage accepts.
func (d *diskDiscovery) addConfiguredExtraFilesystems(extraFilesystems string) {
	for entry := range strings.SplitSeq(extraFilesystems, ",") {
		setting, name := parseFilesystemEntry(entry)
		if setting == "" {
			continue
		}
		if p, found := findPartition(setting, d.partitions); found {
			key, _ := d.ioKey(p.Device)
			d.register(key, p.Mountpoint, false, name)
			continue
		}
		// Usage of a device node is the usage of /dev, not of the device.
		if strings.HasPrefix(setting, "/dev/") {
			slog.Error("Filesystem not mounted", "name", setting)
			continue
		}
		if _, err := d.usageFn(setting); err != nil {
			slog.Error("Invalid filesystem", "name", setting, "err", err)
			continue
		}
		key, _ := d.ioKey(filepath.Base(setting))
		d.register(key, setting, false, name)
	}
}

// addExtraFilesystemFolders registers each folder in /extra-filesystems. The
// I/O device comes from the partition mounted there, else from the folder
// name ("device__name"), which also acts as a hint when the partition device
// does not map to diskstats.
func (d *diskDiscovery) addExtraFilesystemFolders(folderNames []string) {
	// Only direct children count: when the host root is bind-mounted here,
	// disk.Partitions(true) also returns its nested mounts (/proc, /sys, ...).
	mounted := make(map[string]disk.PartitionStat)
	for _, p := range d.partitions {
		if filepath.Dir(p.Mountpoint) == d.efPath && p.Fstype != "autofs" {
			mounted[p.Mountpoint] = p
		}
	}
	for _, folder := range folderNames {
		mountpoint := filepath.Join(d.efPath, folder)
		hint, name := parseFilesystemEntry(folder)
		device := hint
		if p, ok := mounted[mountpoint]; ok && strings.TrimSpace(p.Device) != "" {
			device = strings.TrimSpace(p.Device)
		}
		key, _ := d.ioKey(device, hint)
		d.register(key, mountpoint, false, name)
	}
}

// Sets up the filesystems to monitor for disk usage and I/O.
func (a *Agent) initializeDiskInfo() {
	filesystemRaw, _ := utils.GetEnv("FILESYSTEM")
	filesystem, filesystemName := parseFilesystemEntry(filesystemRaw)
	isWindows := runtime.GOOS == "windows"

	partitions, err := disk.PartitionsWithContext(context.Background(), true)
	if err != nil {
		slog.Error("Error getting disk partitions", "err", err)
	}
	slog.Debug("Disk", "partitions", partitions)

	// trim trailing backslash for Windows devices (#1361)
	if isWindows {
		for i, p := range partitions {
			partitions[i].Device = strings.TrimSuffix(p.Device, "\\")
		}
	}

	diskIoCounters, err := disk.IOCounters()
	if err != nil {
		slog.Error("Error getting diskstats", "err", err)
	}
	slog.Debug("Disk I/O", "diskstats", diskIoCounters)

	d := diskDiscovery{
		agent:          a,
		rootMountPoint: a.getRootMountPoint(),
		partitions:     partitions,
		ioCounters:     diskIoCounters,
		usageFn:        disk.Usage,
		isWindows:      isWindows,
		isDarwin:       runtime.GOOS == "darwin",
		efPath:         "/extra-filesystems",
	}

	hasRoot := d.addConfiguredRootFs(filesystem, filesystemName)

	if extraFilesystems, exists := utils.GetEnv("EXTRA_FILESYSTEMS"); exists {
		d.addConfiguredExtraFilesystems(extraFilesystems)
	}

	for _, p := range partitions {
		if hasRoot {
			break
		}
		if isRootFallbackPartition(p, d.rootMountPoint) {
			hasRoot = d.addPartitionRootFs(p.Device, p.Mountpoint)
		}
	}

	if folders, err := os.ReadDir(d.efPath); err == nil {
		folderNames := make([]string, 0, len(folders))
		for _, folder := range folders {
			if folder.IsDir() {
				folderNames = append(folderNames, folder.Name())
			}
		}
		d.addExtraFilesystemFolders(folderNames)
	}

	// If no root filesystem set, try the most active I/O device as a last
	// resort (e.g. ZFS where dataset names are unrelated to disk names).
	if !hasRoot {
		d.addLastResortRootFs()
	}

	a.pruneDuplicateFilesystems(diskIoCounters, disk.Usage)
	a.initializeDiskIoStats(diskIoCounters)
}

// pruneDuplicateFilesystems drops filesystems that repeat one already kept,
// such as a folder of the root disk bind-mounted into /extra-filesystems
// (#1428). Same usage alone is not enough: in Docker the container's / reports
// the usage of whichever host filesystem holds the Docker root dir (#2103), so
// the I/O devices must match too. A filesystem without diskstats matches any
// device.
func (a *Agent) pruneDuplicateFilesystems(ioCounters map[string]disk.IOCountersStat, usageFn func(string) (*disk.UsageStat, error)) {
	type keptFs struct {
		ioDevice string
		usage    *disk.UsageStat
	}
	sameDevice := func(x, y string) bool {
		_, xKnown := ioCounters[x]
		_, yKnown := ioCounters[y]
		return x == y || !xKnown || !yKnown
	}
	// Root first, so it is the one kept.
	mountpoints := slices.SortedFunc(maps.Keys(a.fsStats), func(x, y string) int {
		if rx, ry := a.fsStats[x].Root, a.fsStats[y].Root; rx != ry {
			if rx {
				return -1
			}
			return 1
		}
		return strings.Compare(x, y)
	})
	var kept []keptFs
	for _, mountpoint := range mountpoints {
		fs := a.fsStats[mountpoint]
		usage, err := usageFn(fs.Mountpoint)
		if err != nil {
			continue
		}
		if !fs.Root && slices.ContainsFunc(kept, func(k keptFs) bool {
			return sameDevice(k.ioDevice, fs.IoDevice) && hasSameDiskUsage(k.usage, usage)
		}) {
			slog.Info("Ignoring duplicate FS", "name", cmp.Or(fs.Name, fs.IoDevice), "mount", fs.Mountpoint)
			delete(a.fsStats, mountpoint)
			continue
		}
		kept = append(kept, keptFs{fs.IoDevice, usage})
	}
}

// setExtraFsStats adds the non-root filesystems to the payload. They are keyed
// by custom name, else by I/O device, as chart history is. When two share a
// key (e.g. two mounts on one device) the later mountpoint uses its base name.
func (a *Agent) setExtraFsStats(stats *system.Stats, info *system.Info) {
	stats.ExtraFs = make(map[string]*system.FsStats)
	info.ExtraFsPct = make(map[string]float64)
	for _, mountpoint := range slices.Sorted(maps.Keys(a.fsStats)) {
		fs := a.fsStats[mountpoint]
		if fs.Root {
			if fs.Name != "" {
				info.RootDiskName = fs.Name
			}
			continue
		}
		if fs.DiskTotal <= 0 {
			continue
		}
		key := cmp.Or(fs.Name, fs.IoDevice)
		if _, taken := stats.ExtraFs[key]; taken || key == "" {
			key = filepath.Base(mountpoint)
			if _, taken := stats.ExtraFs[key]; taken {
				key = mountpoint
			}
		}
		stats.ExtraFs[key] = fs
		info.ExtraFsPct[key] = utils.TwoDecimals(fs.DiskUsed / fs.DiskTotal * 100)
	}
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

func ioCandidate(d disk.IOCountersStat) ioMatchCandidate {
	return ioMatchCandidate{name: d.Name, bytes: d.ReadBytes + d.WriteBytes, ops: d.ReadCount + d.WriteCount}
}

// moreActive reports whether c has seen more I/O than best, breaking ties by
// name so the choice is deterministic.
func (c ioMatchCandidate) moreActive(best ioMatchCandidate) bool {
	if c.bytes != best.bytes {
		return c.bytes > best.bytes
	}
	if c.ops != best.ops {
		return c.ops > best.ops
	}
	return c.name < best.name
}

// findIoDevice prefers exact device/label matches, then falls back to a
// prefix-related candidate with the highest recent activity.
func findIoDevice(filesystem string, diskIoCounters map[string]disk.IOCountersStat) (string, bool) {
	filesystem = normalizeDeviceName(filesystem)
	if filesystem == "" {
		return "", false
	}

	var best ioMatchCandidate
	for _, d := range diskIoCounters {
		name, label := normalizeDeviceName(d.Name), ""
		if d.Label != "" {
			label = normalizeDeviceName(d.Label)
		}
		if name == filesystem || (label != "" && label == filesystem) {
			return d.Name, true
		}
		if prefixRelated(name, filesystem) || (label != "" && prefixRelated(label, filesystem)) {
			if c := ioCandidate(d); best.name == "" || c.moreActive(best) {
				best = c
			}
		}
	}

	if best.name == "" {
		return "", false
	}
	slog.Info("Using disk I/O fallback", "requested", filesystem, "selected", best.name)
	return best.name, true
}

// mostActiveIoDevice returns the device with the highest I/O activity,
// or "" if diskIoCounters is empty.
func mostActiveIoDevice(diskIoCounters map[string]disk.IOCountersStat) string {
	var best ioMatchCandidate
	for _, d := range diskIoCounters {
		if c := ioCandidate(d); best.name == "" || c.moreActive(best) {
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
		if zfsMountpoints[stats.Mountpoint] || stats.IoDevice == "" {
			continue
		}
		// skip if not in diskIoCounters
		d, exists := diskIoCounters[stats.IoDevice]
		if !exists {
			slog.Warn("Device not found in diskstats", "name", stats.IoDevice)
			continue
		}
		// populate initial values
		stats.TotalRead = d.ReadBytes
		stats.TotalWrite = d.WriteBytes
		// several filesystems can share a device; it is read once
		if !slices.Contains(a.fsNames, stats.IoDevice) {
			a.setDiskBaseline(stats.IoDevice, prevDiskFromCounter(d, now))
			a.fsNames = append(a.fsNames, stats.IoDevice)
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
		// Filesystems by I/O device. Deltas are computed once per device, as a
		// second read within the same call would see a near-zero interval.
		filesystems := make(map[string][]*system.FsStats, len(a.fsNames))
		for _, stats := range a.fsStats {
			if stats.IoDevice != "" {
				filesystems[stats.IoDevice] = append(filesystems[stats.IoDevice], stats)
			}
		}
		for name, d := range ioCounters {
			tracked := filesystems[d.Name]
			if len(tracked) == 0 {
				continue
			}

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
			for _, stats := range tracked {
				stats.TotalRead = d.ReadBytes
				stats.TotalWrite = d.WriteBytes
				stats.DiskReadBytes = diskIORead
				stats.DiskWriteBytes = diskIOWrite
				stats.DiskIoStats = [6]float64{diskReadTime, diskWriteTime, diskIoUtilPct, rAwait, wAwait, diskWeightedIO}
				if stats.Root {
					systemStats.DiskIO = [2]uint64{diskIORead, diskIOWrite}
					systemStats.DiskIOTotal = [2]uint64{d.ReadBytes, d.WriteBytes}
					systemStats.DiskIoStats = stats.DiskIoStats
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
