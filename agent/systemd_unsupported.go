//go:build !linux

package agent

import "github.com/henrygd/beszel/src/entities/system"

func getSystemdServices() []system.SystemdService {
	return nil
}
