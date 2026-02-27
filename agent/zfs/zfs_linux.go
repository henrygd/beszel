//go:build linux

// Package zfs provides functions to read ZFS statistics.
package zfs

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ARCSize() (uint64, error) {
	file, err := os.Open("/proc/spl/kstat/zfs/arcstats")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "size") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				return 0, fmt.Errorf("unexpected arcstats size format: %s", line)
			}
			return strconv.ParseUint(fields[2], 10, 64)
		}
	}

	return 0, fmt.Errorf("size field not found in arcstats")
}
