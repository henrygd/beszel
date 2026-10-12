//go:build testing && amd64 && (windows || (linux && glibc))

package agent

import (
	"math"
	"testing"
	"unsafe"
)

func TestNvmlSampleToFloat(t *testing.T) {
	tests := []struct {
		valType int32
		raw     uint64
		want    float64
		ok      bool
	}{
		{0, math.Float64bits(55.5), 55.5, true},
		{1, 0xdeadbeef_0000d878, 55416, true}, // upper union bytes ignored for uint32
		{3, 115000, 115000, true},
		{4, uint64(1<<64 - 5), -5, true},
		{5, 0x00000000_fffffffb, -5, true},
		{6, 0xffff_0064, 100, true},
		{99, 1, 0, false},
	}
	for _, tt := range tests {
		got, ok := nvmlSampleToFloat(tt.valType, tt.raw)
		if got != tt.want || ok != tt.ok {
			t.Errorf("nvmlSampleToFloat(%d, %#x) = %v, %v; want %v, %v", tt.valType, tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPowerFromSamples(t *testing.T) {
	buf := []nvmlSample{{100, 110000}, {200, 120000}, {300, 130000}}
	var seenTs []uint64
	nvmlDeviceGetSamples = func(_ nvmlDevice, _ int, last uint64, valType *int32, count *uint32, samples *nvmlSample) nvmlReturn {
		seenTs = append(seenTs, last)
		var out []nvmlSample
		for _, s := range buf {
			if s.TimeStamp > last {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nvmlErrorNotFound
		}
		*valType = 1 // NVML_VALUE_TYPE_UNSIGNED_INT
		if samples == nil {
			*count = 120 // driver buffer size
			return nvmlReturn(nvmlSuccess)
		}
		*count = uint32(copy(unsafe.Slice(samples, *count), out))
		return nvmlReturn(nvmlSuccess)
	}
	defer func() { nvmlDeviceGetSamples = nil }()

	c := &nvmlCollector{lastSampleTs: []uint64{0}, lastPower: []float64{0}}

	// first poll averages the whole buffer
	if got := c.powerFromSamples(0, 0); got != 120 {
		t.Fatalf("first poll = %v; want 120", got)
	}
	// no new samples reuses the last value
	if got := c.powerFromSamples(0, 0); got != 120 {
		t.Fatalf("poll without new samples = %v; want 120", got)
	}
	// only samples newer than the last seen timestamp are averaged
	buf = append(buf, nvmlSample{400, 50000})
	if got := c.powerFromSamples(0, 0); got != 50 {
		t.Fatalf("poll with new sample = %v; want 50", got)
	}

	wantTs := []uint64{0, 0, 300, 300, 300}
	if len(seenTs) != len(wantTs) {
		t.Fatalf("lastSeenTimeStamp args = %v; want %v", seenTs, wantTs)
	}
	for i := range wantTs {
		if seenTs[i] != wantTs[i] {
			t.Fatalf("lastSeenTimeStamp args = %v; want %v", seenTs, wantTs)
		}
	}
	if c.lastSampleTs[0] != 400 {
		t.Fatalf("lastSampleTs = %d; want 400", c.lastSampleTs[0])
	}
}

func TestPowerFromSamplesUnavailable(t *testing.T) {
	nvmlDeviceGetSamples = nil
	c := &nvmlCollector{lastSampleTs: []uint64{0}, lastPower: []float64{0}}
	if got := c.powerFromSamples(0, 0); got != 0 {
		t.Fatalf("powerFromSamples without symbol = %v; want 0", got)
	}
}
