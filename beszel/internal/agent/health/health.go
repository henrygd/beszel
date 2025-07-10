// Package health provides functions to check and update the health of the agent.
// It uses a file in the temp directory to store the timestamp of the last connection attempt.
// If the timestamp is older than 90 seconds, the agent is considered unhealthy.
// NB: The agent must be started with the Start() method to be considered healthy.
package health

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"
)

// healthFile is the path to the health file
var healthFile = filepath.Join(os.TempDir(), "beszel_health")

// Check checks if the agent is connected by checking the modification time of the health file
func Check() error {
	fileInfo, err := os.Stat(healthFile)
	if err != nil {
		return err
	}
	if time.Since(fileInfo.ModTime()) > 91*time.Second {
		log.Println("over 90 seconds since last connection")
		return errors.New("unhealthy")
	}
	return nil
}

// Update updates the modification time of the health file
func Update() error {
	file, err := os.Create(healthFile)
	if err != nil {
		return err
	}
	return file.Close()
}

// CleanUp removes the health file
func CleanUp() error {
	return os.Remove(healthFile)
}
