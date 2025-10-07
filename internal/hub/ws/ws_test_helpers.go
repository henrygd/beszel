//go:build testing
// +build testing

package ws

// GetPendingCount returns the number of pending requests (for monitoring)
func (rm *RequestManager) GetPendingCount() int {
	rm.RLock()
	defer rm.RUnlock()
	return len(rm.pendingReqs)
}
