//go:build !linux

package agent

import (
	"errors"

	"github.com/henrygd/beszel/internal/entities/systemd"
)

// systemdManager manages the collection of systemd service statistics.
type systemdManager struct {
	hasFreshStats bool
}

// newSystemdManager creates a new systemdManager.
func newSystemdManager() (*systemdManager, error) {
	return &systemdManager{}, nil
}

// getServiceStats returns nil for non-linux systems.
func (sm *systemdManager) getServiceStats(conn any, refresh bool) []*systemd.Service {
	return nil
}

// getServiceStatsCount returns 0 for non-linux systems.
func (sm *systemdManager) getServiceStatsCount() int {
	return 0
}

// getFailedServiceCount returns 0 for non-linux systems.
func (sm *systemdManager) getFailedServiceCount() uint16 {
	return 0
}

func (sm *systemdManager) getServiceDetails(string) (systemd.ServiceDetails, error) {
	return nil, errors.New("systemd manager unavailable")
}
