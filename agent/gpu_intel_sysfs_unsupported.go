//go:build !linux

package agent

type intelSysfsEnergySnapshot struct{}

func (gm *GPUManager) hasIntelSysfs() bool {
	return false
}

func (gm *GPUManager) startIntelSysfsCollector() bool {
	return false
}
