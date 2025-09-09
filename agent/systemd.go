//go:build linux

package agent

import (
	"context"
	"log/slog"
	"strings"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/coreos/go-systemd/v22/dbus"
)

func getSystemdServices() []system.SystemdService {
	// First, try to connect to the system bus
	conn, err := dbus.NewSystemdConnectionContext(context.Background())
	if err != nil {
		// If there's an error, check if it's a permission issue
		if strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "denied") {
			slog.Info("Systemd system bus permission denied, falling back to user bus.")
			// Try connecting to the user bus instead
			conn, err = dbus.NewUserConnectionContext(context.Background())
			if err != nil {
				slog.Error("Error connecting to systemd user bus", "err", err)
				return nil
			}
		} else {
			// It's some other error (e.g., systemd not running)
			slog.Error("Error connecting to systemd system bus", "err", err)
			return nil
		}
	}
	defer conn.Close()

	units, err := conn.ListUnitsContext(context.Background())
	if err != nil {
		slog.Error("Error listing systemd units", "err", err)
		return nil
	}

	var services []system.SystemdService
	for _, unit := range units {
		if strings.HasSuffix(unit.Name, ".service") {
			services = append(services, system.SystemdService{
				Name:   unit.Name,
				Status: unit.ActiveState,
			})
		}
	}
	return services
}
