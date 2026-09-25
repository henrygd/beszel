//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
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
