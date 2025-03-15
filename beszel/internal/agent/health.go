package agent

import (
	"net"
	"time"
)

// Health checks if the agent's server is running by attempting to connect to it.
//
// If an error occurs when attempting to connect to the server, it returns the error.
func Health(addr string, network string) error {
	conn, err := net.DialTimeout(network, addr, 4*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
