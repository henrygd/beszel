//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Linux prints four millisecond fields of /proc/diskstats as 32-bit unsigned ints:
// read time, write time, io time and weighted io time. They wrap to zero at 2^32.
func TestUpdateDiskIoTimeCounterWrap(t *testing.T) {
	const wrap = uint64(1) << 32

	tests := []struct {
		name string
		base uint64 // added to every previous time counter
	}{
		{"no wrap", 0},
		{"32-bit wrap", wrap - 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Deltas over 60s: read 300ms / 10 ops, write 400ms / 20 ops,
			// io time 1200ms, weighted io 3000ms.
			prev := prevDisk{
				readBytes:  20000 * 512,
				writeBytes: 10000 * 512,
				readTime:   tt.base + 900,
				writeTime:  tt.base + 700,
				ioTime:     tt.base + 400,
				weightedIO: tt.base,
				readCount:  1000,
				writeCount: 500,
				at:         time.Now().Add(-60 * time.Second),
			}
			cur := func(v uint64) uint64 { return v % wrap }
			line := fmt.Sprintf("   8       0 sda %d 0 %d %d %d 0 %d %d 0 %d %d\n",
				1010, 21200, cur(prev.readTime+300),
				520, 10400, cur(prev.writeTime+400),
				cur(prev.ioTime+1200), cur(prev.weightedIO+3000))

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "diskstats"), []byte(line), 0o644))
			t.Setenv("HOST_PROC", dir)
			t.Setenv("HOST_SYS", dir)
			t.Setenv("HOST_DEV", dir)
			t.Setenv("HOST_RUN", dir)

			fs := &system.FsStats{Root: true}
			a := &Agent{
				fsNames:  []string{"sda"},
				fsStats:  map[string]*system.FsStats{"sda": fs},
				diskPrev: map[uint16]map[string]prevDisk{60000: {"sda": prev}},
			}
			var stats system.Stats
			a.updateDiskIo(60000, &stats)

			// Same order as DiskIoStats in system.FsStats.
			want := [6]float64{0.5, 0.67, 2, 30, 20, 5}
			for i := range want {
				assert.InDelta(t, want[i], fs.DiskIoStats[i], 0.01, "DiskIoStats[%d]", i)
				assert.InDelta(t, want[i], stats.DiskIoStats[i], 0.01, "system DiskIoStats[%d]", i)
			}
		})
	}
}

// The first sample of a cache interval has no snapshot of its own. It must
// measure the time counters from the same baseline as the byte counters.
func TestUpdateDiskIoFirstSampleOfInterval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOST_PROC", dir)
	t.Setenv("HOST_SYS", dir)
	t.Setenv("HOST_DEV", dir)
	t.Setenv("HOST_RUN", dir)
	writeDiskstats := func(line string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "diskstats"), []byte(line), 0o644))
	}

	writeDiskstats("   8       0 sda 1000 0 20000 900 500 0 10000 700 0 400 0\n")
	counters, err := disk.IOCounters("sda")
	require.NoError(t, err)

	fs := &system.FsStats{Root: true}
	a := &Agent{
		fsStats:  map[string]*system.FsStats{"sda": fs},
		diskPrev: map[uint16]map[string]prevDisk{},
	}
	a.initializeDiskIoStats(counters)

	// updateDiskIo skips samples less than 100ms apart.
	time.Sleep(150 * time.Millisecond)

	// Deltas: read 300ms / 10 ops, write 400ms / 20 ops, io time 1200ms, weighted io 3000ms.
	writeDiskstats("   8       0 sda 1010 0 21200 1200 520 0 10400 1100 0 1600 3000\n")
	var stats system.Stats
	a.updateDiskIo(60000, &stats)

	require.NotZero(t, fs.DiskReadBytes, "bytes are measured from the baseline")
	for i := range 3 {
		assert.NotZero(t, fs.DiskIoStats[i], "DiskIoStats[%d]", i)
	}
	assert.InDelta(t, 30, fs.DiskIoStats[3], 0.01, "r_await")
	assert.InDelta(t, 20, fs.DiskIoStats[4], 0.01, "w_await")
	assert.NotZero(t, fs.DiskIoStats[5], "weighted io")

	// A second interval starts from the latest counters, not from the ones at start.
	time.Sleep(150 * time.Millisecond)
	// Deltas: read 100ms / 10 ops, write 100ms / 20 ops.
	writeDiskstats("   8       0 sda 1020 0 22400 1300 540 0 10800 1200 0 1800 3500\n")
	a.updateDiskIo(1000, &stats)

	assert.InDelta(t, 10, fs.DiskIoStats[3], 0.01, "r_await")
	assert.InDelta(t, 5, fs.DiskIoStats[4], 0.01, "w_await")
}
