//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPhysicalNetworkInterfaceAt(t *testing.T) {
	sysfsRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sysfsRoot, "eth0", "device"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sysfsRoot, "veth1001i0"), 0o755); err != nil {
		t.Fatal(err)
	}

	if !isPhysicalNetworkInterfaceAt(sysfsRoot, "eth0") {
		t.Fatal("expected device-backed interface to be included")
	}
	if isPhysicalNetworkInterfaceAt(sysfsRoot, "veth1001i0") {
		t.Fatal("expected software-only interface to be excluded")
	}
	if isPhysicalNetworkInterfaceAt(sysfsRoot, "lo") {
		t.Fatal("expected interface without a device path to be excluded")
	}
}
