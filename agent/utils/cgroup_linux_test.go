//go:build linux

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInCgroupV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cgroup")
	assert.False(t, InCgroupV2(path))
	for _, tt := range []struct {
		name, contents string
		want           bool
	}{
		{"v1", "3:cpu,cpuacct:/system.slice/agent.service\n2:memory:/\n", false},
		{"v2 service", "0::/system.slice/agent.service\n", true},
		{"hybrid", "2:memory:/\n0::/\n", true},
		{"empty", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o644))
			assert.Equal(t, tt.want, InCgroupV2(path))
		})
	}
}

func TestCgroupMountPoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mountinfo")
	assert.Empty(t, CgroupMountPoint(path, "cgroup2", ""))
	contents := "malformed\n" +
		"30 25 0:26 / /sys/fs/cgroup/cpu rw - cgroup cgroup rw,cpu,cpuacct\n" +
		"31 25 0:27 / /sys/fs/cgroup/memory rw - cgroup cgroup rw,memory\n" +
		`32 25 0:28 /guest /sys/fs/cgroup/unified\040root\134name rw - cgroup2 cgroup2 rw` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	assert.Equal(t, "/sys/fs/cgroup/cpu", CgroupMountPoint(path, "cgroup", "cpuacct"))
	assert.Equal(t, "/sys/fs/cgroup/memory", CgroupMountPoint(path, "cgroup", "memory"))
	assert.Equal(t, `/sys/fs/cgroup/unified root\name`, CgroupMountPoint(path, "cgroup2", ""))
	assert.Empty(t, CgroupMountPoint(path, "cgroup", "mem"))
	assert.Empty(t, CgroupMountPoint(path, "cgroup", "cpuset"))
}

func TestReadCgroupStat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.stat")
	_, err := ReadCgroupStat(path)
	require.Error(t, err)
	contents := "anon 1024\nfile\t2048\ninactive_file 0\n" +
		"negative -1\noverflow 18446744073709551616\ninvalid abc\nmissing\nextra 1 2\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	stat, err := ReadCgroupStat(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]uint64{"anon": 1024, "file": 2048, "inactive_file": 0}, stat)
	_, ok := stat["inactive_file"]
	assert.True(t, ok)
	_, ok = stat["usage_usec"]
	assert.False(t, ok)
}

func TestReadIntFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota")
	_, ok := ReadIntFile(path)
	assert.False(t, ok)
	for _, tt := range []struct {
		contents string
		want     int64
		ok       bool
	}{
		{" -1\n", -1, true},
		{"100000\n", 100000, true},
		{"0", 0, true},
		{"max", 0, false},
		{"9223372036854775808", 9223372036854775807, false},
	} {
		t.Run(tt.contents, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o644))
			value, ok := ReadIntFile(path)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, value)
			}
		})
	}
}
