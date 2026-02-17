//go:build testing
// +build testing

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultCacheTimeMs = uint16(60_000)

// cycleCpuDeltas cycles the CPU tracking data for a specific cache time interval
func (dm *dockerManager) cycleCpuDeltas(cacheTimeMs uint16) {
	// Clear the CPU tracking maps for this cache time interval
	if dm.lastCpuContainer[cacheTimeMs] != nil {
		clear(dm.lastCpuContainer[cacheTimeMs])
	}
	if dm.lastCpuSystem[cacheTimeMs] != nil {
		clear(dm.lastCpuSystem[cacheTimeMs])
	}
}

func TestCalculateMemoryUsage(t *testing.T) {
	tests := []struct {
		name        string
		apiStats    *container.ApiStats
		isWindows   bool
		expected    uint64
		expectError bool
	}{
		{
			name: "Linux with valid memory stats",
			apiStats: &container.ApiStats{
				MemoryStats: container.MemoryStats{
					Usage: 1048576, // 1MB
					Stats: container.MemoryStatsStats{
						Cache:        524288, // 512KB
						InactiveFile: 262144, // 256KB
					},
				},
			},
			isWindows:   false,
			expected:    786432, // 1MB - 256KB (inactive_file takes precedence) = 768KB
			expectError: false,
		},
		{
			name: "Linux with zero cache uses inactive_file",
			apiStats: &container.ApiStats{
				MemoryStats: container.MemoryStats{
					Usage: 1048576, // 1MB
					Stats: container.MemoryStatsStats{
						Cache:        0,
						InactiveFile: 262144, // 256KB
					},
				},
			},
			isWindows:   false,
			expected:    786432, // 1MB - 256KB = 768KB
			expectError: false,
		},
		{
			name: "Windows with valid memory stats",
			apiStats: &container.ApiStats{
				MemoryStats: container.MemoryStats{
					PrivateWorkingSet: 524288, // 512KB
				},
			},
			isWindows:   true,
			expected:    524288,
			expectError: false,
		},
		{
			name: "Linux with zero usage returns error",
			apiStats: &container.ApiStats{
				MemoryStats: container.MemoryStats{
					Usage: 0,
					Stats: container.MemoryStatsStats{
						Cache:        0,
						InactiveFile: 0,
					},
				},
			},
			isWindows:   false,
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := calculateMemoryUsage(tt.apiStats, tt.isWindows)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateCpuPercentage(t *testing.T) {
	tests := []struct {
		name          string
		cpuPct        float64
		containerName string
		expectError   bool
		expectedError string
	}{
		{
			name:          "valid CPU percentage",
			cpuPct:        50.5,
			containerName: "test-container",
			expectError:   false,
		},
		{
			name:          "zero CPU percentage",
			cpuPct:        0.0,
			containerName: "test-container",
			expectError:   false,
		},
		{
			name:          "CPU percentage over 100",
			cpuPct:        150.5,
			containerName: "test-container",
			expectError:   true,
			expectedError: "test-container cpu pct greater than 100: 150.5",
		},
		{
			name:          "CPU percentage exactly 100",
			cpuPct:        100.0,
			containerName: "test-container",
			expectError:   false,
		},
		{
			name:          "negative CPU percentage",
			cpuPct:        -10.0,
			containerName: "test-container",
			expectError:   false, // Function only checks for > 100, not negative
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCpuPercentage(tt.cpuPct, tt.containerName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateContainerStatsValues(t *testing.T) {
	stats := &container.Stats{
		Name:         "test-container",
		Cpu:          0.0,
		Mem:          0.0,
		NetworkSent:  0.0,
		NetworkRecv:  0.0,
		PrevReadTime: time.Time{},
	}

	testTime := time.Now()
	updateContainerStatsValues(stats, 75.5, 1048576, 524288, 262144, testTime)

	// Check CPU percentage (should be rounded to 2 decimals)
	assert.Equal(t, 75.5, stats.Cpu)

	// Check memory (should be converted to MB: 1048576 bytes = 1 MB)
	assert.Equal(t, 1.0, stats.Mem)

	// Check bandwidth (raw bytes)
	assert.Equal(t, [2]uint64{524288, 262144}, stats.Bandwidth)

	// Deprecated fields still populated for backward compatibility with older hubs
	assert.Equal(t, 0.5, stats.NetworkSent)  // 524288 bytes = 0.5 MB
	assert.Equal(t, 0.25, stats.NetworkRecv) // 262144 bytes = 0.25 MB

	// Check read time
	assert.Equal(t, testTime, stats.PrevReadTime)
}

func TestTwoDecimals(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"round down", 1.234, 1.23},
		{"round half up", 1.235, 1.24}, // math.Round rounds half up
		{"no rounding needed", 1.23, 1.23},
		{"negative number", -1.235, -1.24}, // math.Round rounds half up (more negative)
		{"zero", 0.0, 0.0},
		{"large number", 123.456, 123.46}, // rounds 5 up
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := twoDecimals(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBytesToMegabytes(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"1 MB", 1048576, 1.0},
		{"512 KB", 524288, 0.5},
		{"zero", 0, 0},
		{"large value", 1073741824, 1024}, // 1 GB = 1024 MB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesToMegabytes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitializeCpuTracking(t *testing.T) {
	dm := &dockerManager{
		lastCpuContainer: make(map[uint16]map[string]uint64),
		lastCpuSystem:    make(map[uint16]map[string]uint64),
		lastCpuReadTime:  make(map[uint16]map[string]time.Time),
	}

	cacheTimeMs := uint16(30000)

	// Test initializing a new cache time
	dm.initializeCpuTracking(cacheTimeMs)

	// Check that maps were created
	assert.NotNil(t, dm.lastCpuContainer[cacheTimeMs])
	assert.NotNil(t, dm.lastCpuSystem[cacheTimeMs])
	assert.NotNil(t, dm.lastCpuReadTime[cacheTimeMs])
	assert.Empty(t, dm.lastCpuContainer[cacheTimeMs])
	assert.Empty(t, dm.lastCpuSystem[cacheTimeMs])

	// Test initializing existing cache time (should not overwrite)
	dm.lastCpuContainer[cacheTimeMs]["test"] = 100
	dm.lastCpuSystem[cacheTimeMs]["test"] = 200

	dm.initializeCpuTracking(cacheTimeMs)

	// Should still have the existing values
	assert.Equal(t, uint64(100), dm.lastCpuContainer[cacheTimeMs]["test"])
	assert.Equal(t, uint64(200), dm.lastCpuSystem[cacheTimeMs]["test"])
}

func TestGetCpuPreviousValues(t *testing.T) {
	dm := &dockerManager{
		lastCpuContainer: map[uint16]map[string]uint64{
			30000: {"container1": 100, "container2": 200},
		},
		lastCpuSystem: map[uint16]map[string]uint64{
			30000: {"container1": 150, "container2": 250},
		},
	}

	// Test getting existing values
	container, system := dm.getCpuPreviousValues(30000, "container1")
	assert.Equal(t, uint64(100), container)
	assert.Equal(t, uint64(150), system)

	// Test getting non-existing container
	container, system = dm.getCpuPreviousValues(30000, "nonexistent")
	assert.Equal(t, uint64(0), container)
	assert.Equal(t, uint64(0), system)

	// Test getting non-existing cache time
	container, system = dm.getCpuPreviousValues(60000, "container1")
	assert.Equal(t, uint64(0), container)
	assert.Equal(t, uint64(0), system)
}

func TestSetCpuCurrentValues(t *testing.T) {
	dm := &dockerManager{
		lastCpuContainer: make(map[uint16]map[string]uint64),
		lastCpuSystem:    make(map[uint16]map[string]uint64),
	}

	cacheTimeMs := uint16(30000)
	containerId := "test-container"

	// Initialize the cache time maps first
	dm.initializeCpuTracking(cacheTimeMs)

	// Set values
	dm.setCpuCurrentValues(cacheTimeMs, containerId, 500, 750)

	// Check that values were set
	assert.Equal(t, uint64(500), dm.lastCpuContainer[cacheTimeMs][containerId])
	assert.Equal(t, uint64(750), dm.lastCpuSystem[cacheTimeMs][containerId])
}

func TestCalculateNetworkStats(t *testing.T) {
	// Create docker manager with tracker maps
	dm := &dockerManager{
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	cacheTimeMs := uint16(30000)

	// Pre-populate tracker for this cache time with initial values
	sentTracker := deltatracker.NewDeltaTracker[string, uint64]()
	recvTracker := deltatracker.NewDeltaTracker[string, uint64]()
	sentTracker.Set("container1", 1000)
	recvTracker.Set("container1", 800)
	sentTracker.Cycle() // Move to previous
	recvTracker.Cycle()

	dm.networkSentTrackers[cacheTimeMs] = sentTracker
	dm.networkRecvTrackers[cacheTimeMs] = recvTracker

	ctr := &container.ApiInfo{
		IdShort: "container1",
	}

	apiStats := &container.ApiStats{
		Networks: map[string]container.NetworkStats{
			"eth0": {TxBytes: 2000, RxBytes: 1800}, // New values
		},
	}

	stats := &container.Stats{
		PrevReadTime: time.Now().Add(-time.Second), // 1 second ago
	}

	// Test with initialized container
	sent, recv := dm.calculateNetworkStats(ctr, apiStats, stats, true, "test-container", cacheTimeMs)

	// Should return calculated byte rates per second
	assert.GreaterOrEqual(t, sent, uint64(0))
	assert.GreaterOrEqual(t, recv, uint64(0))

	// Cycle and test one-direction change (Tx only) is reflected independently
	dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)
	apiStats.Networks["eth0"] = container.NetworkStats{TxBytes: 2500, RxBytes: 1800} // +500 Tx only
	sent, recv = dm.calculateNetworkStats(ctr, apiStats, stats, true, "test-container", cacheTimeMs)
	assert.Greater(t, sent, uint64(0))
	assert.Equal(t, uint64(0), recv)
}

func TestDockerManagerCreation(t *testing.T) {
	// Test that dockerManager can be created without panicking
	dm := &dockerManager{
		lastCpuContainer:    make(map[uint16]map[string]uint64),
		lastCpuSystem:       make(map[uint16]map[string]uint64),
		lastCpuReadTime:     make(map[uint16]map[string]time.Time),
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	assert.NotNil(t, dm)
	assert.NotNil(t, dm.lastCpuContainer)
	assert.NotNil(t, dm.lastCpuSystem)
	assert.NotNil(t, dm.networkSentTrackers)
	assert.NotNil(t, dm.networkRecvTrackers)
}

func TestCheckDockerVersion(t *testing.T) {
	tests := []struct {
		name             string
		responses        []struct {
			statusCode int
			body       string
		}
		expectedGood     bool
		expectedRequests int
	}{
		{
			name: "200 with good version on first try",
			responses: []struct {
				statusCode int
				body       string
			}{
				{http.StatusOK, `{"Version":"25.0.1"}`},
			},
			expectedGood:     true,
			expectedRequests: 1,
		},
		{
			name: "200 with old version on first try",
			responses: []struct {
				statusCode int
				body       string
			}{
				{http.StatusOK, `{"Version":"24.0.7"}`},
			},
			expectedGood:     false,
			expectedRequests: 1,
		},
		{
			name: "non-200 then 200 with good version",
			responses: []struct {
				statusCode int
				body       string
			}{
				{http.StatusServiceUnavailable, `"not ready"`},
				{http.StatusOK, `{"Version":"25.1.0"}`},
			},
			expectedGood:     true,
			expectedRequests: 2,
		},
		{
			name: "non-200 on all retries",
			responses: []struct {
				statusCode int
				body       string
			}{
				{http.StatusInternalServerError, `"error"`},
				{http.StatusUnauthorized, `"error"`},
			},
			expectedGood:     false,
			expectedRequests: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := requestCount
				requestCount++
				if idx >= len(tt.responses) {
					idx = len(tt.responses) - 1
				}
				w.WriteHeader(tt.responses[idx].statusCode)
				fmt.Fprint(w, tt.responses[idx].body)
			}))
			defer server.Close()

			dm := &dockerManager{
				client: &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
							return net.Dial(network, server.Listener.Addr().String())
						},
					},
				},
			}

			dm.checkDockerVersion()

			assert.Equal(t, tt.expectedGood, dm.goodDockerVersion)
			assert.Equal(t, tt.expectedRequests, requestCount)
		})
	}

	t.Run("request error on all retries", func(t *testing.T) {
		requestCount := 0
		dm := &dockerManager{
			client: &http.Client{
				Transport: &http.Transport{
					DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
						requestCount++
						return nil, errors.New("connection refused")
					},
				},
			},
		}

		dm.checkDockerVersion()

		assert.False(t, dm.goodDockerVersion)
		assert.Equal(t, 2, requestCount)
	})
}

