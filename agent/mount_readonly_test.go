package agent

import (
	"os"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsReadOnlyOpts(t *testing.T) {
	assert.False(t, isReadOnlyOpts(nil))
	assert.False(t, isReadOnlyOpts([]string{"rw", "relatime"}))
	assert.True(t, isReadOnlyOpts([]string{"ro", "relatime"}))
	assert.False(t, isReadOnlyOpts([]string{"ro", "rw"}))
	assert.True(t, isReadOnlyOpts([]string{"rw", "ro"}))
}

func TestParseMountinfoReadOnly(t *testing.T) {
	content := `
22 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw,errors=remount-ro
45 22 8:17 / /mnt/data ro,relatime - ext4 /dev/sdb1 ro
`
	byMount, byDev := parseMountinfoReadOnly(content)
	assert.False(t, byMount["/"])
	assert.True(t, byMount["/mnt/data"])
	assert.False(t, byDev["sda1"])
	assert.True(t, byDev["sdb1"])
}

func TestLookupReadOnly(t *testing.T) {
	byMount := map[string]bool{"/": false, "/mnt/data": true}
	byDev := map[string]bool{"sdb1": true}
	ro, ok := lookupReadOnly(byMount, byDev, "/", "sda1")
	require.True(t, ok)
	assert.False(t, ro)
	ro, ok = lookupReadOnly(byMount, byDev, "/extra-filesystems/sdb1__Data", "sdb1")
	require.True(t, ok)
	assert.True(t, ro)
	_, ok = lookupReadOnly(byMount, byDev, "/missing", "nope")
	assert.False(t, ok)
}

func TestUpdateDiskReadOnlyTransition(t *testing.T) {
	agent := &Agent{
		fsStats: map[string]*system.FsStats{
			"sda1": {Root: true, Mountpoint: "/"},
			"sdb1": {Root: false, Mountpoint: "/mnt/data", Name: "Data"},
		},
		mountRO: &mountReadOnlyState{
			seenWritable: make(map[string]bool),
		},
	}

	// Inject via path by writing a temp mountinfo and pointing path at it.
	dir := t.TempDir()
	path := dir + "/mountinfo"
	require.NoError(t, writeFile(path, `
22 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw
45 22 8:17 / /mnt/data rw,relatime - ext4 /dev/sdb1 rw
`))
	agent.mountRO.path = path
	agent.updateDiskReadOnly()
	assert.False(t, agent.fsStats["sda1"].ReadOnly)
	assert.False(t, agent.fsStats["sdb1"].ReadOnly)
	assert.True(t, agent.mountRO.seenWritable["sdb1"])

	require.NoError(t, writeFile(path, `
22 1 8:1 / / ro,relatime - ext4 /dev/sda1 ro
45 22 8:17 / /mnt/data ro,relatime - ext4 /dev/sdb1 ro
`))
	agent.updateDiskReadOnly()
	assert.True(t, agent.fsStats["sda1"].ReadOnly)
	assert.True(t, agent.fsStats["sdb1"].ReadOnly)

	data := &system.CombinedData{}
	agent.applyReadOnlyInfo(data)
	assert.True(t, data.Info.RootReadOnly)
	assert.True(t, data.Info.ExtraFsReadOnly["Data"])
	assert.Contains(t, data.Info.ReadOnlyFilesystems, "Data")
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
