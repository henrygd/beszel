//go:build testing

package agent

import "context"

// StartForTesting runs the agent until the test cancels its context.
func (a *Agent) StartForTesting(ctx context.Context, opts ServerOptions) error {
	return a.start(ctx, opts)
}

// TESTING ONLY: GetConnectionManager is a helper function to get the connection manager for testing.
func (a *Agent) GetConnectionManager() *ConnectionManager {
	return a.connectionManager
}

// TESTING ONLY: GetState returns a synchronised snapshot of the connection state.
func (c *ConnectionManager) GetState() ConnectionState {
	return c.getState()
}