func TestCycleCpuDeltas(t *testing.T) {
	dm := &dockerManager{
		lastCpuContainer: map[uint16]map[string]uint64{
			30000: {"container1": 100, "container2": 200},
		},
		lastCpuSystem: map[uint16]map[string]uint64{
			30000: {"container1": 150, "container2": 250},
		},
		lastCpuReadTime: map[uint16]map[string]time.Time{
			30000: {"container1": time.Now()},
		},
	}

	cacheTimeMs := uint16(30000)

	// Verify values exist before cycling
	assert.Equal(t, uint64(100), dm.lastCpuContainer[cacheTimeMs]["container1"])
	assert.Equal(t, uint64(200), dm.lastCpuContainer[cacheTimeMs]["container2"])

	// Cycle the CPU deltas
	dm.cycleCpuDeltas(cacheTimeMs)

	// Verify values are cleared
	assert.Empty(t, dm.lastCpuContainer[cacheTimeMs])
	assert.Empty(t, dm.lastCpuSystem[cacheTimeMs])
	// lastCpuReadTime is not affected by cycleCpuDeltas
	assert.NotEmpty(t, dm.lastCpuReadTime[cacheTimeMs])
}

func TestCycleNetworkDeltas(t *testing.T) {
	// Create docker manager with tracker maps
	dm := &dockerManager{
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	cacheTimeMs := uint16(30000)

	// Get trackers for this cache time (creates them)
	sentTracker := dm.getNetworkTracker(cacheTimeMs, true)
	recvTracker := dm.getNetworkTracker(cacheTimeMs, false)

	// Set some test data
	sentTracker.Set("test", 100)
	recvTracker.Set("test", 200)

	// This should not panic
	assert.NotPanics(t, func() {
		dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)
	})

	// Verify that cycle worked by checking deltas are now zero (no previous values)
	assert.Equal(t, uint64(0), sentTracker.Delta("test"))
	assert.Equal(t, uint64(0), recvTracker.Delta("test"))
}

