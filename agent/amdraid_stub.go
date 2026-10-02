//go:build !windows

package agent

func scanAmdRaidDevices() []*DeviceInfo {
	return nil
}

func (sm *SmartManager) collectAmdRaidHealth(deviceInfo *DeviceInfo) (bool, error) {
	return false, nil
}
