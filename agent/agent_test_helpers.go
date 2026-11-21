//go:build testing

package agent

// TESTING ONLY: GetConnectionManager is a helper function to get the connection manager for testing.
func (a *Agent) GetConnectionManager() *ConnectionManager {
	return a.connectionManager
}

// TESTING ONLY: GetState returns a synchronised snapshot of the connection state.
func (c *ConnectionManager) GetState() ConnectionState {
	return c.getState()
}
