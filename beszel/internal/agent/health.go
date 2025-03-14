package agent

import (
	"net"
	"time"
)

// Health checks if the agent's server is running by attempting to connect to it.
// It returns 0 if the server is running, 1 otherwise (as in exit codes).
//
// If an error occurs when attempting to connect to the server, it returns the error.
func Health(addr string, network string) (int, error) {
	conn, err := net.DialTimeout(network, addr, 4*time.Second)
	if err != nil {
		return 1, err
	}
	conn.Close()
	return 0, nil
}
