//go:build !linux

package agent

import "github.com/henrygd/beszel/internal/entities/systemd"

// systemdManager manages the collection of systemd service statistics.
type systemdManager struct{}

// newSystemdManager creates a new systemdManager.
func newSystemdManager() (*systemdManager, error) {
	return &systemdManager{}, nil
}

// getServiceStats returns nil for non-linux systems.
func (sm *systemdManager) getServiceStats() []*systemd.Service {
	return nil
}
