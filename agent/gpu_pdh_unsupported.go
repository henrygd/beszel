//go:build !(windows && (amd64 || arm64))

package agent

func (gm *GPUManager) hasPdhGpu() bool {
	return false
}

func (gm *GPUManager) startPdhCollector() bool {
	return false
}
