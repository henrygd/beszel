//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDataDir(t *testing.T) {
	// Test with explicit dataDir parameter
	t.Run("explicit data dir", func(t *testing.T) {
		tempDir := t.TempDir()
		result, err := GetDataDir(tempDir)
		require.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	// Test with explicit non-existent dataDir that can be created
	t.Run("explicit data dir - create new", func(t *testing.T) {
		tempDir := t.TempDir()
		newDir := filepath.Join(tempDir, "new-data-dir")
		result, err := GetDataDir(newDir)
		require.NoError(t, err)
		assert.Equal(t, newDir, result)

		// Verify directory was created
		stat, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, stat.IsDir())
	})

	// Test with DATA_DIR environment variable
	t.Run("DATA_DIR environment variable", func(t *testing.T) {
		tempDir := t.TempDir()

		// Set environment variable
		oldValue := os.Getenv("DATA_DIR")
		defer func() {
			if oldValue == "" {
				os.Unsetenv("BESZEL_AGENT_DATA_DIR")
			} else {
				os.Setenv("BESZEL_AGENT_DATA_DIR", oldValue)
			}
		}()

		os.Setenv("BESZEL_AGENT_DATA_DIR", tempDir)

		result, err := GetDataDir()
		require.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	// Test with invalid explicit dataDir
	t.Run("invalid explicit data dir", func(t *testing.T) {
		invalidPath := "/invalid/path/that/cannot/be/created"
		_, err := GetDataDir(invalidPath)
		assert.Error(t, err)
	})

	// Test fallback behavior (empty dataDir, no env var)
	t.Run("fallback to default directories", func(t *testing.T) {
		// Clear DATA_DIR environment variable
		oldValue := os.Getenv("DATA_DIR")
		defer func() {
			if oldValue == "" {
				os.Unsetenv("DATA_DIR")
			} else {
				os.Setenv("DATA_DIR", oldValue)
			}
		}()
		os.Unsetenv("DATA_DIR")

		// This will try platform-specific defaults, which may or may not work
		// We're mainly testing that it doesn't panic and returns some result
		result, err := GetDataDir()
		// We don't assert success/failure here since it depends on system permissions
		// Just verify we get a string result if no error
		if err == nil {
			assert.NotEmpty(t, result)
		}
	})
}

func TestTestDataDirs(t *testing.T) {
	// Test with existing valid directory
	t.Run("existing valid directory", func(t *testing.T) {
		tempDir := t.TempDir()
		result, err := testDataDirs([]string{tempDir})
		require.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	// Test with multiple directories, first one valid
	t.Run("multiple dirs - first valid", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidDir := "/invalid/path"
		result, err := testDataDirs([]string{tempDir, invalidDir})
		require.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	// Test with multiple directories, second one valid
	t.Run("multiple dirs - second valid", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidDir := "/invalid/path"
		result, err := testDataDirs([]string{invalidDir, tempDir})
		require.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	// Test with non-existing directory that can be created
	t.Run("create new directory", func(t *testing.T) {
		tempDir := t.TempDir()
		newDir := filepath.Join(tempDir, "new-dir")
		result, err := testDataDirs([]string{newDir})
		require.NoError(t, err)
		assert.Equal(t, newDir, result)

		// Verify directory was created
		stat, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, stat.IsDir())
	})

	// Test with no valid directories
	t.Run("no valid directories", func(t *testing.T) {
		invalidPaths := []string{"/invalid/path1", "/invalid/path2"}
		_, err := testDataDirs(invalidPaths)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "data directory not found")
	})
}

func TestIsValidDataDir(t *testing.T) {
	// Test with existing directory
	t.Run("existing directory", func(t *testing.T) {
		tempDir := t.TempDir()
		valid, err := isValidDataDir(tempDir, false)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	// Test with non-existing directory, createIfNotExists=false
	t.Run("non-existing dir - no create", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentDir := filepath.Join(tempDir, "does-not-exist")
		valid, err := isValidDataDir(nonExistentDir, false)
		require.NoError(t, err)
		assert.False(t, valid)
	})

	// Test with non-existing directory, createIfNotExists=true
	t.Run("non-existing dir - create", func(t *testing.T) {
		tempDir := t.TempDir()
		newDir := filepath.Join(tempDir, "new-dir")
		valid, err := isValidDataDir(newDir, true)
		require.NoError(t, err)
		assert.True(t, valid)

		// Verify directory was created
		stat, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, stat.IsDir())
	})

	// Test with file instead of directory
	t.Run("file instead of directory", func(t *testing.T) {
		tempDir := t.TempDir()
		tempFile := filepath.Join(tempDir, "testfile")
		err := os.WriteFile(tempFile, []byte("test"), 0644)
		require.NoError(t, err)

		valid, err := isValidDataDir(tempFile, false)
		assert.Error(t, err)
		assert.False(t, valid)
		assert.Contains(t, err.Error(), "is not a directory")
	})
}

func TestDirectoryExists(t *testing.T) {
	// Test with existing directory
	t.Run("existing directory", func(t *testing.T) {
		tempDir := t.TempDir()
		exists, err := directoryExists(tempDir)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	// Test with non-existing directory
	t.Run("non-existing directory", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentDir := filepath.Join(tempDir, "does-not-exist")
		exists, err := directoryExists(nonExistentDir)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	// Test with file instead of directory
	t.Run("file instead of directory", func(t *testing.T) {
		tempDir := t.TempDir()
		tempFile := filepath.Join(tempDir, "testfile")
		err := os.WriteFile(tempFile, []byte("test"), 0644)
		require.NoError(t, err)

		exists, err := directoryExists(tempFile)
		assert.Error(t, err)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "is not a directory")
	})
}

func TestDirectoryIsWritable(t *testing.T) {
	// Test with writable directory
	t.Run("writable directory", func(t *testing.T) {
		tempDir := t.TempDir()
		writable, err := directoryIsWritable(tempDir)
		require.NoError(t, err)
		assert.True(t, writable)
	})

	// Test with non-existing directory
	t.Run("non-existing directory", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentDir := filepath.Join(tempDir, "does-not-exist")
		writable, err := directoryIsWritable(nonExistentDir)
		assert.Error(t, err)
		assert.False(t, writable)
	})

	// Test with non-writable directory (Unix-like systems only)
	t.Run("non-writable directory", func(t *testing.T) {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			t.Skip("Skipping non-writable directory test on", runtime.GOOS)
		}

		tempDir := t.TempDir()
		readOnlyDir := filepath.Join(tempDir, "readonly")

		// Create the directory
		err := os.Mkdir(readOnlyDir, 0755)
		require.NoError(t, err)

		// Make it read-only
		err = os.Chmod(readOnlyDir, 0444)
		require.NoError(t, err)

		// Restore permissions after test for cleanup
		defer func() {
			os.Chmod(readOnlyDir, 0755)
		}()

		writable, err := directoryIsWritable(readOnlyDir)
		assert.Error(t, err)
		assert.False(t, writable)
	})
}
