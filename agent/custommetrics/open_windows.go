//go:build windows

package custommetrics

import "os"

// openNonBlocking opens a file for reading. Windows keeps named pipes out of
// the file system, so opening a path where a file is expected cannot block.
func openNonBlocking(path string) (*os.File, error) {
	return os.Open(path)
}
