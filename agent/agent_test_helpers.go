//go:build testing
// +build testing

package agent

// TESTING ONLY: GetConnectionManager is a helper function to get the connection manager for testing.
func (a *Agent) GetConnectionManager() *ConnectionManager {
	return a.connectionManager
}
