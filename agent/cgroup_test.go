package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// writeCgroupFiles lays out files under dir and points the cgroup roots at it.
func withCgroupRoots(t *testing.T, v2, v1 map[string]string) {
	t.Helper()
	base := t.TempDir()

	write := func(sub string, files map[string]string) string {
		root := filepath.Join(base, sub)
		for name, content := range files {
			assert.NoError(t, os.MkdirAll(root, 0o755))
			assert.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o644))
		}
		return root
	}

	origV2, origV1 := cgroupV2Root, cgroupV1Root
	t.Cleanup(func() { cgroupV2Root, cgroupV1Root = origV2, origV1 })

	if v2 != nil {
		cgroupV2Root = write("v2", v2)
	} else {
		cgroupV2Root = filepath.Join(base, "missing-v2")
	}
	if v1 != nil {
		cgroupV1Root = write("v1", v1)
	} else {
		cgroupV1Root = filepath.Join(base, "missing-v1")
	}
}

func TestReadCgroupMemory(t *testing.T) {
	t.Run("cgroup v2 with limit and usage", func(t *testing.T) {
		withCgroupRoots(t, map[string]string{
			"memory.max":     "268435456\n",
			"memory.current": "200000000\n",
			"memory.stat":    "anon 120000000\nfile 80000000\nkernel 5000000\n",
		}, nil)

		s := readCgroupMemory()
		assert.True(t, s.limitOK)
		assert.Equal(t, uint64(268435456), s.limit)
		assert.True(t, s.usageOK)
		assert.Equal(t, uint64(80000000), s.cache)
		assert.Equal(t, uint64(120000000), s.used) // current - file
	})

	t.Run("cgroup v2 unlimited still reports usage", func(t *testing.T) {
		withCgroupRoots(t, map[string]string{
			"memory.max":     "max\n",
			"memory.current": "150000000\n",
			"memory.stat":    "file 50000000\n",
		}, nil)

		s := readCgroupMemory()
		assert.False(t, s.limitOK)
		assert.True(t, s.usageOK)
		assert.Equal(t, uint64(100000000), s.used)
		assert.Equal(t, uint64(50000000), s.cache)
	})

	t.Run("cgroup v2 limit without usage accounting", func(t *testing.T) {
		withCgroupRoots(t, map[string]string{
			"memory.max": "268435456\n",
		}, nil)

		s := readCgroupMemory()
		assert.True(t, s.limitOK)
		assert.Equal(t, uint64(268435456), s.limit)
		assert.False(t, s.usageOK)
	})

	t.Run("falls back to cgroup v1", func(t *testing.T) {
		withCgroupRoots(t, nil, map[string]string{
			"memory.limit_in_bytes": "536870912\n",
			"memory.usage_in_bytes": "300000000\n",
			"memory.stat":           "cache 100000000\ntotal_cache 90000000\n",
		})

		s := readCgroupMemory()
		assert.True(t, s.limitOK)
		assert.Equal(t, uint64(536870912), s.limit)
		assert.True(t, s.usageOK)
		assert.Equal(t, uint64(90000000), s.cache)
		assert.Equal(t, uint64(210000000), s.used)
	})

	t.Run("cgroup v1 falls back to cache when total_cache is absent", func(t *testing.T) {
		withCgroupRoots(t, nil, map[string]string{
			"memory.limit_in_bytes": "536870912\n",
			"memory.usage_in_bytes": "300000000\n",
			"memory.stat":           "cache 90000000\nrss 200000000\n",
		})

		s := readCgroupMemory()
		assert.Equal(t, uint64(90000000), s.cache)
		assert.Equal(t, uint64(210000000), s.used)
	})

	t.Run("cgroup v1 unlimited sentinel is not a limit", func(t *testing.T) {
		withCgroupRoots(t, nil, map[string]string{
			"memory.limit_in_bytes": "9223372036854771712\n",
			"memory.usage_in_bytes": "300000000\n",
			"memory.stat":           "total_cache 90000000\n",
		})

		s := readCgroupMemory()
		assert.False(t, s.limitOK)
		assert.True(t, s.usageOK)
		assert.Equal(t, uint64(210000000), s.used)
	})

	t.Run("no cgroup files", func(t *testing.T) {
		withCgroupRoots(t, nil, nil)
		s := readCgroupMemory()
		assert.False(t, s.limitOK)
		assert.False(t, s.usageOK)
	})
}
