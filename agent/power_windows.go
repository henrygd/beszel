//go:build windows

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
	case common.PowerActionShutdown, common.PowerActionReboot, common.PowerActionSuspend, common.PowerActionHibernate:
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
		name, args = "shutdown.exe", []string{"/s", "/t", "0"}
	case common.PowerActionReboot:
		name, args = "shutdown.exe", []string{"/r", "/t", "0"}
	case common.PowerActionHibernate:
		name, args = "shutdown.exe", []string{"/h"}
	case common.PowerActionSuspend:
		name, args = "rundll32.exe", []string{"powrprof.dll,SetSuspendState", "0,1,0"}
	default:
		return fmt.Errorf("unsupported power action %q", action)
	}
	if err := exec.Command(name, args...).Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}
