package agent

import (
	"os"
	"strconv"
	"strings"
)

// readStringFile returns trimmed file contents or empty string on error.
func readStringFile(path string) string {
	content, _ := readStringFileOK(path)
	return content
}

// readStringFileOK returns trimmed file contents and read success.
func readStringFileOK(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// fileExists reports whether the given path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readUintFile parses a decimal uint64 value from a file.
func readUintFile(path string) (uint64, bool) {
	raw, ok := readStringFileOK(path)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
