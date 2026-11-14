//go:build windows && testing

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyEmbeddedDirReplacesOutdatedFiles(t *testing.T) {
	src := fstest.MapFS{
		"lhm/beszel_lhm.exe":               {Data: []byte("new helper")},
		"lhm/LibreHardwareMonitorLib.dll":  {Data: []byte("new library")},
		"lhm/unchanged.dll":                {Data: []byte("same")},
		"lhm/sub/LibreHardwareMonitor.xml": {Data: []byte("docs")},
	}
	dest := t.TempDir()
	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(dest, name), []byte(content), 0o755))
	}
	// an older helper extracted by a previous agent version
	write("beszel_lhm.exe", "old helper")
	write("LibreHardwareMonitorLib.dll", "old library")
	write("unchanged.dll", "same")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(filepath.Join(dest, "unchanged.dll"), old, old))

	require.NoError(t, copyEmbeddedDir(src, "lhm", dest))

	for name, want := range map[string]string{
		"beszel_lhm.exe":               "new helper",
		"LibreHardwareMonitorLib.dll":  "new library",
		"unchanged.dll":                "same",
		"sub/LibreHardwareMonitor.xml": "docs",
	} {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		require.NoError(t, err)
		assert.Equal(t, want, string(got), name)
	}

	// files that are already up to date are not rewritten
	info, err := os.Stat(filepath.Join(dest, "unchanged.dll"))
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old), "unchanged file was rewritten")
}
