package utils

import (
	"math"
	"os"
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
