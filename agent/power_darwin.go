//go:build darwin

package agent

import (
	"fmt"
	"os/exec"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"
)

func powerActionsEnabled() bool {
	enabled, _ := utils.GetEnv("POWER_COMMANDS_ENABLED")
	return enabled == "true"
}

func powerActionSupported(action common.PowerAction) bool {
	switch action {
	case common.PowerActionShutdown, common.PowerActionReboot, common.PowerActionSuspend:
		return true
	default:
		return false
	}
}

func runPowerAction(action common.PowerAction) error {
	var name string
	var args []string
	switch action {
	case common.PowerActionShutdown:
		name, args = "shutdown", []string{"-h", "now"}
	case common.PowerActionReboot:
		name, args = "shutdown", []string{"-r", "now"}
	case common.PowerActionSuspend:
		name, args = "pmset", []string{"sleepnow"}
	default:
		return fmt.Errorf("unsupported power action %q", action)
	}
	if err := exec.Command(name, args...).Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}
