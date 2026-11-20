//go:build testing && amd64 && (windows || (linux && glibc))

package agent

import (
	"math"
	"testing"
)

func TestMemBandwidthRejectsNonFiniteGpm(t *testing.T) {
	value := 0.0
	nvmlGpmSampleGet = func(_ nvmlDevice, _ nvmlGpmSample) nvmlReturn { return nvmlReturn(nvmlSuccess) }
	nvmlGpmMetricsGetFn = func(m *nvmlGpmMetricsGet) nvmlReturn {
		m.Metrics[0].NvmlReturn = int32(nvmlSuccess)
		m.Metrics[0].Value = value
		return nvmlReturn(nvmlSuccess)
	}
	nvmlGpmSampleFree = func(_ nvmlGpmSample) nvmlReturn { return nvmlReturn(nvmlSuccess) }
	defer func() { nvmlGpmSampleGet, nvmlGpmMetricsGetFn, nvmlGpmSampleFree = nil, nil, nil }()

	c := &nvmlCollector{memBw: []nvmlMemBw{{detected: true, peak: 1000, gpm: true, samples: [2]nvmlGpmSample{1, 2}, hasPrev: true}}}

	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		value = bad
		c.memBw[0].hasPrev = true
		c.memBw[0].failures = 0
		if bw, _, ok := c.memBandwidth(0, 0, 0); ok || bw != 0 {
			t.Fatalf("value %v: got bw=%v ok=%v, want rejected", bad, bw, ok)
		}
		if c.memBw[0].failures != 1 || c.memBw[0].hasPrev {
			t.Fatalf("value %v: failures=%d hasPrev=%v, want 1/false", bad, c.memBw[0].failures, c.memBw[0].hasPrev)
		}
	}

	value = 50
	c.memBw[0].hasPrev = true
	bw, approx, ok := c.memBandwidth(0, 0, 0)
	if !ok || approx || bw != gpuMemBandwidthFromUtil(50, 1000) || c.memBw[0].failures != 0 {
		t.Fatalf("finite value: got bw=%v approx=%v ok=%v failures=%d", bw, approx, ok, c.memBw[0].failures)
	}
}
