//go:build testing
// +build testing

package agent

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestCacheData() *system.CombinedData {
	return &system.CombinedData{
		Stats: system.Stats{
			Cpu:       50.5,
			Mem:       8192,
			DiskTotal: 100000,
		},
		Info: system.Info{
			AgentVersion: "0.12.0",
		},
		Containers: []*container.Stats{
			{
				Name: "test-container",
				Cpu:  25.0,
			},
		},
	}
}

func TestNewSystemDataCache(t *testing.T) {
	cache := NewSystemDataCache()
	require.NotNil(t, cache)
	assert.NotNil(t, cache.cache)
	assert.Empty(t, cache.cache)
}

func TestCacheGetSet(t *testing.T) {
	cache := NewSystemDataCache()
	data := createTestCacheData()

	// Test setting data
	cache.Set(data, 1000) // 1 second cache

	// Test getting fresh data
	retrieved, isCached := cache.Get(1000)
	assert.True(t, isCached)
	assert.Equal(t, data, retrieved)

	// Test getting non-existent cache key
	_, isCached = cache.Get(2000)
	assert.False(t, isCached)
}

func TestCacheFreshness(t *testing.T) {
	cache := NewSystemDataCache()
	data := createTestCacheData()

	testCases := []struct {
		name        string
		cacheTimeMs uint16
		sleepMs     time.Duration
		expectFresh bool
	}{
		{
			name:        "fresh data - well within cache time",
			cacheTimeMs: 1000, // 1 second
			sleepMs:     100,  // 100ms
			expectFresh: true,
		},
		{
			name:        "fresh data - at 50% of cache time boundary",
			cacheTimeMs: 1000, // 1 second, 50% = 500ms
			sleepMs:     499,  // just under 500ms
			expectFresh: true,
		},
		{
			name:        "stale data - exactly at 50% cache time",
			cacheTimeMs: 1000, // 1 second, 50% = 500ms
			sleepMs:     500,  // exactly 500ms
			expectFresh: false,
		},
		{
			name:        "stale data - well beyond cache time",
			cacheTimeMs: 1000, // 1 second
			sleepMs:     800,  // 800ms
			expectFresh: false,
		},
		{
			name:        "short cache time",
			cacheTimeMs: 200, // 200ms, 50% = 100ms
			sleepMs:     150, // 150ms > 100ms
			expectFresh: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Set data
				cache.Set(data, tc.cacheTimeMs)

				// Wait for the specified duration
				if tc.sleepMs > 0 {
					time.Sleep(tc.sleepMs * time.Millisecond)
				}

				// Check freshness
				_, isCached := cache.Get(tc.cacheTimeMs)
				assert.Equal(t, tc.expectFresh, isCached)
			})
		})
	}
}

func TestCacheMultipleIntervals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewSystemDataCache()
		data1 := createTestCacheData()
		data2 := &system.CombinedData{
			Stats: system.Stats{
				Cpu: 75.0,
				Mem: 16384,
			},
			Info: system.Info{
				AgentVersion: "0.12.0",
			},
			Containers: []*container.Stats{},
		}

		// Set data for different intervals
		cache.Set(data1, 500)  // 500ms cache
		cache.Set(data2, 1000) // 1000ms cache

		// Both should be fresh immediately
		retrieved1, isCached1 := cache.Get(500)
		assert.True(t, isCached1)
		assert.Equal(t, data1, retrieved1)

		retrieved2, isCached2 := cache.Get(1000)
		assert.True(t, isCached2)
		assert.Equal(t, data2, retrieved2)

		// Wait 300ms - 500ms cache should be stale (250ms threshold), 1000ms should still be fresh (500ms threshold)
		time.Sleep(300 * time.Millisecond)

		_, isCached1 = cache.Get(500)
		assert.False(t, isCached1)

		_, isCached2 = cache.Get(1000)
		assert.True(t, isCached2)

		// Wait another 300ms (total 600ms) - now 1000ms cache should also be stale
		time.Sleep(300 * time.Millisecond)
		_, isCached2 = cache.Get(1000)
		assert.False(t, isCached2)
	})
}

func TestCacheOverwrite(t *testing.T) {
	cache := NewSystemDataCache()
	data1 := createTestCacheData()
	data2 := &system.CombinedData{
		Stats: system.Stats{
			Cpu: 90.0,
			Mem: 32768,
		},
		Info: system.Info{
			AgentVersion: "0.12.0",
		},
		Containers: []*container.Stats{},
	}

	// Set initial data
	cache.Set(data1, 1000)
	retrieved, isCached := cache.Get(1000)
	assert.True(t, isCached)
	assert.Equal(t, data1, retrieved)

	// Overwrite with new data
	cache.Set(data2, 1000)
	retrieved, isCached = cache.Get(1000)
	assert.True(t, isCached)
	assert.Equal(t, data2, retrieved)
	assert.NotEqual(t, data1, retrieved)
}

func TestCacheMiss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewSystemDataCache()

		// Test getting from empty cache
		_, isCached := cache.Get(1000)
		assert.False(t, isCached)

		// Set data for one interval
		data := createTestCacheData()
		cache.Set(data, 1000)

		// Test getting different interval
		_, isCached = cache.Get(2000)
		assert.False(t, isCached)

		// Test getting after data has expired
		time.Sleep(600 * time.Millisecond) // 600ms > 500ms (50% of 1000ms)
		_, isCached = cache.Get(1000)
		assert.False(t, isCached)
	})
}

func TestCacheZeroInterval(t *testing.T) {
	cache := NewSystemDataCache()
	data := createTestCacheData()

	// Set with zero interval - should allow immediate cache
	cache.Set(data, 0)

	// With 0 interval, 50% is 0, so it should never be considered fresh
	// (time.Since(lastUpdate) >= 0, which is not < 0)
	_, isCached := cache.Get(0)
	assert.False(t, isCached)
}

func TestCacheLargeInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewSystemDataCache()
		data := createTestCacheData()

		// Test with maximum uint16 value
		cache.Set(data, 65535) // ~65 seconds

		// Should be fresh immediately
		_, isCached := cache.Get(65535)
		assert.True(t, isCached)

		// Should still be fresh after a short time
		time.Sleep(100 * time.Millisecond)
		_, isCached = cache.Get(65535)
		assert.True(t, isCached)
	})
}
