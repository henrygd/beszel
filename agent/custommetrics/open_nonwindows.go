//go:build !windows

package custommetrics

import (
	"os"
	"syscall"
)

// openNonBlocking opens a file for reading without waiting for a writer, so a
// FIFO put where a metrics file was expected opens at once instead of blocking
// collection under the agent lock. Reading a regular file is unaffected.
func openNonBlocking(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