func TestConstants(t *testing.T) {
	// Test that constants are properly defined
	assert.Equal(t, uint16(60000), defaultCacheTimeMs)
	assert.Equal(t, uint64(5e9), maxNetworkSpeedBps)
	assert.Equal(t, 2100, dockerTimeoutMs)
}

func TestDockerStatsWithMockData(t *testing.T) {
	// Create a docker manager with initialized tracking
	dm := &dockerManager{
		lastCpuContainer:    make(map[uint16]map[string]uint64),
		lastCpuSystem:       make(map[uint16]map[string]uint64),
		lastCpuReadTime:     make(map[uint16]map[string]time.Time),
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		containerStatsMap:   make(map[string]*container.Stats),
	}

	cacheTimeMs := uint16(30000)

	// Test that initializeCpuTracking works
	dm.initializeCpuTracking(cacheTimeMs)
	assert.NotNil(t, dm.lastCpuContainer[cacheTimeMs])
	assert.NotNil(t, dm.lastCpuSystem[cacheTimeMs])

	// Test that we can set and get CPU values
	dm.setCpuCurrentValues(cacheTimeMs, "test-container", 1000, 2000)
	container, system := dm.getCpuPreviousValues(cacheTimeMs, "test-container")
	assert.Equal(t, uint64(1000), container)
	assert.Equal(t, uint64(2000), system)
}

func TestMemoryStatsEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		usage     uint64
		cache     uint64
		inactive  uint64
		isWindows bool
		expected  uint64
		hasError  bool
	}{
		{"Linux normal case", 1000, 200, 0, false, 800, false},
		{"Linux with inactive file", 1000, 0, 300, false, 700, false},
		{"Windows normal case", 0, 0, 0, true, 500, false},
		{"Linux zero usage error", 0, 0, 0, false, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiStats := &container.ApiStats{
				MemoryStats: container.MemoryStats{
					Usage: tt.usage,
					Stats: container.MemoryStatsStats{
						Cache:        tt.cache,
						InactiveFile: tt.inactive,
					},
				},
			}

			if tt.isWindows {
				apiStats.MemoryStats.PrivateWorkingSet = tt.expected
			}

			result, err := calculateMemoryUsage(apiStats, tt.isWindows)

			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestContainerStatsInitialization(t *testing.T) {
	stats := &container.Stats{Name: "test-container"}

	// Verify initial values
	assert.Equal(t, "test-container", stats.Name)
	assert.Equal(t, 0.0, stats.Cpu)
	assert.Equal(t, 0.0, stats.Mem)
	assert.Equal(t, 0.0, stats.NetworkSent)
	assert.Equal(t, 0.0, stats.NetworkRecv)
	assert.Equal(t, time.Time{}, stats.PrevReadTime)

	// Test updating values
	testTime := time.Now()
	updateContainerStatsValues(stats, 45.67, 2097152, 1048576, 524288, testTime)

	assert.Equal(t, 45.67, stats.Cpu)
	assert.Equal(t, 2.0, stats.Mem)
	assert.Equal(t, [2]uint64{1048576, 524288}, stats.Bandwidth)
	// Deprecated fields still populated for backward compatibility with older hubs
	assert.Equal(t, 1.0, stats.NetworkSent) // 1048576 bytes = 1 MB
	assert.Equal(t, 0.5, stats.NetworkRecv) // 524288 bytes = 0.5 MB
	assert.Equal(t, testTime, stats.PrevReadTime)
}

// Test with real Docker API test data
func TestCalculateMemoryUsageWithRealData(t *testing.T) {
	// Load minimal container stats from test data
	data, err := os.ReadFile("test-data/container.json")
	require.NoError(t, err)

	var apiStats container.ApiStats
	err = json.Unmarshal(data, &apiStats)
	require.NoError(t, err)

	// Test memory calculation with real data
	usedMemory, err := calculateMemoryUsage(&apiStats, false)
	require.NoError(t, err)

	// From the real data: usage - inactive_file = 507400192 - 165130240 = 342269952
	expected := uint64(507400192 - 165130240)
	assert.Equal(t, expected, usedMemory)
}

func TestCpuPercentageCalculationWithRealData(t *testing.T) {
	// Load minimal container stats from test data
	data1, err := os.ReadFile("test-data/container.json")
	require.NoError(t, err)

	data2, err := os.ReadFile("test-data/container2.json")
	require.NoError(t, err)

	var apiStats1, apiStats2 container.ApiStats
	err = json.Unmarshal(data1, &apiStats1)
	require.NoError(t, err)
	err = json.Unmarshal(data2, &apiStats2)
	require.NoError(t, err)

	// Calculate delta manually: 314891801000 - 312055276000 = 2836525000
	// System delta: 1368474900000000 - 1366399830000000 = 2075070000000
	// Expected %: (2836525000 / 2075070000000) * 100 ≈ 0.1367%
	expectedPct := float64(2836525000) / float64(2075070000000) * 100.0
	actualPct := apiStats2.CalculateCpuPercentLinux(apiStats1.CPUStats.CPUUsage.TotalUsage, apiStats1.CPUStats.SystemUsage)

	assert.InDelta(t, expectedPct, actualPct, 0.01)
}

func TestNetworkStatsCalculationWithRealData(t *testing.T) {
	// Create synthetic test data to avoid timing issues
	apiStats1 := &container.ApiStats{
		Networks: map[string]container.NetworkStats{
			"eth0": {TxBytes: 1000000, RxBytes: 500000},
		},
	}

	apiStats2 := &container.ApiStats{
		Networks: map[string]container.NetworkStats{
			"eth0": {TxBytes: 3000000, RxBytes: 1500000}, // 2MB sent, 1MB received increase
		},
	}

	// Create docker manager with tracker maps
	dm := &dockerManager{
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	ctr := &container.ApiInfo{IdShort: "test-container"}
	cacheTimeMs := uint16(30000) // Test with 30 second cache

	// Use exact timing for deterministic results
	exactly1000msAgo := time.Now().Add(-1000 * time.Millisecond)
	stats := &container.Stats{
		PrevReadTime: exactly1000msAgo,
	}

	// First call sets baseline
	sent1, recv1 := dm.calculateNetworkStats(ctr, apiStats1, stats, true, "test", cacheTimeMs)
	assert.Equal(t, uint64(0), sent1)
	assert.Equal(t, uint64(0), recv1)

	// Cycle to establish baseline for this cache time
	dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)

	// Calculate expected results precisely
	deltaSent := uint64(2000000)                             // 3000000 - 1000000
	deltaRecv := uint64(1000000)                             // 1500000 - 500000
	expectedElapsedMs := uint64(1000)                        // Exactly 1000ms
	expectedSentRate := deltaSent * 1000 / expectedElapsedMs // Should be exactly 2000000
	expectedRecvRate := deltaRecv * 1000 / expectedElapsedMs // Should be exactly 1000000

	// Second call with changed data
	sent2, recv2 := dm.calculateNetworkStats(ctr, apiStats2, stats, true, "test", cacheTimeMs)

	// Should be exactly the expected rates (no tolerance needed)
	assert.Equal(t, expectedSentRate, sent2)
	assert.Equal(t, expectedRecvRate, recv2)

	// Bad speed cap: set absurd delta over 1ms and expect 0 due to cap
	dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)
	stats.PrevReadTime = time.Now().Add(-1 * time.Millisecond)
	apiStats1.Networks["eth0"] = container.NetworkStats{TxBytes: 0, RxBytes: 0}
	apiStats2.Networks["eth0"] = container.NetworkStats{TxBytes: 10 * 1024 * 1024 * 1024, RxBytes: 0} // 10GB delta
	_, _ = dm.calculateNetworkStats(ctr, apiStats1, stats, true, "test", cacheTimeMs)                 // baseline
	dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)
	sent3, recv3 := dm.calculateNetworkStats(ctr, apiStats2, stats, true, "test", cacheTimeMs)
	assert.Equal(t, uint64(0), sent3)
	assert.Equal(t, uint64(0), recv3)
}

