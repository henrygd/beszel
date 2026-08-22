//go:build !linux && !windows && !darwin

package agent

func getNetworkInterfaceSpeeds() map[string]uint64 {
	return nil
}
