package deltatracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDeltaTracker(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()
	assert.NotNil(t, tracker)
	assert.Empty(t, tracker.current)
	assert.Empty(t, tracker.previous)
}

func TestSet(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()
	tracker.Set("key1", 10)

	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	assert.Equal(t, 10, tracker.current["key1"])
}

func TestDeltas(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()

	// Test with no previous values
	tracker.Set("key1", 10)
	tracker.Set("key2", 20)

	deltas := tracker.Deltas()
	assert.Equal(t, 0, deltas["key1"])
	assert.Equal(t, 0, deltas["key2"])

	// Cycle to move current to previous
	tracker.Cycle()

	// Set new values and check deltas
	tracker.Set("key1", 15) // Delta should be 5 (15-10)
	tracker.Set("key2", 25) // Delta should be 5 (25-20)
	tracker.Set("key3", 30) // New key, delta should be 0

	deltas = tracker.Deltas()
	assert.Equal(t, 5, deltas["key1"])
	assert.Equal(t, 5, deltas["key2"])
	assert.Equal(t, 0, deltas["key3"])
}

func TestCycle(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()

	tracker.Set("key1", 10)
	tracker.Set("key2", 20)

	// Verify current has values
	tracker.mu.RLock()
	assert.Equal(t, 10, tracker.current["key1"])
	assert.Equal(t, 20, tracker.current["key2"])
	assert.Empty(t, tracker.previous)
	tracker.mu.RUnlock()

	tracker.Cycle()

	// After cycle, previous should have the old current values
	// and current should be empty
	tracker.mu.RLock()
	assert.Empty(t, tracker.current)
	assert.Equal(t, 10, tracker.previous["key1"])
	assert.Equal(t, 20, tracker.previous["key2"])
	tracker.mu.RUnlock()
}

func TestCompleteWorkflow(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()

	// First interval
	tracker.Set("server1", 100)
	tracker.Set("server2", 200)

	// Get deltas for first interval (should be zero)
	firstDeltas := tracker.Deltas()
	assert.Equal(t, 0, firstDeltas["server1"])
	assert.Equal(t, 0, firstDeltas["server2"])

	// Cycle to next interval
	tracker.Cycle()

	// Second interval
	tracker.Set("server1", 150) // Delta: 50
	tracker.Set("server2", 180) // Delta: -20
	tracker.Set("server3", 300) // New server, delta: 300

	secondDeltas := tracker.Deltas()
	assert.Equal(t, 50, secondDeltas["server1"])
	assert.Equal(t, -20, secondDeltas["server2"])
	assert.Equal(t, 0, secondDeltas["server3"])
}

func TestDeltaTrackerWithDifferentTypes(t *testing.T) {
	// Test with int64
	intTracker := NewDeltaTracker[string, int64]()
	intTracker.Set("pid1", 1000)
	intTracker.Cycle()
	intTracker.Set("pid1", 1200)
	intDeltas := intTracker.Deltas()
	assert.Equal(t, int64(200), intDeltas["pid1"])

	// Test with float64
	floatTracker := NewDeltaTracker[string, float64]()
	floatTracker.Set("cpu1", 1.5)
	floatTracker.Cycle()
	floatTracker.Set("cpu1", 2.7)
	floatDeltas := floatTracker.Deltas()
	assert.InDelta(t, 1.2, floatDeltas["cpu1"], 0.0001)

	// Test with int keys
	pidTracker := NewDeltaTracker[int, int64]()
	pidTracker.Set(101, 20000)
	pidTracker.Cycle()
	pidTracker.Set(101, 22500)
	pidDeltas := pidTracker.Deltas()
	assert.Equal(t, int64(2500), pidDeltas[101])
}

func TestDelta(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()

	// Test getting delta for non-existent key
	result := tracker.Delta("nonexistent")
	assert.Equal(t, 0, result)

	// Test getting delta for key with no previous value
	tracker.Set("key1", 10)
	result = tracker.Delta("key1")
	assert.Equal(t, 0, result)

	// Cycle to move current to previous
	tracker.Cycle()

	// Test getting delta for key with previous value
	tracker.Set("key1", 15)
	result = tracker.Delta("key1")
	assert.Equal(t, 5, result)

	// Test getting delta for key that exists in previous but not current
	result = tracker.Delta("key1")
	assert.Equal(t, 5, result) // Should still return 5

	// Test getting delta for key that exists in current but not previous
	tracker.Set("key2", 20)
	result = tracker.Delta("key2")
	assert.Equal(t, 0, result)
}

func TestDeltaWithDifferentTypes(t *testing.T) {
	// Test with int64
	intTracker := NewDeltaTracker[string, int64]()
	intTracker.Set("pid1", 1000)
	intTracker.Cycle()
	intTracker.Set("pid1", 1200)
	result := intTracker.Delta("pid1")
	assert.Equal(t, int64(200), result)

	// Test with float64
	floatTracker := NewDeltaTracker[string, float64]()
	floatTracker.Set("cpu1", 1.5)
	floatTracker.Cycle()
	floatTracker.Set("cpu1", 2.7)
	floatResult := floatTracker.Delta("cpu1")
	assert.InDelta(t, 1.2, floatResult, 0.0001)

	// Test with int keys
	pidTracker := NewDeltaTracker[int, int64]()
	pidTracker.Set(101, 20000)
	pidTracker.Cycle()
	pidTracker.Set(101, 22500)
	pidResult := pidTracker.Delta(101)
	assert.Equal(t, int64(2500), pidResult)
}

func TestDeltaConcurrentAccess(t *testing.T) {
	tracker := NewDeltaTracker[string, int]()

	// Set initial values
	tracker.Set("key1", 10)
	tracker.Set("key2", 20)
	tracker.Cycle()

	// Set new values
	tracker.Set("key1", 15)
	tracker.Set("key2", 25)

	// Test concurrent access safety
	result1 := tracker.Delta("key1")
	result2 := tracker.Delta("key2")

	assert.Equal(t, 5, result1)
	assert.Equal(t, 5, result2)
}
