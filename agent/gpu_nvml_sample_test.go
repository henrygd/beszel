//go:build amd64 && (windows || (linux && glibc))

package agent

import (
	"math"
	"testing"
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
