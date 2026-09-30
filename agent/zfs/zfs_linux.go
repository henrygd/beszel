//go:build linux

// Package zfs provides functions to read ZFS statistics.
package zfs

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	procZfsPath = "/proc/spl/kstat/zfs"
	devZfsPath  = "/dev/zfs"
)

// errNoPoolIOStats marks an "iostats" file that predates OpenZFS 2.3 and
// reports no pool read/write counters (TRIM statistics only), so callers may
// fall back to older counter sources.
var errNoPoolIOStats = errors.New("no pool I/O counters")

func ARCSize() (uint64, error) {
	file, err := os.Open(filepath.Join(procZfsPath, "arcstats"))
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "size") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				return 0, fmt.Errorf("unexpected arcstats size format: %s", line)
			}
			return strconv.ParseUint(fields[2], 10, 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return 0, fmt.Errorf("size field not found in arcstats")
}

// checkZfsDevice lets containers without /dev/zfs fail fast instead of
// waiting for ZFS utility commands to time out.
func checkZfsDevice() error {
	_, err := os.Stat(devZfsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNoZfs
		}
		return err
	}
	return nil
}

// PoolKernelStats reads pool state and cumulative I/O counters directly from
// procfs. These kstats are the same interfaces used by node_exporter's Linux
// ZFS collector and avoid keeping a `zpool iostat` subprocess alive.
func PoolKernelStats() ([]PoolKernelStat, error) {
	poolDirs := make(map[string]struct{})
	for _, filename := range []string{"state", "io", "iostats", "objset-*"} {
		paths, err := filepath.Glob(filepath.Join(procZfsPath, "*", filename))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			poolDirs[filepath.Dir(path)] = struct{}{}
		}
	}
	if len(poolDirs) == 0 {
		return nil, ErrNoZfs
	}
	pools := make([]PoolKernelStat, 0, len(poolDirs))
	for poolDir := range poolDirs {
		nread, nwrite, err := readPoolCounters(poolDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // pool may have been exported after the glob
			}
			return nil, err
		}
		state, err := os.ReadFile(filepath.Join(poolDir, "state"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		pools = append(pools, PoolKernelStat{
			Name: filepath.Base(poolDir), Health: strings.ToUpper(strings.TrimSpace(string(state))),
			NRead: nread, NWrite: nwrite,
		})
	}
	if len(pools) == 0 {
		return nil, ErrNoZfs
	}
	return pools, nil
}

// readPoolCounters supports the ZFS kernel interfaces. OpenZFS 2.3+ exposes
// logical pool read/write counters in "iostats" that cover every objset,
// including mounted snapshots, which never get an "objset-*" kstat. OpenZFS
// through 2.0 exposes aggregate vdev counters in "io". When neither pool-level
// interface is usable, sum the logical I/O counters exposed for each dataset.
func readPoolCounters(poolDir string) (uint64, uint64, error) {
	nread, nwrite, err := readPoolIOStats(filepath.Join(poolDir, "iostats"))
	if err == nil {
		return nread, nwrite, nil
	}
	// Older sources are tried only when "iostats" is absent or predates
	// OpenZFS 2.3. A read or parse failure on a usable file is returned:
	// silently switching counter sources would feed kernelStats counters
	// from different interfaces under the same pool name and produce a
	// false I/O spike once "iostats" recovers.
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errNoPoolIOStats) {
		return 0, 0, err
	}
	nread, nwrite, err = readPoolIO(filepath.Join(poolDir, "io"))
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return nread, nwrite, err
	}
	return readPoolObjsets(poolDir)
}

func readPoolIO(path string) (uint64, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "nread" {
			continue
		}
		if !scanner.Scan() {
			break
		}
		values := strings.Fields(scanner.Text())
		if len(values) < 2 {
			break
		}
		nread, err := strconv.ParseUint(values[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parsing nread in %s: %w", path, err)
		}
		nwrite, err := strconv.ParseUint(values[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parsing nwritten in %s: %w", path, err)
		}
		return nread, nwrite, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return 0, 0, fmt.Errorf("I/O counters not found in %s", path)
}

// readPoolIOStats reads the logical pool I/O counters exported by OpenZFS 2.3+
// in the "iostats" kstat. The file exists on earlier releases but then only
// reports TRIM statistics, so all four byte counters are required for the file
// to be usable.
func readPoolIOStats(path string) (uint64, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	var arcRead, arcWrite, directRead, directWrite uint64
	var found uint8
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		var target *uint64
		var bit uint8
		switch fields[0] {
		case "arc_read_bytes":
			target, bit = &arcRead, 1<<0
		case "direct_read_bytes":
			target, bit = &directRead, 1<<1
		case "arc_write_bytes":
			target, bit = &arcWrite, 1<<2
		case "direct_write_bytes":
			target, bit = &directWrite, 1<<3
		default:
			continue
		}
		value, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parsing %s in %s: %w", fields[0], path, err)
		}
		*target = value
		found |= bit
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if found == 0 {
		return 0, 0, fmt.Errorf("%w in %s", errNoPoolIOStats, path)
	}
	if found != 0x0f {
		return 0, 0, fmt.Errorf("incomplete pool I/O counters in %s", path)
	}
	return arcRead + directRead, arcWrite + directWrite, nil
}

func readPoolObjsets(poolDir string) (uint64, uint64, error) {
	paths, err := filepath.Glob(filepath.Join(poolDir, "objset-*"))
	if err != nil {
		return 0, 0, err
	}
	if len(paths) == 0 {
		return 0, 0, fmt.Errorf("dataset I/O counters not found in %s", poolDir)
	}

	var totalRead, totalWrite uint64
	objsetsRead := 0
	for _, path := range paths {
		nread, nwrite, err := readObjsetIO(path)
		if errors.Is(err, os.ErrNotExist) {
			continue // dataset may have been destroyed after the glob
		}
		if err != nil {
			return 0, 0, err
		}
		totalRead += nread
		totalWrite += nwrite
		objsetsRead++
	}
	if objsetsRead == 0 {
		return 0, 0, fmt.Errorf("dataset I/O counters not found in %s", poolDir)
	}
	return totalRead, totalWrite, nil
}

func readObjsetIO(path string) (uint64, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	var nread, nwrite uint64
	var foundRead, foundWrite bool
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		var target *uint64
		switch fields[0] {
		case "nread":
			target = &nread
			foundRead = true
		case "nwritten":
			target = &nwrite
			foundWrite = true
		default:
			continue
		}
		value, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parsing %s in %s: %w", fields[0], path, err)
		}
		*target = value
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if !foundRead || !foundWrite {
		return 0, 0, fmt.Errorf("incomplete I/O counters in %s", path)
	}
	return nread, nwrite, nil
}
