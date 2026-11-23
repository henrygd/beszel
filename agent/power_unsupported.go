//go:build !linux && !windows && !darwin && !freebsd

package agent

import (
	"errors"

	"github.com/henrygd/beszel/internal/common"
)

func powerActionsEnabled() bool                    { return false }
func powerActionSupported(common.PowerAction) bool { return false }
func runPowerAction(common.PowerAction) error {
	return errors.New("power actions are only supported on Linux")
}
