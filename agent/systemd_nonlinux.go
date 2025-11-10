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

func (sm *systemdManager) getServiceDetails(string) (systemd.ServiceDetails, error) {
	return nil, errors.New("systemd manager unavailable")
}
