package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/disk"
)

// fsRegistrationContext holds the shared lookup state needed to resolve a
// filesystem into the tracked fsStats key and metadata.
type fsRegistrationContext struct {
	filesystem     string // value of optional FILESYSTEM env var
	isWindows      bool
	efPath         string // path to extra filesystems (default "/extra-filesystems")
	diskIoCounters map[string]disk.IOCountersStat
}

// diskDiscovery groups the transient state for a single initializeDiskInfo run so
// helper methods can share the same partitions, mount paths, and lookup functions
type diskDiscovery struct {
	agent          *Agent
	rootMountPoint string
	partitions     []disk.PartitionStat
	usageFn        func(string) (*disk.UsageStat, error)
	ctx            fsRegistrationContext
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

// extraFilesystemPartitionInfo derives the I/O device and optional display name
// for a mounted /extra-filesystems partition. Prefer the partition device reported
// by the system and only use the folder name for custom naming metadata.
func extraFilesystemPartitionInfo(p disk.PartitionStat) (device, customName string) {
	device = strings.TrimSpace(p.Device)
	folderDevice, customName := parseFilesystemEntry(filepath.Base(p.Mountpoint))
	if device == "" {
		device = folderDevice
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

// registerFilesystemStats resolves the tracked key and stats payload for a
// filesystem before it is inserted into fsStats.
func registerFilesystemStats(existing map[string]*system.FsStats, device, mountpoint string, root bool, customName string, ctx fsRegistrationContext) (string, *system.FsStats, bool) {
	key := device
	if !ctx.isWindows {
		key = filepath.Base(device)
	}

	if root {
		// Try to map root device to a diskIoCounters entry. First checks for an
		// exact key match, then uses findIoDevice for normalized / prefix-based
		// matching (e.g. nda0p2 -> nda0), and finally falls back to FILESYSTEM.
		if _, ioMatch := ctx.diskIoCounters[key]; !ioMatch {
			if matchedKey, match := findIoDevice(key, ctx.diskIoCounters); match {
				key = matchedKey
			} else if ctx.filesystem != "" {
				if matchedKey, match := findIoDevice(ctx.filesystem, ctx.diskIoCounters); match {
					key = matchedKey
				}
			}
			if _, ioMatch = ctx.diskIoCounters[key]; !ioMatch {
				slog.Warn("Root I/O unmapped; set FILESYSTEM", "device", device, "mountpoint", mountpoint)
			}
		}
	} else {
		// Check if non-root has diskstats and prefer the folder device for
		// /extra-filesystems mounts when the discovered partition device is a
		// mapper path (e.g. luks UUID) that obscures the underlying block device.
		if _, ioMatch := ctx.diskIoCounters[key]; !ioMatch {
			if strings.HasPrefix(mountpoint, ctx.efPath) {
				folderDevice, _ := parseFilesystemEntry(filepath.Base(mountpoint))
				if folderDevice != "" {
					if matchedKey, match := findIoDevice(folderDevice, ctx.diskIoCounters); match {
						key = matchedKey
					}
				}
			}
			if _, ioMatch = ctx.diskIoCounters[key]; !ioMatch {
				if matchedKey, match := findIoDevice(key, ctx.diskIoCounters); match {
					key = matchedKey
				}
			}
		}
	}

	if _, exists := existing[key]; exists {
		return "", nil, false
	}

	fsStats := &system.FsStats{Root: root, Mountpoint: mountpoint}
	if customName != "" {
		fsStats.Name = customName
	}
	return key, fsStats, true
}

// addFsStat inserts a discovered filesystem if it resolves to a new tracking
// key. The key selection itself lives in buildFsStatRegistration so that logic
// can stay directly unit-tested.
func (d *diskDiscovery) addFsStat(device, mountpoint string, root bool, customName string) {
	key, fsStats, ok := registerFilesystemStats(d.agent.fsStats, device, mountpoint, root, customName, d.ctx)
	if !ok {
		return
	}
	d.agent.fsStats[key] = fsStats
	name := key
	if customName != "" {
		name = customName
	}
	slog.Info("Detected disk", "name", name, "device", device, "mount", mountpoint, "io", key, "root", root)
}

// addConfiguredRootFs resolves FILESYSTEM against partitions first, then falls
// back to direct diskstats matching for setups like ZFS where partitions do not
// expose the physical device name.
func (d *diskDiscovery) addConfiguredRootFs() bool {
	if d.ctx.filesystem == "" {
		return false
	}

	for _, p := range d.partitions {
		if filesystemMatchesPartitionSetting(d.ctx.filesystem, p) {
			d.addFsStat(p.Device, p.Mountpoint, true, "")
			return true
		}
	}

	// FILESYSTEM may name a physical disk absent from partitions (e.g. ZFS lists
	// dataset paths like zroot/ROOT/default, not block devices).
	if ioKey, match := findIoDevice(d.ctx.filesystem, d.ctx.diskIoCounters); match {
		d.agent.fsStats[ioKey] = &system.FsStats{Root: true, Mountpoint: d.rootMountPoint}
		return true
	}

	slog.Warn("Partition details not found", "filesystem", d.ctx.filesystem)
	return false
}

func isRootFallbackPartition(p disk.PartitionStat, rootMountPoint string) bool {
	return p.Mountpoint == rootMountPoint ||
		(isDockerSpecialMountpoint(p.Mountpoint) && strings.HasPrefix(p.Device, "/dev"))
}

// addPartitionRootFs handles the non-configured root fallback path when a
// partition looks like the active root mount but still needs translating to an
// I/O device key.
func (d *diskDiscovery) addPartitionRootFs(device, mountpoint string) bool {
	fs, match := findIoDevice(filepath.Base(device), d.ctx.diskIoCounters)
	if !match {
		return false
	}
	// The resolved I/O device is already known here, so use it directly to avoid
	// a second fallback search inside buildFsStatRegistration.
	d.addFsStat(fs, mountpoint, true, "")
	return true
}

// addLastResortRootFs is only used when neither FILESYSTEM nor partition-based
// heuristics can identify root, so it picks the busiest I/O device as a final
// fallback and preserves the root mountpoint for usage collection.
func (d *diskDiscovery) addLastResortRootFs() {
	rootKey := mostActiveIoDevice(d.ctx.diskIoCounters)
	if rootKey != "" {
		slog.Warn("Using most active device for root I/O; set FILESYSTEM to override", "device", rootKey)
	} else {
		rootKey = filepath.Base(d.rootMountPoint)
		if _, exists := d.agent.fsStats[rootKey]; exists {
			rootKey = "root"
		}
		slog.Warn("Root I/O device not detected; set FILESYSTEM to override")
	}
	d.agent.fsStats[rootKey] = &system.FsStats{Root: true, Mountpoint: d.rootMountPoint}
}

// findPartitionByFilesystemSetting matches an EXTRA_FILESYSTEMS entry against a
// discovered partition either by mountpoint or by device suffix.
func findPartitionByFilesystemSetting(filesystem string, partitions []disk.PartitionStat) (disk.PartitionStat, bool) {
	for _, p := range partitions {
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
		d.addFsStat(p.Device, p.Mountpoint, false, customName)
		return
	}

	if _, err := d.usageFn(filesystem); err == nil {
		d.addFsStat(filepath.Base(filesystem), filesystem, false, customName)
		return
	} else {
		slog.Error("Invalid filesystem", "name", filesystem, "err", err)
	}
}

// addConfiguredExtraFilesystems parses and registers the comma-separated
// EXTRA_FILESYSTEMS env var entries.
func (d *diskDiscovery) addConfiguredExtraFilesystems(extraFilesystems string) {
	for fsEntry := range strings.SplitSeq(extraFilesystems, ",") {
		filesystem, customName := parseFilesystemEntry(fsEntry)
		d.addConfiguredExtraFsEntry(filesystem, customName)
	}
}

// addPartitionExtraFs registers partitions mounted under /extra-filesystems so
// their display names can come from the folder name while their I/O keys still
// prefer the underlying partition device. Only direct children are matched to
// avoid registering nested virtual mounts (e.g. /proc, /sys) that are returned by
// disk.Partitions(true) when the host root is bind-mounted in /extra-filesystems.
func (d *diskDiscovery) addPartitionExtraFs(p disk.PartitionStat) {
	if filepath.Dir(p.Mountpoint) != d.ctx.efPath {
		return
	}
	device, customName := extraFilesystemPartitionInfo(p)
	d.addFsStat(device, p.Mountpoint, false, customName)
}

// addExtraFilesystemFolders handles bare directories under /extra-filesystems
// that may not appear in partition discovery, while skipping mountpoints that
// were already registered from higher-fidelity sources.
func (d *diskDiscovery) addExtraFilesystemFolders(folderNames []string) {
	existingMountpoints := make(map[string]bool, len(d.agent.fsStats))
	for _, stats := range d.agent.fsStats {
		existingMountpoints[stats.Mountpoint] = true
	}

	for _, folderName := range folderNames {
		mountpoint := filepath.Join(d.ctx.efPath, folderName)
		slog.Debug("/extra-filesystems", "mountpoint", mountpoint)
		if existingMountpoints[mountpoint] {
			continue
		}
		device, customName := parseFilesystemEntry(folderName)
		d.addFsStat(device, mountpoint, false, customName)
	}
}

// Sets up the filesystems to monitor for disk usage and I/O.
func (a *Agent) initializeDiskInfo() {
	filesystem, _ := utils.GetEnv("FILESYSTEM")
	hasRoot := false
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
	ctx := fsRegistrationContext{
		filesystem:     filesystem,
		isWindows:      isWindows,
		diskIoCounters: diskIoCounters,
		efPath:         "/extra-filesystems",
	}

	// Get the appropriate root mount point for this system
	discovery := diskDiscovery{
		agent:          a,
		rootMountPoint: a.getRootMountPoint(),
		partitions:     partitions,
		usageFn:        disk.Usage,
		ctx:            ctx,
	}

	hasRoot = discovery.addConfiguredRootFs()

	// Add EXTRA_FILESYSTEMS env var values to fsStats
	if extraFilesystems, exists := utils.GetEnv("EXTRA_FILESYSTEMS"); exists {
		discovery.addConfiguredExtraFilesystems(extraFilesystems)
	}

	// Process partitions for various mount points
	for _, p := range partitions {
		if !hasRoot && isRootFallbackPartition(p, discovery.rootMountPoint) {
			hasRoot = discovery.addPartitionRootFs(p.Device, p.Mountpoint)
		}
		discovery.addPartitionExtraFs(p)
	}

	// Check all folders in /extra-filesystems and add them if not already present
	if folders, err := os.ReadDir(discovery.ctx.efPath); err == nil {
		folderNames := make([]string, 0, len(folders))
		for _, folder := range folders {
			if folder.IsDir() {
				folderNames = append(folderNames, folder.Name())
			}
		}
		discovery.addExtraFilesystemFolders(folderNames)
	}

	// If no root filesystem set, try the most active I/O device as a last
	// resort (e.g. ZFS where dataset names are unrelated to disk names).
	if !hasRoot {
		discovery.addLastResortRootFs()
	}

	a.pruneDuplicateRootExtraFilesystems()
	a.initializeDiskIoStats(diskIoCounters)
}

// Removes extra filesystems that mirror root usage (https://github.com/henrygd/beszel/issues/1428).
func (a *Agent) pruneDuplicateRootExtraFilesystems() {
	var rootMountpoint string
	for _, stats := range a.fsStats {
		if stats != nil && stats.Root {
			rootMountpoint = stats.Mountpoint
			break
		}
	}
	if rootMountpoint == "" {
		return
	}
	rootUsage, err := disk.Usage(rootMountpoint)
	if err != nil {
		return
	}
	for name, stats := range a.fsStats {
		if stats == nil || stats.Root {
			continue
		}
		extraUsage, err := disk.Usage(stats.Mountpoint)
		if err != nil {
			continue
		}
		if hasSameDiskUsage(rootUsage, extraUsage) {
			slog.Info("Ignoring duplicate FS", "name", name, "mount", stats.Mountpoint)
			delete(a.fsStats, name)
		}
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
	name := filepath.Base(strings.TrimSpace(value))
	if name == "." {
		return ""
	}
	return name
}

// Sets start values for disk I/O stats.
func (a *Agent) initializeDiskIoStats(diskIoCounters map[string]disk.IOCountersStat) {
	a.fsNames = a.fsNames[:0]
	now := time.Now()
	for device, stats := range a.fsStats {
		// skip if not in diskIoCounters
		d, exists := diskIoCounters[device]
		if !exists {
			slog.Warn("Device not found in diskstats", "name", device)
			continue
		}
		// populate initial values
		stats.Time = now
		stats.TotalRead = d.ReadBytes
		stats.TotalWrite = d.WriteBytes
		// add to list of valid io device names
		a.fsNames = append(a.fsNames, device)
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

	// disk usage
	for _, stats := range a.fsStats {
		// Skip non-root filesystems if caching is active
		if cacheExtraFs && !stats.Root {
			continue
		}
		if d, err := disk.Usage(stats.Mountpoint); err == nil {
			stats.DiskTotal = utils.BytesToGigabytes(d.Total)
			stats.DiskUsed = utils.BytesToGigabytes(d.Used)
			if stats.Root {
				systemStats.DiskTotal = utils.BytesToGigabytes(d.Total)
				systemStats.DiskUsed = utils.BytesToGigabytes(d.Used)
				systemStats.DiskPct = utils.TwoDecimals(d.UsedPercent)
			}
		} else {
			// reset stats if error (likely unmounted)
			slog.Error("Error getting disk stats", "name", stats.Mountpoint, "err", err)
			stats.DiskTotal = 0
			stats.DiskUsed = 0
			stats.TotalRead = 0
			stats.TotalWrite = 0
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
			stats := a.fsStats[d.Name]
			if stats == nil {
				// skip devices not tracked
				continue
			}

			// Previous snapshot for this interval and device
			prev, hasPrev := a.diskPrev[cacheTimeMs][name]
			if !hasPrev {
				// Seed from agent-level fsStats if present, else seed from current
				prev = prevDisk{
					readBytes:  stats.TotalRead,
					writeBytes: stats.TotalWrite,
					readTime:   d.ReadTime,
					writeTime:  d.WriteTime,
					ioTime:     d.IoTime,
					weightedIO: d.WeightedIO,
					readCount:  d.ReadCount,
					writeCount: d.WriteCount,
					at:         stats.Time,
				}
				if prev.at.IsZero() {
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
			diskReadTime := utils.TwoDecimals(float64(d.ReadTime-prev.readTime) / float64(msElapsed) * 100)
			diskWriteTime := utils.TwoDecimals(float64(d.WriteTime-prev.writeTime) / float64(msElapsed) * 100)

			// I/O utilization %: fraction of wall time the device had any I/O in progress (0-100).
			diskIoUtilPct := utils.TwoDecimals(float64(d.IoTime-prev.ioTime) / float64(msElapsed) * 100)

			// Weighted I/O: queue-depth weighted I/O time, normalized to interval (can exceed 100%).
			// Linux kernel field 11: incremented by iops_in_progress × ms_since_last_update.
			// Used to display queue depth. Multipled by 100 to increase accuracy of digit truncation (divided by 100 in UI).
			diskWeightedIO := utils.TwoDecimals(float64(d.WeightedIO-prev.weightedIO) / float64(msElapsed) * 100)

			// r_await / w_await: average time per read/write operation in milliseconds.
			// Equivalent to r_await and w_await in iostat.
			var rAwait, wAwait float64
			if deltaReadCount := d.ReadCount - prev.readCount; deltaReadCount > 0 {
				rAwait = utils.TwoDecimals(float64(d.ReadTime-prev.readTime) / float64(deltaReadCount))
			}
			if deltaWriteCount := d.WriteCount - prev.writeCount; deltaWriteCount > 0 {
				wAwait = utils.TwoDecimals(float64(d.WriteTime-prev.writeTime) / float64(deltaWriteCount))
			}

			// Update global fsStats baseline for cross-interval correctness
			stats.Time = now
			stats.TotalRead = d.ReadBytes
			stats.TotalWrite = d.WriteBytes
			stats.DiskReadPs = readMbPerSecond
			stats.DiskWritePs = writeMbPerSecond
			stats.DiskReadBytes = diskIORead
			stats.DiskWriteBytes = diskIOWrite
			stats.DiskIoStats[0] = diskReadTime
			stats.DiskIoStats[1] = diskWriteTime
			stats.DiskIoStats[2] = diskIoUtilPct
			stats.DiskIoStats[3] = rAwait
			stats.DiskIoStats[4] = wAwait
			stats.DiskIoStats[5] = diskWeightedIO

			if stats.Root {
				systemStats.DiskReadPs = stats.DiskReadPs
				systemStats.DiskWritePs = stats.DiskWritePs
				systemStats.DiskIO[0] = diskIORead
				systemStats.DiskIO[1] = diskIOWrite
				systemStats.DiskIoStats[0] = diskReadTime
				systemStats.DiskIoStats[1] = diskWriteTime
				systemStats.DiskIoStats[2] = diskIoUtilPct
				systemStats.DiskIoStats[3] = rAwait
				systemStats.DiskIoStats[4] = wAwait
				systemStats.DiskIoStats[5] = diskWeightedIO
			}
		}
	}
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
