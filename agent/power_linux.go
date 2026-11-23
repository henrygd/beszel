//go:build linux

package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"
)

func powerActionsEnabled() bool {
	enabled, _ := utils.GetEnv("POWER_COMMANDS_ENABLED")
	return enabled == "true" && isSystemdAvailable()
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
	targets := map[common.PowerAction]string{
		common.PowerActionShutdown:  "poweroff.target",
		common.PowerActionReboot:    "reboot.target",
		common.PowerActionSuspend:   "suspend.target",
		common.PowerActionHibernate: "hibernate.target",
	}
	target, ok := targets[action]
	if !ok {
		return fmt.Errorf("unsupported power action %q", action)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer conn.Close()
	if _, err := conn.StartUnitContext(ctx, target, "replace", nil); err != nil {
		return fmt.Errorf("start %s: %w", target, err)
	}
	return nil
}
