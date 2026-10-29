// Package health provides functions to check and update the health of the agent.
// It uses a file in the temp directory to store the timestamp of the last connection attempt.
// If the timestamp is older than 90 seconds, the agent is considered unhealthy.
// NB: The agent must be started with the Start() method to be considered healthy.
package health

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// healthFile is the path to the health file
var healthFile = getHealthFilePath()

func getHealthFilePath() string {
	filename := "beszel_health"
	if runtime.GOOS == "linux" {
		fullPath := filepath.Join("/dev/shm", filename)
		if err := updateHealthFile(fullPath); err == nil {
			return fullPath
		}
	}
	return filepath.Join(os.TempDir(), filename)
}

func updateHealthFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	return file.Close()
}

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
	return updateHealthFile(healthFile)
}

// CleanUp removes the health file. The path is shared by every agent on the host, so a file that
// is already gone - another agent removed it, or a test ran two agents - is not a failure to
// report: it would surface as the error from Start.
func CleanUp() error {
	if err := os.Remove(healthFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
