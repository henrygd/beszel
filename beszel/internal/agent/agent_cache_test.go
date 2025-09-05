//go:build testing
// +build testing

package agent

import (
	"beszel/internal/entities/system"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCache_GetSet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewSessionCache(69 * time.Second)

		testData := &system.CombinedData{
			Info: system.Info{
				Hostname: "test-host",
				Cores:    4,
			},
			Stats: system.Stats{
				Cpu:     50.0,
				MemPct:  30.0,
				DiskPct: 40.0,
			},
		}

		// Test initial state - should not be cached
		data, isCached := cache.Get("session1")
		assert.False(t, isCached, "Expected no cached data initially")
		assert.NotNil(t, data, "Expected data to be initialized")
		// Set data for session1
		cache.Set("session1", testData)

		time.Sleep(15 * time.Second)

		// Get data for a different session - should be cached
		data, isCached = cache.Get("session2")
		assert.True(t, isCached, "Expected data to be cached for non-primary session")
		require.NotNil(t, data, "Expected cached data to be returned")
		assert.Equal(t, "test-host", data.Info.Hostname, "Hostname should match test data")
		assert.Equal(t, 4, data.Info.Cores, "Cores should match test data")
		assert.Equal(t, 50.0, data.Stats.Cpu, "CPU should match test data")
		assert.Equal(t, 30.0, data.Stats.MemPct, "Memory percentage should match test data")
		assert.Equal(t, 40.0, data.Stats.DiskPct, "Disk percentage should match test data")

		time.Sleep(10 * time.Second)

		// Get data for the primary session - should not be cached
		data, isCached = cache.Get("session1")
		assert.False(t, isCached, "Expected data not to be cached for primary session")
		require.NotNil(t, data, "Expected data to be returned even if not cached")
		assert.Equal(t, "test-host", data.Info.Hostname, "Hostname should match test data")
		// if not cached, agent will update the data
		cache.Set("session1", testData)

		time.Sleep(45 * time.Second)

		// Get data for a different session - should still be cached
		_, isCached = cache.Get("session2")
		assert.True(t, isCached, "Expected data to be cached for non-primary session")

		// Wait for the lease to expire
		time.Sleep(30 * time.Second)

		// Get data for session2 - should not be cached
		_, isCached = cache.Get("session2")
		assert.False(t, isCached, "Expected data not to be cached after lease expiration")
	})
}

func TestSessionCache_NilData(t *testing.T) {
	// Create a new SessionCache
	cache := NewSessionCache(30 * time.Second)

	// Test setting nil data (should not panic)
	assert.NotPanics(t, func() {
		cache.Set("session1", nil)
	}, "Setting nil data should not panic")

	// Get data - should not be nil even though we set nil
	data, _ := cache.Get("session2")
	assert.NotNil(t, data, "Expected data to not be nil after setting nil data")
}
