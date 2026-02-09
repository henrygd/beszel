//go:build !linux

package agent

// Non-Linux builds: eMMC health via sysfs is not available.

func scanEmmcDevices() []*DeviceInfo {
	return nil
}

func (sm *SmartManager) collectEmmcHealth(deviceInfo *DeviceInfo) (bool, error) {
	return false, nil
}

