package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcessCountsCache(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			var cache processCountsCache
			now := time.Now()
			calls := 0
			var wantErr error
			if fail {
				wantErr = errors.New("scan failed")
			}
			collect := func() ([5]uint32, error) {
				calls++
				return [5]uint32{uint32(calls)}, wantErr
			}
			counts, err := cache.get(now, collect)
			require.Equal(t, wantErr, err)
			require.Equal(t, uint32(1), counts[0])
			for _, elapsed := range []time.Duration{0, time.Second, processCountsCacheDuration - time.Nanosecond} {
				counts, err = cache.get(now.Add(elapsed), collect)
				require.Equal(t, wantErr, err)
				require.Equal(t, uint32(1), counts[0])
			}
			require.Equal(t, 1, calls)
			counts, err = cache.get(now.Add(processCountsCacheDuration), collect)
			require.Equal(t, wantErr, err)
			require.Equal(t, uint32(2), counts[0])
			require.Equal(t, 2, calls)
		})
	}
}

func BenchmarkProcessCounts(b *testing.B) {
	b.Run("fresh", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := getProcessCounts(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		var cache processCountsCache
		if _, err := cache.get(time.Now(), getProcessCounts); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := cache.get(time.Now(), getProcessCounts); err != nil {
				b.Fatal(err)
			}
		}
	})
}
