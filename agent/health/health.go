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
	"slices"
	"sync"
	"time"
)

// healthFiles are the candidate health file paths, most preferred first. On Linux
// /dev/shm comes first because /tmp is not always writable, e.g. for a non-root user
// in the agent image (#1455).
// Resolving them must not write: package init runs in the `agent health` process too.
var healthFiles = getHealthFilePaths("/dev/shm")

// written holds the candidates this process has written, so CleanUp removes only those. The
// others may be read-only to this process (e.g. /tmp under systemd ProtectSystem=strict without
// PrivateTmp) or belong to another agent (e.g. in the sticky /dev/shm), and failing to remove
// them must not turn a normal shutdown into an error.
var (
	writtenMu sync.Mutex
	written   []string
)

func getHealthFilePaths(shmDir string) []string {
	const filename = "beszel_health"
	tmpPath := filepath.Join(os.TempDir(), filename)
	if runtime.GOOS == "linux" {
		return []string{filepath.Join(shmDir, filename), tmpPath}
	}
	return []string{tmpPath}
}

func updateHealthFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	return file.Close()
}

// Check checks if the agent is connected by checking the modification time of the health file.
// It runs in its own process (`agent health`), so it must only read: creating or touching the
// file here would refresh the timestamp it is about to check. The newest candidate counts, so a
// stale file left in another location (e.g. by an agent that ran as a different user) is ignored.
func Check() error {
	var newest time.Time
	var err error
	for _, path := range healthFiles {
		fileInfo, statErr := os.Stat(path)
		if statErr != nil {
			err = statErr
			continue
		}
		if fileInfo.ModTime().After(newest) {
			newest = fileInfo.ModTime()
		}
	}
	if newest.IsZero() {
		return err
	}
	if time.Since(newest) > 91*time.Second {
		log.Println("over 90 seconds since last connection")
		return errors.New("unhealthy")
	}
	return nil
}

// Update updates the modification time of the health file, in the first location that is writable
func Update() error {
	var err error
	for _, path := range healthFiles {
		if err = updateHealthFile(path); err == nil {
			writtenMu.Lock()
			if !slices.Contains(written, path) {
				written = append(written, path)
			}
			writtenMu.Unlock()
			return nil
		}
	}
	return err
}

// CleanUp removes the health files this process wrote. The path is shared by every agent on the
// host, so a file that is already gone - another agent removed it, or a test ran two agents - is
// not a failure to report: it would surface as the error from Start. Candidates this process never
// wrote are left alone (see written).
func CleanUp() error {
	writtenMu.Lock()
	defer writtenMu.Unlock()
	for len(written) > 0 {
		if err := os.Remove(written[0]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		written = written[1:]
	}
	return nil
}