func TestContainerStatsEndToEndWithRealData(t *testing.T) {
	// Load minimal container stats
	data, err := os.ReadFile("test-data/container.json")
	require.NoError(t, err)

	var apiStats container.ApiStats
	err = json.Unmarshal(data, &apiStats)
	require.NoError(t, err)

	// Create a docker manager with proper initialization
	dm := &dockerManager{
		lastCpuContainer:    make(map[uint16]map[string]uint64),
		lastCpuSystem:       make(map[uint16]map[string]uint64),
		lastCpuReadTime:     make(map[uint16]map[string]time.Time),
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		containerStatsMap:   make(map[string]*container.Stats),
	}

	// Initialize CPU tracking
	cacheTimeMs := uint16(30000)
	dm.initializeCpuTracking(cacheTimeMs)

	// Create container info
	ctr := &container.ApiInfo{
		IdShort: "abc123",
	}

	// Initialize container stats
	stats := &container.Stats{Name: "jellyfin"}
	dm.containerStatsMap[ctr.IdShort] = stats

	// Test individual components that we can verify
	usedMemory, memErr := calculateMemoryUsage(&apiStats, false)
	assert.NoError(t, memErr)
	assert.Greater(t, usedMemory, uint64(0))

	// Test CPU percentage validation
	cpuPct := 85.5
	err = validateCpuPercentage(cpuPct, "jellyfin")
	assert.NoError(t, err)

	err = validateCpuPercentage(150.0, "jellyfin")
	assert.Error(t, err)

	// Test stats value updates
	testStats := &container.Stats{}
	testTime := time.Now()
	updateContainerStatsValues(testStats, cpuPct, usedMemory, 1000000, 500000, testTime)

	assert.Equal(t, cpuPct, testStats.Cpu)
	assert.Equal(t, bytesToMegabytes(float64(usedMemory)), testStats.Mem)
	assert.Equal(t, [2]uint64{1000000, 500000}, testStats.Bandwidth)
	// Deprecated fields still populated for backward compatibility with older hubs
	assert.Equal(t, bytesToMegabytes(1000000), testStats.NetworkSent)
	assert.Equal(t, bytesToMegabytes(500000), testStats.NetworkRecv)
	assert.Equal(t, testTime, testStats.PrevReadTime)
}

func TestEdgeCasesWithRealData(t *testing.T) {
	// Test with minimal container stats
	minimalStats := &container.ApiStats{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 1000},
			SystemUsage: 50000,
		},
		MemoryStats: container.MemoryStats{
			Usage: 1000000,
			Stats: container.MemoryStatsStats{
				Cache:        0,
				InactiveFile: 0,
			},
		},
		Networks: map[string]container.NetworkStats{
			"eth0": {TxBytes: 1000, RxBytes: 500},
		},
	}

	// Test memory calculation with zero cache/inactive
	usedMemory, err := calculateMemoryUsage(minimalStats, false)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1000000), usedMemory) // Should equal usage when no cache

	// Test CPU percentage calculation
	cpuPct := minimalStats.CalculateCpuPercentLinux(0, 0) // First run
	assert.Equal(t, 0.0, cpuPct)

	// Test with Windows data
	minimalStats.MemoryStats.PrivateWorkingSet = 800000
	usedMemory, err = calculateMemoryUsage(minimalStats, true)
	assert.NoError(t, err)
	assert.Equal(t, uint64(800000), usedMemory)
}

