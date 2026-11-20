// Package utils provides utility functions for the agent.
package utils

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// GetEnv retrieves an environment variable with a "BESZEL_AGENT_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_AGENT_" + key); exists {
		return value, exists
	}
	return os.LookupEnv(key)
}

// BytesToMegabytes converts bytes to megabytes and rounds to two decimal places.
func BytesToMegabytes(b float64) float64 {
	return TwoDecimals(b / 1048576)
}

// BytesToGigabytes converts bytes to gigabytes and rounds to two decimal places.
func BytesToGigabytes(b uint64) float64 {
	return TwoDecimals(float64(b) / 1073741824)
}

// TwoDecimals rounds a float64 value to two decimal places.
func TwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

// func RoundFloat(val float64, precision uint) float64 {
//     ratio := math.Pow(10, float64(precision))
//     return math.Round(val*ratio) / ratio
// }

// ReadStringFile returns trimmed file contents or empty string on error.
func ReadStringFile(path string) string {
	content, _ := ReadStringFileOK(path)
	return content
}

// ReadStringFileOK returns trimmed file contents and read success.
func ReadStringFileOK(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// ReadStringFileLimited reads a file into a string with a maximum size (in bytes) to avoid
// allocating large buffers and potential panics with pseudo-files when the size is misreported.
func ReadStringFileLimited(path string, maxSize int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, maxSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n < 0 {
		return "", fmt.Errorf("%s returned negative bytes: %d", path, n)
	}
	return strings.TrimSpace(string(buf[:n])), nil
}

// FileExists reports whether the given path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadUintFile parses a decimal uint64 value from a file.
func ReadUintFile(path string) (uint64, bool) {
	raw, ok := ReadStringFileOK(path)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// safeLookPathHomebrewExec is a seccomp-safe replacement for exec.LookPath:
// Android seccomp filters (Android < 12) kill processes that issue the
// faccessat2 syscall which Go >= 1.24 uses inside exec.LookPath. Scanning
// PATH with os.Stat (newfstatat) is safe on every Android version.
func safeLookPathHomebrewExec(file string) (string, error) {
	if strings.Contains(file, "/") {
		if err := statExecutable(file); err != nil {
			return "", err
		}
		return file, nil
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, file)
		if err := statExecutable(path); err == nil {
			return path, nil
		}
	}
	return "", fs.ErrNotExist
}

// statExecutable reports whether path is a regular executable file.
func statExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fs.ErrInvalid
	}
	if fi.Mode()&0o111 == 0 {
		return fs.ErrPermission
	}
	return nil
}

// LookPathHomebrew is like exec.LookPath but also checks Homebrew paths.
func LookPathHomebrew(file string) (string, error) {
	foundPath, lookPathErr := safeLookPathHomebrewExec(file)
	if lookPathErr == nil {
		return foundPath, nil
	}
	var homebrewPath string
	switch runtime.GOOS {
	case "darwin":
		homebrewPath = filepath.Join("/opt", "homebrew", "bin", file)
	case "linux":
		homebrewPath = filepath.Join("/home", "linuxbrew", ".linuxbrew", "bin", file)
	}
	if homebrewPath != "" {
		if _, err := os.Stat(homebrewPath); err == nil {
			return homebrewPath, nil
		}
	}
	return "", lookPathErr
}
