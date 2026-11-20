//go:build linux

package agent

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	perfEventSourceDevices = "/sys/bus/event_source/devices"
	drmClassRoot           = "/sys/class/drm"
	deviceTreeCompatible   = "/proc/device-tree/compatible"
	debugfsRoot            = "/sys/kernel/debug"

	// memBwPMUProbeDelay is how long newly opened PMU counters must count
	// before they are trusted. Some PMUs open fine but never count (the
	// amd_umc PMUs on Strix Halo); real memory traffic is never zero.
	memBwPMUProbeDelay = 100 * time.Millisecond
)

// newMemBandwidthManager picks the first working source: memory controller
// PMUs, then AMD APU gpu_metrics, then the Jetson Orin activity monitor. It
// returns nil when none is available or the agent lacks permission (for
// example in an unprivileged container); the feature is then disabled.
func newMemBandwidthManager() *memBandwidthManager {
	m, err := newPMUMemBandwidthManager()
	if err == nil {
		return m
	}
	slog.Debug("Memory bandwidth PMU unavailable", "err", err)
	if path, err := findAMDAPUGPUMetrics(drmClassRoot); err == nil {
		slog.Debug("Memory bandwidth enabled", "source", path)
		src := &amdAPUDramSource{path: path}
		return newMemBwSamplerManager("amdgpu_gpu_metrics", src.sample)
	}
	if src, err := newTegraActmonSource(deviceTreeCompatible, debugfsRoot); err == nil {
		slog.Debug("Memory bandwidth enabled", "source", src.path)
		return src.manager()
	} else {
		slog.Debug("Memory bandwidth disabled", "err", err)
	}
	return nil
}

// newPMUMemBandwidthManager opens memory controller counters once.
func newPMUMemBandwidthManager() (*memBandwidthManager, error) {
	counters, err := discoverMemBwCounters(perfEventSourceDevices)
	if err != nil {
		return nil, err
	}
	fds, err := openMemBwCounters(counters)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 24) // value, time_enabled, time_running
	read := func() ([]memBwReading, error) {
		counts := make([]memBwReading, len(fds))
		for i, fd := range fds {
			n, err := unix.Read(fd, buf)
			if err != nil || n != len(buf) {
				return nil, fmt.Errorf("read counter %d: %v", i, err)
			}
			counts[i] = memBwReading{
				value:   binary.NativeEndian.Uint64(buf[0:8]),
				enabled: binary.NativeEndian.Uint64(buf[8:16]),
				running: binary.NativeEndian.Uint64(buf[16:24]),
			}
		}
		return counts, nil
	}
	if err := probeMemBwCounters(read, memBwPMUProbeDelay); err != nil {
		closeFds(fds)
		return nil, fmt.Errorf("%s: %w", counters[0].pmu, err)
	}
	slog.Debug("Memory bandwidth enabled", "pmu", counters[0].pmu, "counters", len(counters))
	return &memBandwidthManager{
		counters: counters,
		read:     read,
		now:      time.Now,
		prev:     make(map[uint16]memBwSample),
	}, nil
}

// openMemBwCounters opens every counter system-wide (pid -1) on its PMU's CPU.
// All counters must open; partial coverage would under-report bandwidth.
func openMemBwCounters(counters []memBwCounter) ([]int, error) {
	fds := make([]int, 0, len(counters))
	for _, c := range counters {
		attr := unix.PerfEventAttr{
			Type:        c.pmuType,
			Config:      c.config[0],
			Ext1:        c.config[1],
			Ext2:        c.config[2],
			Read_format: unix.PERF_FORMAT_TOTAL_TIME_ENABLED | unix.PERF_FORMAT_TOTAL_TIME_RUNNING,
		}
		attr.Size = uint32(unsafe.Sizeof(attr))
		fd, err := unix.PerfEventOpen(&attr, -1, c.cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
		if err != nil {
			closeFds(fds)
			return nil, fmt.Errorf("perf_event_open %s cpu %d: %w", c.pmu, c.cpu, err)
		}
		fds = append(fds, fd)
	}
	return fds, nil
}

func closeFds(fds []int) {
	for _, fd := range fds {
		unix.Close(fd)
	}
}