func TestDockerStatsWorkflow(t *testing.T) {
	// Test the complete workflow that can be tested without HTTP calls
	dm := &dockerManager{
		lastCpuContainer:    make(map[uint16]map[string]uint64),
		lastCpuSystem:       make(map[uint16]map[string]uint64),
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		containerStatsMap:   make(map[string]*container.Stats),
	}

	cacheTimeMs := uint16(30000)

	// Test CPU tracking workflow
	dm.initializeCpuTracking(cacheTimeMs)
	assert.NotNil(t, dm.lastCpuContainer[cacheTimeMs])

	// Test setting and getting CPU values
	dm.setCpuCurrentValues(cacheTimeMs, "test-container", 1000, 50000)
	containerVal, systemVal := dm.getCpuPreviousValues(cacheTimeMs, "test-container")
	assert.Equal(t, uint64(1000), containerVal)
	assert.Equal(t, uint64(50000), systemVal)

	// Test network tracking workflow (multi-interface summation)
	sentTracker := dm.getNetworkTracker(cacheTimeMs, true)
	recvTracker := dm.getNetworkTracker(cacheTimeMs, false)

	// Simulate two interfaces summed by setting combined totals
	sentTracker.Set("test-container", 1000+2000)
	recvTracker.Set("test-container", 500+700)

	deltaSent := sentTracker.Delta("test-container")
	deltaRecv := recvTracker.Delta("test-container")
	assert.Equal(t, uint64(0), deltaSent) // No previous value
	assert.Equal(t, uint64(0), deltaRecv)

	// Cycle and test again
	dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)

	// Increase each interface total (combined totals go up by 1500 and 800)
	sentTracker.Set("test-container", (1000+2000)+1500)
	recvTracker.Set("test-container", (500+700)+800)

	deltaSent = sentTracker.Delta("test-container")
	deltaRecv = recvTracker.Delta("test-container")
	assert.Equal(t, uint64(1500), deltaSent)
	assert.Equal(t, uint64(800), deltaRecv)
}

func TestNetworkRateCalculationFormula(t *testing.T) {
	// Test the exact formula used in calculateNetworkStats
	testCases := []struct {
		name         string
		deltaBytes   uint64
		elapsedMs    uint64
		expectedRate uint64
	}{
		{"1MB over 1 second", 1000000, 1000, 1000000},
		{"2MB over 1 second", 2000000, 1000, 2000000},
		{"1MB over 2 seconds", 1000000, 2000, 500000},
		{"500KB over 500ms", 500000, 500, 1000000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This is the exact formula from calculateNetworkStats
			actualRate := tc.deltaBytes * 1000 / tc.elapsedMs
			assert.Equal(t, tc.expectedRate, actualRate,
				"Rate calculation should be exact: %d bytes * 1000 / %d ms = %d",
				tc.deltaBytes, tc.elapsedMs, tc.expectedRate)
		})
	}
}

func TestGetHostInfo(t *testing.T) {
	data, err := os.ReadFile("test-data/system_info.json")
	require.NoError(t, err)

	var info container.HostInfo
	err = json.Unmarshal(data, &info)
	require.NoError(t, err)

	assert.Equal(t, "6.8.0-31-generic", info.KernelVersion)
	assert.Equal(t, "Ubuntu 24.04 LTS", info.OperatingSystem)
	// assert.Equal(t, "24.04", info.OSVersion)
	// assert.Equal(t, "linux", info.OSType)
	// assert.Equal(t, "x86_64", info.Architecture)
	assert.EqualValues(t, 4, info.NCPU)
	assert.EqualValues(t, 2095882240, info.MemTotal)
	// assert.Equal(t, "27.0.1", info.ServerVersion)
}

