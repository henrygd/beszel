//go:build testing

package health

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	// Override healthFiles to use a temporary directory for this test.
	healthFile := useHealthFiles(t, filepath.Join(t.TempDir(), "beszel_health_test"))[0]

	t.Run("check with no health file", func(t *testing.T) {
		err := Check()
		require.Error(t, err)
		assert.True(t, os.IsNotExist(err), "expected a file-not-exist error, but got: %v", err)
	})

	t.Run("update and check", func(t *testing.T) {
		err := Update()
		require.NoError(t, err, "Update() failed")

		err = Check()
		assert.NoError(t, err, "Check() failed immediately after Update()")
	})

	// This test uses synctest to simulate time passing.
	t.Run("check with simulated time", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// Update the file to set the initial timestamp.
			require.NoError(t, Update(), "Update() failed inside synctest")

			// Set the mtime to the current fake time to align the file's timestamp with the simulated clock.
			now := time.Now()
			require.NoError(t, os.Chtimes(healthFile, now, now), "Chtimes failed")

			// Wait a duration less than the threshold.
			time.Sleep(89 * time.Second)
			synctest.Wait()

			// The check should still pass.
			assert.NoError(t, Check(), "Check() failed after 89s")

			// Wait for the total duration to exceed the threshold.
			time.Sleep(5 * time.Second)
			synctest.Wait()

			// The check should now fail as unhealthy.
			err := Check()
			require.Error(t, err, "Check() should have failed after 91s")
			assert.Equal(t, "unhealthy", err.Error(), "Check() returned wrong error")
		})
	})
}

// The health file path is shared by every agent on the host, so a second removal must not be
// reported as a failure: it reaches the caller as the error returned by Agent.Start.
func TestCleanUpIsIdempotent(t *testing.T) {
	healthFile := useHealthFiles(t, filepath.Join(t.TempDir(), "beszel_health"))[0]

	require.NoError(t, Update())
	require.FileExists(t, healthFile)
	require.NoError(t, CleanUp())
	require.NoFileExists(t, healthFile)
	assert.NoError(t, CleanUp(), "removing a health file that is already gone is not an error")
}

// useHealthFiles points the package at the given candidate paths for one test.
func useHealthFiles(t *testing.T, paths ...string) []string {
	t.Helper()
	original := healthFiles
	healthFiles = paths
	t.Cleanup(func() { healthFiles = original })
	return paths
}

// `agent health` runs package init and Check in its own process. If either wrote the file, a
// hung agent's stale timestamp would be refreshed by the very check meant to catch it.
func TestGetHealthFilePathsDoesNotWrite(t *testing.T) {
	shmDir := t.TempDir()
	paths := getHealthFilePaths(shmDir)
	if runtime.GOOS == "linux" {
		assert.Equal(t, filepath.Join(shmDir, "beszel_health"), paths[0])
	}
	assert.NoFileExists(t, filepath.Join(shmDir, "beszel_health"))
}

func TestCheckDoesNotTouchHealthFile(t *testing.T) {
	healthFile := useHealthFiles(t, filepath.Join(t.TempDir(), "beszel_health"))[0]
	require.NoError(t, Update())
	stale := time.Now().Add(-2 * time.Minute)
	require.NoError(t, os.Chtimes(healthFile, stale, stale))

	err := Check()
	require.Error(t, err)
	assert.Equal(t, "unhealthy", err.Error())

	info, err := os.Stat(healthFile)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(stale), "Check changed the health file's mtime")
}

// The first location is not always writable (#1455), so Update falls back to the next one and
// Check finds the file there.
func TestHealthFileFallback(t *testing.T) {
	dir := t.TempDir()
	unwritable := filepath.Join(dir, "missing-dir", "beszel_health")
	fallback := filepath.Join(dir, "beszel_health")
	useHealthFiles(t, unwritable, fallback)

	require.NoError(t, Update())
	assert.NoFileExists(t, unwritable)
	assert.FileExists(t, fallback)
	assert.NoError(t, Check())
}

// A file another agent left behind (say, root's, which a non-root agent cannot overwrite) must not
// hide the one the running agent keeps fresh.
func TestCheckIgnoresStaleFileInOtherLocation(t *testing.T) {
	dir := t.TempDir()
	stalePath := filepath.Join(dir, "shm_beszel_health")
	freshPath := filepath.Join(dir, "tmp_beszel_health")
	useHealthFiles(t, stalePath, freshPath)

	require.NoError(t, os.WriteFile(stalePath, nil, 0o644))
	stale := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(stalePath, stale, stale))
	require.NoError(t, os.WriteFile(freshPath, nil, 0o644))

	assert.NoError(t, Check())
}
