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

var procZfsPath = "/proc/spl/kstat/zfs"

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

// PoolKernelStats reads pool state and cumulative I/O counters directly from
// procfs. These kstats are the same interfaces used by node_exporter's Linux
// ZFS collector and avoid keeping a `zpool iostat` subprocess alive.
func PoolKernelStats() ([]PoolKernelStat, error) {
	poolDirs := make(map[string]struct{})
	for _, filename := range []string{"state", "io", "objset-*"} {
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

// readPoolCounters supports both ZFS kernel interfaces. OpenZFS through 2.3
// exposes aggregate vdev counters in "io". When that file is unavailable, sum
// the logical I/O counters exposed for each dataset in the pool.
func readPoolCounters(poolDir string) (uint64, uint64, error) {
	nread, nwrite, err := readPoolIO(filepath.Join(poolDir, "io"))
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
