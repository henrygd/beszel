//go:build testing && !windows

package custommetrics

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A FIFO or directory where a metrics file is expected is skipped, never
// opened: opening a FIFO blocks until something writes to it, and Collect
// runs under the agent lock.
func TestCollectorSkipsNonRegularFiles(t *testing.T) {
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/m.d
    - path: {dir}/g/*
    - path: {dir}/pipe.prom
`)
	f.write("m.d/a.prom", "a 1\n", 0)
	require.NoError(t, syscall.Mkfifo(filepath.Join(f.dir, "m.d", "fifo.prom"), 0o644), "in a directory source")
	f.write("g/b.prom", "b 2\n", 0)
	require.NoError(t, os.Mkdir(filepath.Join(f.dir, "g", "sub"), 0o755), "matched by a glob")
	require.NoError(t, syscall.Mkfifo(filepath.Join(f.dir, "pipe.prom"), 0o644), "named by a file source")

	done := make(chan map[string]float64, 1)
	go func() {
		values, _ := f.collect(60000, 0)
		done <- values
	}()
	select {
	case values := <-done:
		assert.Equal(t, map[string]float64{"a": 1, "b": 2}, values)
	case <-time.After(5 * time.Second):
		t.Fatal("Collect blocked on a FIFO")
	}
	assert.Empty(t, *f.warnings, "skipped without a warning")
}

// unblockFifo opens a FIFO for writing, releasing a reader blocked opening it,
// so a failed test can finish.
func unblockFifo(path string) {
	if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
		_ = w.Close()
	}
}

// readRegular checks the file it has open, so a FIFO is refused at once,
// without waiting for a writer.
func TestReadRegularRejectsFifo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe.prom")
	require.NoError(t, syscall.Mkfifo(path, 0o644))
	done := make(chan error, 1)
	go func() {
		_, _, err := readRegular(path, maxFileSize)
		done <- err
	}()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, errNotRegular)
	case <-time.After(5 * time.Second):
		unblockFifo(path)
		t.Fatal("readRegular blocked on a FIFO")
	}
}

// A producer can rename a FIFO over a metrics file at any moment, including
// between Collect's stat and its open. Collect must never wait on it.
func TestCollectorFifoSwappedIn(t *testing.T) {
	f := newFixture(t, "metrics:\n  sources:\n    - path: {dir}/m.prom\n")
	target := filepath.Join(f.dir, "m.prom")
	regular := filepath.Join(f.dir, "regular.tmp")
	fifo := filepath.Join(f.dir, "fifo.tmp")
	var stop atomic.Bool
	stopped := make(chan struct{})
	go func() { // a broken or hostile producer, alternating what it renames into place
		defer close(stopped)
		for !stop.Load() {
			_ = os.WriteFile(regular, []byte("m 1\n"), 0o644)
			_ = os.Rename(regular, target)
			_ = syscall.Mkfifo(fifo, 0o644)
			_ = os.Rename(fifo, target)
		}
	}()
	t.Cleanup(func() {
		stop.Store(true)
		<-stopped
	})

	// Before the fix, Collect blocked within a thousand calls.
	for calls := range 10000 {
		done := make(chan struct{})
		go func() {
			f.c.Collect(60000, time.Now())
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("Collect blocked on a FIFO after %d calls", calls)
		}
	}
}

// A config path that is a FIFO is not waited on either: the feature stays off,
// and one warning says why.
func TestConfigFileFifo(t *testing.T) {
	warnings := captureWarnings(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, syscall.Mkfifo(path, 0o644))
	f := &configFile{path: path, explicit: true}
	done := make(chan struct{})
	go func() {
		for range 3 {
			f.refresh()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		unblockFifo(path)
		t.Fatal("refresh blocked on a FIFO named as the config")
	}
	assert.Nil(t, f.cfg)
	require.Len(t, *warnings, 1, "%v", *warnings)
	assert.Contains(t, (*warnings)[0], "Custom metrics config not loaded")
	assert.Contains(t, (*warnings)[0], "not a regular file")
}

// The likeliest real fault is permissions: a producer writing a file the
// agent's user cannot read. A file or directory that cannot be read is
// skipped with one warning an hour, and everything readable is still
// reported, including through symlinks to a file or a directory.
func TestCollectorUnreadableAndLinkedFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files whatever their permissions")
	}
	f := newFixture(t, `metrics:
  sources:
    - path: {dir}/m.d
    - path: {dir}/linked.d
    - path: {dir}/locked.d
`)
	f.write("m.d/ok.prom", "ok 1\n", 0)
	secret := f.write("m.d/secret.prom", "secret 2\n", 0)
	require.NoError(t, os.Chmod(secret, 0))
	dangling := filepath.Join(f.dir, "m.d", "dangling.prom")
	require.NoError(t, os.Symlink(filepath.Join(f.dir, "nowhere.prom"), dangling))
	target := f.write("elsewhere/target.txt", "via_file_link 3\n", 0)
	require.NoError(t, os.Symlink(target, filepath.Join(f.dir, "m.d", "link.prom")))
	f.write("real.d/r.prom", "via_dir_link 4\n", 0)
	require.NoError(t, os.Symlink(filepath.Join(f.dir, "real.d"), filepath.Join(f.dir, "linked.d")))
	f.write("locked.d/l.prom", "locked 5\n", 0)
	locked := filepath.Join(f.dir, "locked.d")
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // so the temporary directory can be removed

	for range 2 {
		values, _ := f.collect(60000, time.Second)
		assert.Equal(t, map[string]float64{"ok": 1, "via_file_link": 3, "via_dir_link": 4}, values)
	}
	assert.Equal(t, 1, f.warned("Custom metrics file not readable file "+secret))
	assert.Equal(t, 1, f.warned("Custom metrics file not readable file "+dangling))
	assert.Equal(t, 1, f.warned("Custom metrics source not readable path "+locked))
	assert.Len(t, *f.warnings, 3, "%v", *f.warnings)
}
