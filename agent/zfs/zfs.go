// Package zfs provides functions to read ZFS statistics.
package zfs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var commandTimeout = 10 * time.Second

var commandOutput = func(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s: %w", name, commandTimeout, ctx.Err())
	}
	return out, err
}

// ErrNoZfs is returned when the ZFS utilities or kernel interfaces are unavailable.
var ErrNoZfs = errors.New("zfs utilities unavailable")

// PoolStat is a snapshot of a ZFS pool's capacity and health.
type PoolStat struct {
	Name   string
	Size   uint64 // total capacity in bytes
	Alloc  uint64 // allocated bytes
	Free   uint64 // free bytes
	Health string // ONLINE, DEGRADED, FAULTED, ...
}

// PoolKernelStat is the inexpensive pool telemetry exposed by the ZFS kernel.
// NRead and NWrite are cumulative byte counters since the pool was imported.
type PoolKernelStat struct {
	Name   string
	Health string
	NRead  uint64
	NWrite uint64
}

// PoolIoStats holds calculated per-second I/O rates for a pool.
type PoolIoStats struct {
	NRead  uint64
	NWrite uint64
}

// Dataset is a single ZFS dataset with usage information.
type Dataset struct {
	Name       string
	Used       uint64
	Avail      uint64
	Mountpoint string
}

// PoolStats returns capacity and health for all pools on the system using
// `zpool list`. Frequent health and I/O sampling uses PoolKernelStats instead.
func PoolStats() ([]PoolStat, error) {
	out, err := commandOutput("zpool", "list", "-Hp", "-o", "name,size,alloc,free,health")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && strings.Contains(string(exitErr.Stderr), "no pools available") {
			return nil, nil
		}
		return nil, fmt.Errorf("zpool list: %w", err)
	}
	return parseZpoolListOutput(out)
}

// Datasets returns all datasets on the system with usage and mountpoint
// information using `zfs list` (recursive by default).
func Datasets() ([]Dataset, error) {
	out, err := commandOutput("zfs", "list", "-Hp", "-o", "name,used,avail,mountpoint")
	if err != nil {
		return nil, fmt.Errorf("zfs list: %w", err)
	}
	return parseZfsListOutput(out)
}

// parseZpoolListOutput parses `zpool list -Hp -o name,size,alloc,free,health` output.
// Columns are tab-separated; numeric columns are raw bytes.
func parseZpoolListOutput(out []byte) ([]PoolStat, error) {
	var pools []PoolStat
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "no pools available" && len(pools) == 0 {
			return nil, nil
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			return nil, fmt.Errorf("unexpected zpool list line: %q", line)
		}
		size, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing size for pool %q: %w", fields[0], err)
		}
		alloc, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing alloc for pool %q: %w", fields[0], err)
		}
		free, err := strconv.ParseUint(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing free for pool %q: %w", fields[0], err)
		}
		pools = append(pools, PoolStat{
			Name:   fields[0],
			Size:   size,
			Alloc:  alloc,
			Free:   free,
			Health: fields[4],
		})
	}
	return pools, scanner.Err()
}

// parseZfsListOutput parses `zfs list -Hp -o name,used,avail,mountpoint` output.
// The mountpoint column may contain spaces, so it is split on tabs only.
func parseZfsListOutput(out []byte) ([]Dataset, error) {
	var datasets []Dataset
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 4 {
			return nil, fmt.Errorf("unexpected zfs list line: %q", line)
		}
		used, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing used for dataset %q: %w", fields[0], err)
		}
		avail, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing avail for dataset %q: %w", fields[0], err)
		}
		datasets = append(datasets, Dataset{
			Name:       fields[0],
			Used:       used,
			Avail:      avail,
			Mountpoint: fields[3],
		})
	}
	return datasets, scanner.Err()
}