func TestDeltaTrackerCacheTimeIsolation(t *testing.T) {
	// Test that different cache times have separate DeltaTracker instances
	dm := &dockerManager{
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	ctr := &container.ApiInfo{IdShort: "web-server"}
	cacheTime1 := uint16(30000)
	cacheTime2 := uint16(60000)

	// Get trackers for different cache times (creates separate instances)
	sentTracker1 := dm.getNetworkTracker(cacheTime1, true)
	recvTracker1 := dm.getNetworkTracker(cacheTime1, false)

	sentTracker2 := dm.getNetworkTracker(cacheTime2, true)
	recvTracker2 := dm.getNetworkTracker(cacheTime2, false)

	// Verify they are different instances
	assert.NotSame(t, sentTracker1, sentTracker2)
	assert.NotSame(t, recvTracker1, recvTracker2)

	// Set values for cache time 1
	sentTracker1.Set(ctr.IdShort, 1000000)
	recvTracker1.Set(ctr.IdShort, 500000)

	// Set values for cache time 2
	sentTracker2.Set(ctr.IdShort, 2000000)
	recvTracker2.Set(ctr.IdShort, 1000000)

	// Verify they don't interfere (both should return 0 since no previous values)
	assert.Equal(t, uint64(0), sentTracker1.Delta(ctr.IdShort))
	assert.Equal(t, uint64(0), recvTracker1.Delta(ctr.IdShort))
	assert.Equal(t, uint64(0), sentTracker2.Delta(ctr.IdShort))
	assert.Equal(t, uint64(0), recvTracker2.Delta(ctr.IdShort))

	// Cycle cache time 1 trackers
	dm.cycleNetworkDeltasForCacheTime(cacheTime1)

	// Set new values for cache time 1
	sentTracker1.Set(ctr.IdShort, 3000000) // 2MB increase
	recvTracker1.Set(ctr.IdShort, 1500000) // 1MB increase

	// Cache time 1 should show deltas, cache time 2 should still be 0
	assert.Equal(t, uint64(2000000), sentTracker1.Delta(ctr.IdShort))
	assert.Equal(t, uint64(1000000), recvTracker1.Delta(ctr.IdShort))
	assert.Equal(t, uint64(0), sentTracker2.Delta(ctr.IdShort)) // Unaffected
	assert.Equal(t, uint64(0), recvTracker2.Delta(ctr.IdShort)) // Unaffected

	// Cycle cache time 2 and verify it works independently
	dm.cycleNetworkDeltasForCacheTime(cacheTime2)
	sentTracker2.Set(ctr.IdShort, 2500000) // 0.5MB increase
	recvTracker2.Set(ctr.IdShort, 1200000) // 0.2MB increase

	assert.Equal(t, uint64(500000), sentTracker2.Delta(ctr.IdShort))
	assert.Equal(t, uint64(200000), recvTracker2.Delta(ctr.IdShort))
}

func TestParseDockerStatus(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedStatus string
		expectedHealth container.DockerHealth
	}{
		{
			name:           "status with About an removed",
			input:          "Up About an hour (healthy)",
			expectedStatus: "Up an hour",
			expectedHealth: container.DockerHealthHealthy,
		},
		{
			name:           "status without About an unchanged",
			input:          "Up 2 hours (healthy)",
			expectedStatus: "Up 2 hours",
			expectedHealth: container.DockerHealthHealthy,
		},
		{
			name:           "status with About and no parentheses",
			input:          "Up About an hour",
			expectedStatus: "Up an hour",
			expectedHealth: container.DockerHealthNone,
		},
		{
			name:           "status without parentheses",
			input:          "Created",
			expectedStatus: "Created",
			expectedHealth: container.DockerHealthNone,
		},
		{
			name:           "empty status",
			input:          "",
			expectedStatus: "",
			expectedHealth: container.DockerHealthNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, health := parseDockerStatus(tt.input)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedHealth, health)
		})
	}
}

func TestConstantsAndUtilityFunctions(t *testing.T) {
	// Test constants are properly defined
	assert.Equal(t, uint16(60000), defaultCacheTimeMs)
	assert.Equal(t, uint64(5e9), maxNetworkSpeedBps)
	assert.Equal(t, 2100, dockerTimeoutMs)
	assert.Equal(t, uint32(1024*1024), uint32(maxLogFrameSize)) // 1MB
	assert.Equal(t, 5*1024*1024, maxTotalLogSize)               // 5MB

	// Test utility functions
	assert.Equal(t, 1.5, twoDecimals(1.499))
	assert.Equal(t, 1.5, twoDecimals(1.5))
	assert.Equal(t, 1.5, twoDecimals(1.501))

	assert.Equal(t, 1.0, bytesToMegabytes(1048576)) // 1 MB
	assert.Equal(t, 0.5, bytesToMegabytes(524288))  // 512 KB
	assert.Equal(t, 0.0, bytesToMegabytes(0))
}

func TestDecodeDockerLogStream(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expected    string
		expectError bool
		multiplexed bool
	}{
		{
			name: "simple log entry",
			input: []byte{
				// Frame 1: stdout, 11 bytes
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0B,
				'H', 'e', 'l', 'l', 'o', ' ', 'W', 'o', 'r', 'l', 'd',
			},
			expected:    "Hello World",
			expectError: false,
			multiplexed: true,
		},
		{
			name: "multiple frames",
			input: []byte{
				// Frame 1: stdout, 5 bytes
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				'H', 'e', 'l', 'l', 'o',
				// Frame 2: stdout, 5 bytes
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				'W', 'o', 'r', 'l', 'd',
			},
			expected:    "HelloWorld",
			expectError: false,
			multiplexed: true,
		},
		{
			name: "zero length frame",
			input: []byte{
				// Frame 1: stdout, 0 bytes
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				// Frame 2: stdout, 5 bytes
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				'H', 'e', 'l', 'l', 'o',
			},
			expected:    "Hello",
			expectError: false,
			multiplexed: true,
		},
		{
			name:        "empty input",
			input:       []byte{},
			expected:    "",
			expectError: false,
			multiplexed: true,
		},
		{
			name:        "raw stream (not multiplexed)",
			input:       []byte("raw log content"),
			expected:    "raw log content",
			multiplexed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.input)
			var builder strings.Builder
			err := decodeDockerLogStream(reader, &builder, tt.multiplexed)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, builder.String())
			}
		})
	}
}

