package agent

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

var pingTimeRegex = regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)

// probeICMP executes system ping command and parses latency. Returns -1 on failure.
func probeICMP(target string) float64 {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", "3000", target)
	default: // linux, darwin, freebsd
		cmd = exec.Command("ping", "-c", "1", "-W", "3", target)
	}

	start := time.Now()
	output, err := cmd.Output()
	if err != nil {
		// If ping fails but we got output, still try to parse
		if len(output) == 0 {
			return -1
		}
	}

	matches := pingTimeRegex.FindSubmatch(output)
	if len(matches) >= 2 {
		if ms, err := strconv.ParseFloat(string(matches[1]), 64); err == nil {
			return ms
		}
	}

	// Fallback: use wall clock time if ping succeeded but parsing failed
	if err == nil {
		return float64(time.Since(start).Microseconds()) / 1000.0
	}
	return -1
}
