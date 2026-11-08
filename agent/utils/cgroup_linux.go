//go:build linux

package utils

import (
	"os"
	"strconv"
	"strings"
)

// InCgroupV2 reports whether the given /proc/self/cgroup file lists the
// unified hierarchy (a "0::<path>" entry).
func InCgroupV2(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			return true
		}
	}
	return false
}

// CgroupMountPoint finds a hierarchy's mount point in the given mountinfo file.
// Use fstype "cgroup2" and an empty controller for v2, or "cgroup" and the
// desired controller for v1. It returns the mount root, not a process's leaf.
func CgroupMountPoint(path, fstype, controller string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		left, right, found := strings.Cut(line, " - ")
		if !found {
			continue
		}
		post := strings.Fields(right)
		if len(post) == 0 || post[0] != fstype {
			continue
		}
		if controller != "" && !mountOptHas(post, controller) {
			continue
		}
		fields := strings.Fields(left)
		if len(fields) >= 5 {
			return unescapeMountPoint(fields[4])
		}
	}
	return ""
}

// mountOptHas checks the comma-separated super options after the separator.
func mountOptHas(post []string, opt string) bool {
	if len(post) < 3 {
		return false
	}
	for o := range strings.SplitSeq(post[2], ",") {
		if o == opt {
			return true
		}
	}
	return false
}

func unescapeMountPoint(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

// ReadCgroupStat reads all unsigned counters from a cgroup key/value stat file
// in one read. Malformed entries are omitted; callers must check required keys.
func ReadCgroupStat(path string) (map[string]uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]uint64)
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			values[fields[0]] = value
		}
	}
	return values, nil
}

// ReadIntFile parses a signed decimal integer (e.g. a v1 CPU quota of -1).
func ReadIntFile(path string) (int64, bool) {
	raw, ok := ReadStringFileOK(path)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, err == nil
}
