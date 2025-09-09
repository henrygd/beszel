//go:build !linux

package agent

import "github.com/henrygd/beszel/internal/entities/system"

func getSystemdServices() []system.SystemdService {
	return nil
}
