//go:build !linux

package agent

import (
	"errors"
)

func (gm *GPUManager) hasAmdSysfs() bool {
	return false
}

func (gm *GPUManager) collectAmdStats() error {
	return errors.ErrUnsupported
}
