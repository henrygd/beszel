package deltatracker

import (
	"sync"

	"golang.org/x/exp/constraints"
)

// Numeric is a constraint that permits any integer or floating-point type.
type Numeric interface {
	constraints.Integer | constraints.Float
}

// DeltaTracker is a generic, thread-safe tracker for calculating differences
// in numeric values over time.
// K is the key type (e.g., int, string).
// V is the value type (e.g., int, int64, float32, float64).
type DeltaTracker[K comparable, V Numeric] struct {
	mu       sync.RWMutex
	current  map[K]V
	previous map[K]V
}

// NewDeltaTracker creates a new generic tracker.
func NewDeltaTracker[K comparable, V Numeric]() *DeltaTracker[K, V] {
	return &DeltaTracker[K, V]{
		current:  make(map[K]V),
		previous: make(map[K]V),
	}
}

// Set records the current value for a given ID.
func (t *DeltaTracker[K, V]) Set(id K, value V) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current[id] = value
}

// Deltas returns a map of all calculated deltas for the current interval.
func (t *DeltaTracker[K, V]) Deltas() map[K]V {
	t.mu.RLock()
	defer t.mu.RUnlock()

	deltas := make(map[K]V)
	for id, currentVal := range t.current {
		if previousVal, ok := t.previous[id]; ok {
			deltas[id] = currentVal - previousVal
		} else {
			deltas[id] = 0
		}
	}
	return deltas
}

// Delta returns the delta for a single key.
// Returns 0 if the key doesn't exist or has no previous value.
func (t *DeltaTracker[K, V]) Delta(id K) V {
	t.mu.RLock()
	defer t.mu.RUnlock()

	currentVal, currentOk := t.current[id]
	if !currentOk {
		return 0
	}

	previousVal, previousOk := t.previous[id]
	if !previousOk {
		return 0
	}

	return currentVal - previousVal
}

// Cycle prepares the tracker for the next interval.
func (t *DeltaTracker[K, V]) Cycle() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.previous = t.current
	t.current = make(map[K]V)
}

// 	// --- Example 1: Integer values (unchanged) ---
// 	fmt.Println("--- 🚀 Example with int64 values (PIDs) ---")
// 	pidTracker := NewDeltaTracker[int, int64]()
// 	pidTracker.Set(101, 20000)
// 	pidTracker.Cycle()
// 	pidTracker.Set(101, 22500)
// 	fmt.Println("PID Deltas:", pidTracker.Deltas())
// 	fmt.Println("----------------------------------------")

// 	// --- Example 2: Float values (New!) ---
// 	fmt.Println("\n--- 🚀 Example with float64 values (CPU Load) ---")
// 	// Track the 1-minute load average for different servers.
// 	loadTracker := NewDeltaTracker[string, float64]()

// 	// Minute 1
// 	loadTracker.Set("server-alpha", 0.74)
// 	loadTracker.Set("server-beta", 1.15)
// 	fmt.Println("Minute 1 Loads:", loadTracker.Deltas())
// 	loadTracker.Cycle()

// 	// Minute 2
// 	loadTracker.Set("server-alpha", 0.68) // Load decreased
// 	loadTracker.Set("server-beta", 1.55)  // Load increased
// 	loadTracker.Set("server-gamma", 0.25) // New server

// 	minute2Deltas := loadTracker.Deltas()
// 	fmt.Println("Minute 2 Load Deltas:", minute2Deltas)
// 	fmt.Printf("Change in alpha's load: %.2f\n", minute2Deltas["server-alpha"])
// 	fmt.Printf("Change in beta's load:  %.2f\n", minute2Deltas["server-beta"])
// 	fmt.Println("----------------------------------------")
