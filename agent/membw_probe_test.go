//go:build membwprobe

package agent

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMemBwLiveProbe prints live system memory bandwidth from the real source
// selection. Manual tool for validating a host without installing an agent:
//
//	go test -c -tags membwprobe -o membw-probe ./agent
//	MEMBW_PROBE_SECONDS=20 ./membw-probe -test.run TestMemBwLiveProbe -test.v
func TestMemBwLiveProbe(t *testing.T) {
	seconds, _ := strconv.Atoi(os.Getenv("MEMBW_PROBE_SECONDS"))
	if seconds <= 0 {
		seconds = 10
	}
	start := time.Now()
	m := newMemBandwidthManager()
	t.Logf("init took %v", time.Since(start))
	if m == nil {
		t.Fatal("no memory bandwidth source")
	}
	t.Logf("source=%s counters=%d", m.counters[0].pmu, len(m.counters))
	m.get(1)
	for range seconds {
		time.Sleep(time.Second)
		bw, ok := m.get(1)
		t.Logf("%s read=%.2f write=%.2f total=%.2f GB/s ok=%v", time.Now().Format("15:04:05"), bw[0], bw[1], bw[0]+bw[1], ok)
	}
}
