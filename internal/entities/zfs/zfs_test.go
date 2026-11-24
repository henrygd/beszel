//go:build testing

package zfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPoolBackend(t *testing.T) {
	assert.Equal(t, "zfs", PoolBackend("tank"))
	assert.Equal(t, "btrfs", PoolBackend("b:uuid"))
	assert.Equal(t, "lvm", PoolBackend("l:uuid"))
}

func TestCanRefreshPool(t *testing.T) {
	data := &ZfsData{CompleteBackends: []string{"lvm"}}
	assert.True(t, data.AnyBackendComplete())
	assert.True(t, data.CanRefreshPool("l:uuid"))
	assert.False(t, data.CanRefreshPool("b:uuid"))
	assert.False(t, data.CanRefreshPool("tank"))

	assert.False(t, (&ZfsData{}).AnyBackendComplete())
	assert.True(t, (&ZfsData{Complete: true}).CanRefreshPool("b:uuid"), "old agents report only Complete")
}
