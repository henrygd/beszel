//go:build testing
// +build testing

package agent_test

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"beszel/internal/agent"
)

// setupTestServer creates a temporary server for testing
func setupTestServer(t *testing.T) (string, func()) {
	// Create a temporary socket file for Unix socket testing
	tempSockFile := os.TempDir() + "/beszel_health_test.sock"

	// Clean up any existing socket file
	os.Remove(tempSockFile)

	// Create a listener
	listener, err := net.Listen("unix", tempSockFile)
	require.NoError(t, err, "Failed to create test listener")

	// Start a simple server in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return // Listener closed
		}
		defer conn.Close()
		// Just accept the connection and do nothing
	}()

	// Return the socket file path and a cleanup function
	return tempSockFile, func() {
		listener.Close()
		os.Remove(tempSockFile)
	}
}

// setupTCPTestServer creates a temporary TCP server for testing
func setupTCPTestServer(t *testing.T) (string, func()) {
	// Listen on a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to create test listener")

	// Get the port that was assigned
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// Start a simple server in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return // Listener closed
		}
		defer conn.Close()
		// Just accept the connection and do nothing
	}()

	// Return the address and a cleanup function
	return fmt.Sprintf("127.0.0.1:%d", port), func() {
		listener.Close()
	}
}

func TestHealth(t *testing.T) {
	t.Run("server is running (unix socket)", func(t *testing.T) {
		// Setup a test server
		sockFile, cleanup := setupTestServer(t)
		defer cleanup()

		// Run the health check with explicit parameters
		result, err := agent.Health(sockFile, "unix")
		require.NoError(t, err, "Failed to check health")

		// Verify the result
		assert.Equal(t, 0, result, "Health check should return 0 when server is running")
	})

	t.Run("server is running (tcp address)", func(t *testing.T) {
		// Setup a test server
		addr, cleanup := setupTCPTestServer(t)
		defer cleanup()

		// Run the health check with explicit parameters
		result, err := agent.Health(addr, "tcp")
		require.NoError(t, err, "Failed to check health")

		// Verify the result
		assert.Equal(t, 0, result, "Health check should return 0 when server is running")
	})

	t.Run("server is not running", func(t *testing.T) {
		// Use an address that's likely not in use
		addr := "127.0.0.1:65535"

		// Run the health check with explicit parameters
		result, err := agent.Health(addr, "tcp")
		require.Error(t, err, "Health check should return an error when server is not running")

		// Verify the result
		assert.Equal(t, 1, result, "Health check should return 1 when server is not running")
	})

	t.Run("invalid network", func(t *testing.T) {
		// Use an invalid network type
		result, err := agent.Health("127.0.0.1:8080", "invalid_network")
		require.Error(t, err, "Health check should return an error with invalid network")
		assert.Equal(t, 1, result, "Health check should return 1 when network is invalid")
	})

	t.Run("unix socket not found", func(t *testing.T) {
		// Use a non-existent unix socket
		nonExistentSocket := os.TempDir() + "/non_existent_socket.sock"

		// Make sure it really doesn't exist
		os.Remove(nonExistentSocket)

		result, err := agent.Health(nonExistentSocket, "unix")
		require.Error(t, err, "Health check should return an error when socket doesn't exist")
		assert.Equal(t, 1, result, "Health check should return 1 when socket doesn't exist")
	})
}
