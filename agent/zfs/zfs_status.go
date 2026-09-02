package zfs

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PoolStatus holds parsed `zpool status` information for one pool.
type PoolStatus struct {
	Name  string
	State string // ONLINE, DEGRADED, FAULTED, ...
	Scrub ScrubStatus
	Vdevs []VdevStatus
}

// ScrubStatus holds the scrub (or resilver) status parsed from the scan line.
type ScrubStatus struct {
	State    string // NONE, SCANNING, FINISHED, CANCELED
	Progress string // e.g. "10.00%" while scanning
	Errors   uint64
}

// VdevStatus is a single vdev row (mirror, raidz, or leaf disk).
type VdevStatus struct {
	Name         string
	State        string
	ReadErrs     uint64
	WriteErrs    uint64
	ChecksumErrs uint64
}

var (
	progressRe = regexp.MustCompile(`(\d+\.\d+)%\s+done`)
	errorsRe   = regexp.MustCompile(`with\s+(\d+)\s+errors`)
)

// PoolStatuses runs `zpool status` and parses per-pool state, scrub, and vdev
// information. The human-readable format has been stable across OpenZFS
// releases; rows are matched by their tabular shape rather than position.
func PoolStatuses() ([]PoolStatus, error) {
	out, err := commandOutput("zpool", "status")
	if err != nil {
		return nil, fmt.Errorf("zpool status: %w", err)
	}
	return parseZpoolStatusOutput(out)
}

// parseZpoolStatusOutput parses the output of `zpool status`.
func parseZpoolStatusOutput(out []byte) ([]PoolStatus, error) {
	var pools []PoolStatus
	var current *PoolStatus
	inConfig := false
	scanContinuation := false // next non-blank line continues the scan line (progress)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "pool:"):
			pools = append(pools, PoolStatus{Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))})
			current = &pools[len(pools)-1]
			inConfig = false
			scanContinuation = false
		case current == nil:
			continue
		case strings.HasPrefix(trimmed, "state:"):
			current.State = strings.TrimSpace(strings.TrimPrefix(trimmed, "state:"))
		case strings.HasPrefix(trimmed, "scan:"):
			current.Scrub = parseScanLine(trimmed)
			// zpool status prints the progress percentage on the line after scan.
			scanContinuation = true
		case trimmed == "config:":
			inConfig = true
		case scanContinuation:
			// The line after scan: may be an indented progress continuation.
			if m := progressRe.FindStringSubmatch(trimmed); m != nil {
				current.Scrub.Progress = m[1] + "%"
			}
			scanContinuation = false
		case inConfig && (line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			// Table rows are indented; blank lines separate sections. The
			// column header and the pool's own row are skipped.
			if trimmed != "" && !strings.HasPrefix(trimmed, "NAME") {
				if vdev, ok := parseVdevLine(trimmed, current.Name); ok {
					current.Vdevs = append(current.Vdevs, vdev)
				}
			}
		case inConfig:
			// unindented line (errors:, status:, next pool:) ends the table
			inConfig = false
		}
	}
	return pools, scanner.Err()
}

// parseScanLine maps a `scan:` line to a ScrubStatus.
func parseScanLine(line string) ScrubStatus {
	var scrub ScrubStatus
	switch {
	case strings.Contains(line, "in progress"):
		scrub.State = "SCANNING"
	case strings.Contains(line, "canceled"):
		scrub.State = "CANCELED"
	case strings.Contains(line, "repaired"), strings.Contains(line, "resilvered"):
		scrub.State = "FINISHED"
	default:
		scrub.State = "NONE"
	}
	if m := progressRe.FindStringSubmatch(line); m != nil {
		scrub.Progress = m[1] + "%"
	}
	if m := errorsRe.FindStringSubmatch(line); m != nil {
		if n, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			scrub.Errors = n
		}
	}
	return scrub
}

// parseVdevLine parses one row of the config table. Rows have the shape
// "NAME STATE READ WRITE CKSUM [extra...]". The first data row is the pool
// itself and is skipped since it duplicates pool-level info.
func parseVdevLine(line, poolName string) (VdevStatus, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return VdevStatus{}, false
	}
	if fields[0] == poolName {
		return VdevStatus{}, false
	}
	read, err1 := strconv.ParseUint(fields[2], 10, 64)
	write, err2 := strconv.ParseUint(fields[3], 10, 64)
	cksum, err3 := strconv.ParseUint(fields[4], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return VdevStatus{}, false
	}
	return VdevStatus{
		Name:         fields[0],
		State:        fields[1],
		ReadErrs:     read,
		WriteErrs:    write,
		ChecksumErrs: cksum,
	}, true
}
