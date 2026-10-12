//go:build testing

package ws

import "time"

// GetPendingCount returns the number of pending requests (for monitoring)
func (rm *RequestManager) GetPendingCount() int {
	rm.RLock()
	defer rm.RUnlock()
	return len(rm.pendingReqs)
}

// TESTING ONLY: Deadline reports the connection read deadline.
func Deadline() time.Duration { return deadline }

// TESTING ONLY: SetDeadline overrides the connection read deadline so tests can
// drive the poll schedule at a reduced scale. It returns the previous value.
func SetDeadline(d time.Duration) time.Duration {
	previous := deadline
	deadline = d
	return previous
}
