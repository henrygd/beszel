package ghupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandleSELinuxContext_NoSELinux(t *testing.T) {
	// On systems without SELinux, getenforce will fail and the function
	// should return nil without error
	if _, err := exec.LookPath("getenforce"); err != nil {
		tempFile := filepath.Join(t.TempDir(), "test-binary")
		if err := os.WriteFile(tempFile, []byte("test"), 0755); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		err := HandleSELinuxContext(tempFile)
		if err != nil {
			t.Errorf("HandleSELinuxContext() on non-SELinux system returned error: %v", err)
		}
	}
}

func TestHandleSELinuxContext_InvalidPath(t *testing.T) {
	// Even with an invalid path, the function should handle gracefully
	// on non-SELinux systems (getenforce fails early)
	if _, err := exec.LookPath("getenforce"); err != nil {
		err := HandleSELinuxContext("/nonexistent/path/binary")
		if err != nil {
			t.Errorf("HandleSELinuxContext() with invalid path on non-SELinux system returned error: %v", err)
		}
	}
}
