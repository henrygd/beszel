//go:build !linux || !(amd64 || arm64 || arm || 386 || riscv64 || loong64 || s390x)

package ipmi

import "errors"

// Open is only implemented on Linux.
func Open() (*Collector, error) {
	return nil, errors.ErrUnsupported
}