func TestDecodeDockerLogStreamMemoryProtection(t *testing.T) {
	t.Run("excessively large frame should error", func(t *testing.T) {
		// Create a frame with size exceeding maxLogFrameSize
		excessiveSize := uint32(maxLogFrameSize + 1)
		input := []byte{
			// Frame header with excessive size
			0x01, 0x00, 0x00, 0x00,
			byte(excessiveSize >> 24), byte(excessiveSize >> 16), byte(excessiveSize >> 8), byte(excessiveSize),
		}

		reader := bytes.NewReader(input)
		var builder strings.Builder
		err := decodeDockerLogStream(reader, &builder, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "log frame size")
		assert.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("total size limit should truncate", func(t *testing.T) {
		// Create frames that exceed maxTotalLogSize (5MB)
		// Use frames within maxLogFrameSize (1MB) to avoid single-frame rejection
		frameSize := uint32(800 * 1024) // 800KB per frame
		var input []byte

		// Frames 1-6: 800KB each (total 4.8MB - within 5MB limit)
		for i := 0; i < 6; i++ {
			char := byte('A' + i)
			frameHeader := []byte{
				0x01, 0x00, 0x00, 0x00,
				byte(frameSize >> 24), byte(frameSize >> 16), byte(frameSize >> 8), byte(frameSize),
			}
			input = append(input, frameHeader...)
			input = append(input, bytes.Repeat([]byte{char}, int(frameSize))...)
		}

		// Frame 7: 800KB (would bring total to 5.6MB, exceeding 5MB limit - should be truncated)
		frame7Header := []byte{
			0x01, 0x00, 0x00, 0x00,
			byte(frameSize >> 24), byte(frameSize >> 16), byte(frameSize >> 8), byte(frameSize),
		}
		input = append(input, frame7Header...)
		input = append(input, bytes.Repeat([]byte{'Z'}, int(frameSize))...)

		reader := bytes.NewReader(input)
		var builder strings.Builder
		err := decodeDockerLogStream(reader, &builder, true)

		// Should complete without error (graceful truncation)
		assert.NoError(t, err)
		// Should have read 6 frames (4.8MB total, stopping before 7th would exceed 5MB limit)
		expectedSize := int(frameSize) * 6
		assert.Equal(t, expectedSize, builder.Len())
		// Should contain A-F but not Z
		result := builder.String()
		assert.Contains(t, result, "A")
		assert.Contains(t, result, "F")
		assert.NotContains(t, result, "Z")
	})
}

func TestShouldExcludeContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		patterns      []string
		expected      bool
	}{
		{
			name:          "empty patterns excludes nothing",
			containerName: "any-container",
			patterns:      []string{},
			expected:      false,
		},
		{
			name:          "exact match - excluded",
			containerName: "test-web",
			patterns:      []string{"test-web", "test-api"},
			expected:      true,
		},
		{
			name:          "exact match - not excluded",
			containerName: "prod-web",
			patterns:      []string{"test-web", "test-api"},
			expected:      false,
		},
		{
			name:          "wildcard prefix match - excluded",
			containerName: "test-web",
			patterns:      []string{"test-*"},
			expected:      true,
		},
		{
			name:          "wildcard prefix match - not excluded",
			containerName: "prod-web",
			patterns:      []string{"test-*"},
			expected:      false,
		},
		{
			name:          "wildcard suffix match - excluded",
			containerName: "myapp-staging",
			patterns:      []string{"*-staging"},
			expected:      true,
		},
		{
			name:          "wildcard suffix match - not excluded",
			containerName: "myapp-prod",
			patterns:      []string{"*-staging"},
			expected:      false,
		},
		{
			name:          "wildcard both sides match - excluded",
			containerName: "test-myapp-staging",
			patterns:      []string{"*-myapp-*"},
			expected:      true,
		},
		{
			name:          "wildcard both sides match - not excluded",
			containerName: "prod-yourapp-live",
			patterns:      []string{"*-myapp-*"},
			expected:      false,
		},
		{
			name:          "multiple patterns - matches first",
			containerName: "test-container",
			patterns:      []string{"test-*", "*-staging"},
			expected:      true,
		},
		{
			name:          "multiple patterns - matches second",
			containerName: "myapp-staging",
			patterns:      []string{"test-*", "*-staging"},
			expected:      true,
		},
		{
			name:          "multiple patterns - no match",
			containerName: "prod-web",
			patterns:      []string{"test-*", "*-staging"},
			expected:      false,
		},
		{
			name:          "mixed exact and wildcard - exact match",
			containerName: "temp-container",
			patterns:      []string{"temp-container", "test-*"},
			expected:      true,
		},
		{
			name:          "mixed exact and wildcard - wildcard match",
			containerName: "test-web",
			patterns:      []string{"temp-container", "test-*"},
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := &dockerManager{
				excludeContainers: tt.patterns,
			}
			result := dm.shouldExcludeContainer(tt.containerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnsiEscapePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "simple color code",
			input:    "\x1b[34mINFO\x1b[0m client mode",
			expected: "INFO client mode",
		},
		{
			name:     "multiple color codes",
			input:    "\x1b[31mERROR\x1b[0m: \x1b[33mWarning\x1b[0m message",
			expected: "ERROR: Warning message",
		},
		{
			name:     "bold and color",
			input:    "\x1b[1;32mSUCCESS\x1b[0m",
			expected: "SUCCESS",
		},
		{
			name:     "cursor movement codes",
			input:    "Line 1\x1b[KLine 2",
			expected: "Line 1Line 2",
		},
		{
			name:     "256 color code",
			input:    "\x1b[38;5;196mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "RGB/truecolor code",
			input:    "\x1b[38;2;255;0;0mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "mixed content with newlines",
			input:    "\x1b[34m2024-01-01 12:00:00\x1b[0m INFO Starting\n\x1b[31m2024-01-01 12:00:01\x1b[0m ERROR Failed",
			expected: "2024-01-01 12:00:00 INFO Starting\n2024-01-01 12:00:01 ERROR Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ansiEscapePattern.ReplaceAllString(tt.input, "")
			assert.Equal(t, tt.expected, result)
		})
	}
}
