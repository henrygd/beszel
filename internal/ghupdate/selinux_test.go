package ghupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandleSELinuxContext_NoSELinux(t *testing.T) {
	// Skip on SELinux systems - this test is for non-SELinux behavior
	if _, err := exec.LookPath("getenforce"); err == nil {
		t.Skip("skipping on SELinux-enabled system")
	}

	// On systems without SELinux, getenforce will fail and the function
	// should return nil without error
	tempFile := filepath.Join(t.TempDir(), "test-binary")
	if err := os.WriteFile(tempFile, []byte("test"), 0755); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	err := HandleSELinuxContext(tempFile)
	if err != nil {
		t.Errorf("HandleSELinuxContext() on non-SELinux system returned error: %v", err)
	}
}

func TestHandleSELinuxContext_InvalidPath(t *testing.T) {
	// Skip on SELinux systems - this test is for non-SELinux behavior
	if _, err := exec.LookPath("getenforce"); err == nil {
		t.Skip("skipping on SELinux-enabled system")
	}

	// On non-SELinux systems, getenforce fails early so even invalid paths succeed
	err := HandleSELinuxContext("/nonexistent/path/binary")
	if err != nil {
		t.Errorf("HandleSELinuxContext() with invalid path on non-SELinux system returned error: %v", err)
	}
}

func TestTrySemanageRestorecon_NoTools(t *testing.T) {
	// Skip if semanage is available (we don't want to modify system SELinux policy)
	if _, err := exec.LookPath("semanage"); err == nil {
		t.Skip("skipping on system with semanage available")
	}

	// Should return false when semanage is not available
	result := trySemanageRestorecon("/some/path")
	if result {
		t.Error("trySemanageRestorecon() returned true when semanage is not available")
	}
}
