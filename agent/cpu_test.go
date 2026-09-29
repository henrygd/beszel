//go:build testing

package agent

import (
	"runtime"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
)

func TestGetAllBusy(t *testing.T) {
	times := cpu.TimesStat{
		User: 1, System: 2, Idle: 3, Nice: 4, Iowait: 5,
		Irq: 6, Softirq: 7, Steal: 8, Guest: 9, GuestNice: 10,
	}
	wantTotal, wantBusy := 55.0, 47.0
	if runtime.GOOS == "linux" {
		wantTotal, wantBusy = 36, 28
	}
	total, busy := getAllBusy(times)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, wantBusy, busy)
	assert.InDelta(t, wantBusy/wantTotal*100, calculateBusy(cpu.TimesStat{}, times), 1e-10)
	assert.Zero(t, calculateBusy(times, times))
	assert.Zero(t, calculateBusy(times, cpu.TimesStat{}))
}
