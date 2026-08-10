// Package zfs provides functions to read ZFS statistics.
package zfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

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

// PoolIoStats holds per-second I/O rates for a pool from `zpool iostat`.
type PoolIoStats struct {
	NRead  uint64 // bytes/s read
	NWrite uint64 // bytes/s written
}

var zpoolIostatRowRe = regexp.MustCompile(`^(\S+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*$`)

// PoolIoWatcher streams per-pool I/O rates from a long-running
// `zpool iostat -p 1` process and keeps the most recent per-second sample.
// It restarts itself if the process dies (e.g. after a pool export) until
// stopped.
type PoolIoWatcher struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	latest  map[string]PoolIoStats
	last    time.Time
	stopped bool
}

// NewPoolIoWatcher starts the streaming `zpool iostat -p 1` process.
func NewPoolIoWatcher() (*PoolIoWatcher, error) {
	w := &PoolIoWatcher{latest: make(map[string]PoolIoStats)}
	if err := w.start(); err != nil {
		return nil, err
	}
	return w, nil
}

// start spawns the iostat process and begins reading its output. No-op once
// stopped.
func (w *PoolIoWatcher) start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return nil
	}
	cmd := exec.Command("zpool", "iostat", "-p", "1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cmd = cmd
	go w.readLoop(stdout)
	return nil
}

// readLoop consumes the iostat stream until it ends, then restarts unless
// stopped. A failed restart is retried on a short cadence so transient spawn
// failures do not kill pool I/O permanently. When the agent process exits, the
// zpool child receives SIGPIPE on the closed pipe and terminates on its own.
func (w *PoolIoWatcher) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		w.handleLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		slog.Debug("ZFS pool I/O watcher stream ended", "err", err)
	}
	for {
		w.mu.Lock()
		stopped := w.stopped
		w.mu.Unlock()
		if stopped {
			return
		}
		time.Sleep(time.Second)
		if err := w.start(); err != nil {
			slog.Debug("ZFS pool I/O watcher restart failed", "err", err)
			continue
		}
		return
	}
}

// handleLine ingests one line of `zpool iostat -p` output. Only pool rows
// (7 whitespace-separated numeric columns) match; header and separator lines
// are ignored. Later rows overwrite earlier ones, so `latest` converges to the
// per-second interval rates.
func (w *PoolIoWatcher) handleLine(line string) {
	m := zpoolIostatRowRe.FindStringSubmatch(strings.TrimRight(line, " "))
	if m == nil {
		return
	}
	read, err1 := strconv.ParseUint(m[6], 10, 64)
	write, err2 := strconv.ParseUint(m[7], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	w.mu.Lock()
	w.latest[m[1]] = PoolIoStats{NRead: read, NWrite: write}
	w.last = time.Now()
	w.mu.Unlock()
}

// Latest returns a copy of the most recent per-pool rates, or ok=false when no
// sample has arrived within the last 10 seconds.
func (w *PoolIoWatcher) Latest() (map[string]PoolIoStats, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.latest) == 0 || time.Since(w.last) > 10*time.Second {
		return nil, false
	}
	out := make(map[string]PoolIoStats, len(w.latest))
	for name, io := range w.latest {
		out[name] = io
	}
	return out, true
}

// Stop terminates the iostat process and disables restarts.
func (w *PoolIoWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}

// Dataset is a single ZFS dataset with usage information.
type Dataset struct {
	Name       string
	Used       uint64
	Avail      uint64
	Mountpoint string
}

// PoolStats returns capacity and health for all pools on the system using
// `zpool list`. The same source node_exporter uses for zfs_pool_healthy.
func PoolStats() ([]PoolStat, error) {
	out, err := exec.Command("zpool", "list", "-Hp", "-o", "name,size,alloc,free,health").Output()
	if err != nil {
		return nil, fmt.Errorf("zpool list: %w", err)
	}
	return parseZpoolListOutput(out)
}

// Datasets returns all datasets on the system with usage and mountpoint
// information using `zfs list` (recursive by default).
func Datasets() ([]Dataset, error) {
	out, err := exec.Command("zfs", "list", "-Hp", "-o", "name,used,avail,mountpoint").Output()
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
