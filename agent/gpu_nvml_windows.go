//go:build windows

package agent

import (
	"golang.org/x/sys/windows"
)

func openLibrary(name string) (uintptr, error) {
	handle, err := windows.LoadLibrary(name)
	return uintptr(handle), err
}

func getNVMLPath() string {
	return "nvml.dll"
}

func hasSymbol(lib uintptr, symbol string) bool {
	_, err := windows.GetProcAddress(windows.Handle(lib), symbol)
	return err == nil
}

func (c *nvmlCollector) isGPUActive(bdf string) bool {
	return true
}
