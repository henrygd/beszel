//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFsKeyStore(t *testing.T) {
	t.Run("round trips pinned keys", func(t *testing.T) {
		dir := t.TempDir()
		s := loadFsKeyStore(dir)
		s.pin("fsid:a", "sdb1")
		s.save()

		key, ok := loadFsKeyStore(dir).key("fsid:a")
		assert.True(t, ok)
		assert.Equal(t, "sdb1", key)
	})

	t.Run("does not write when nothing changed", func(t *testing.T) {
		dir := t.TempDir()
		s := loadFsKeyStore(dir)
		s.pin("fsid:a", "sdb1")
		s.save()
		path := filepath.Join(dir, fsKeysFile)
		require.NoError(t, os.Chmod(path, 0o400))
		t.Cleanup(func() { os.Chmod(path, 0o600) })

		s = loadFsKeyStore(dir)
		s.pin("fsid:a", "sdb1")
		assert.False(t, s.changed)
		s.save()
	})

	t.Run("pinning a key releases it from another ID", func(t *testing.T) {
		s := loadFsKeyStore("")
		s.pin("fsid:old", "sdb1")
		s.pin("fsid:new", "sdb1")

		_, ok := s.key("fsid:old")
		assert.False(t, ok)
		key, _ := s.key("fsid:new")
		assert.Equal(t, "sdb1", key)
	})

	t.Run("works without a data dir", func(t *testing.T) {
		s := loadFsKeyStore("")
		s.pin("fsid:a", "sdb1")
		s.save()
		key, _ := s.key("fsid:a")
		assert.Equal(t, "sdb1", key)
	})

	t.Run("replaces an invalid file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, fsKeysFile)
		require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

		s := loadFsKeyStore(dir)
		assert.Empty(t, s.keys)
		s.pin("fsid:a", "sdb1")
		s.save()

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.JSONEq(t, `{"fsid:a":"sdb1"}`, string(data))
	})
}
