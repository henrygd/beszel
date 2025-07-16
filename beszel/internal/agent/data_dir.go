package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// getDataDir returns the path to the data directory for the agent and an error
// if the directory is not valid. Pass an empty string to attempt to find the
// optimal data directory.
func getDataDir(dataDir string) (string, error) {
	if dataDir == "" {
		dataDir, _ = GetEnv("DATA_DIR")
	}

	if dataDir != "" {
		return testDataDirs([]string{dataDir})
	}

	var dirsToTry []string

	if runtime.GOOS == "windows" {
		dirsToTry = []string{
			filepath.Join(os.Getenv("APPDATA"), "beszel-agent"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "beszel-agent"),
		}
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dirsToTry = []string{
			"/var/lib/beszel-agent",
			filepath.Join(homeDir, ".config", "beszel"),
		}
	}
	return testDataDirs(dirsToTry)
}

func testDataDirs(paths []string) (string, error) {
	// first check if the directory exists and is writable
	for _, path := range paths {
		if valid, _ := isValidDataDir(path, false); valid {
			return path, nil
		}
	}
	// if the directory doesn't exist, try to create it
	for _, path := range paths {
		exists, _ := directoryExists(path)
		if exists {
			continue
		}

		if err := os.MkdirAll(path, 0755); err != nil {
			continue
		}

		// Verify the created directory is actually writable
		writable, _ := directoryIsWritable(path)
		if !writable {
			continue
		}

		return path, nil
	}

	return "", errors.New("data directory not found")
}

func isValidDataDir(path string, createIfNotExists bool) (bool, error) {
	exists, err := directoryExists(path)
	if err != nil {
		return false, err
	}

	if !exists {
		if !createIfNotExists {
			return false, nil
		}
		if err = os.MkdirAll(path, 0755); err != nil {
			return false, err
		}
	}

	// Always check if the directory is writable
	writable, err := directoryIsWritable(path)
	if err != nil {
		return false, err
	}
	return writable, nil
}

// directoryExists checks if a directory exists
func directoryExists(path string) (bool, error) {
	// Check if directory exists
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !stat.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}
	return true, nil
}

// directoryIsWritable tests if a directory is writable by creating and removing a temporary file
func directoryIsWritable(path string) (bool, error) {
	testFile := filepath.Join(path, ".write-test")
	file, err := os.Create(testFile)
	if err != nil {
		return false, err
	}
	defer file.Close()
	defer os.Remove(testFile)
	return true, nil
}
