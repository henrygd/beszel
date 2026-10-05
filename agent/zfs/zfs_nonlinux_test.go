//go:build testing && !linux

package zfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorsUseUtilitiesOnNonLinux(t *testing.T) {
	oldCommandOutput := commandOutput
	commandOutput = func(name string, args ...string) ([]byte, error) {
		switch name {
		case "zpool":
			return []byte("tank\t100\t50\t50\tONLINE\n"), nil
		case "zfs":
			return []byte("tank\t50\t50\t/tank\n"), nil
		default:
			t.Fatalf("unexpected command %s", name)
			return nil, nil
		}
	}
	t.Cleanup(func() { commandOutput = oldCommandOutput })

	pools, err := PoolStats()
	require.NoError(t, err)
	assert.Equal(t, []PoolStat{{Name: "tank", Size: 100, Alloc: 50, Free: 50, Health: "ONLINE"}}, pools)
	datasets, err := Datasets()
	require.NoError(t, err)
	assert.Equal(t, []Dataset{{Name: "tank", Used: 50, Avail: 50, Mountpoint: "/tank"}}, datasets)
}
