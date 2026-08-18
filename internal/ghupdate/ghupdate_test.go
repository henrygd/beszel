package ghupdate

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveSuffix(t *testing.T) {
	tests := []struct {
		name                 string
		binary, goos, goarch string
		goarm, want          string
	}{
		{"armv5 agent", "beszel-agent", "linux", "arm", "5", "beszel-agent_linux_armv5.tar.gz"},
		{"armv6 keeps legacy name", "beszel-agent", "linux", "arm", "6", "beszel-agent_linux_arm.tar.gz"},
		{"hub keeps legacy arm name", "beszel", "linux", "arm", "6", "beszel_linux_arm.tar.gz"},
		{"armv7 agent", "beszel-agent", "linux", "arm", "7", "beszel-agent_linux_armv7.tar.gz"},
		{"newer arm keeps legacy name", "beszel-agent", "linux", "arm", "8", "beszel-agent_linux_arm.tar.gz"},
		{"unknown arm keeps legacy name", "beszel-agent", "linux", "arm", "", "beszel-agent_linux_arm.tar.gz"},
		{"amd64 hub", "beszel", "linux", "amd64", "", "beszel_linux_amd64.tar.gz"},
		{"windows", "beszel-agent", "windows", "amd64", "", "beszel-agent_windows_amd64.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := archiveSuffix(tt.binary, tt.goos, tt.goarch, tt.goarm); got != tt.want {
				t.Errorf("archiveSuffix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReleaseFindAssetBySuffix(t *testing.T) {
	r := release{
		Assets: []*releaseAsset{
			{Name: "test1.zip", Id: 1},
			{Name: "test2.zip", Id: 2},
			{Name: "test22.zip", Id: 22},
			{Name: "test3.zip", Id: 3},
		},
	}

	asset, err := r.findAssetBySuffix("2.zip")
	if err != nil {
		t.Fatalf("Expected nil, got err: %v", err)
	}

	if asset.Id != 2 {
		t.Fatalf("Expected asset with id %d, got %v", 2, asset)
	}
}

func TestExtractFailure(t *testing.T) {
	testDir := t.TempDir()

	// Test with missing zip file
	missingZipPath := filepath.Join(testDir, "missing_test.zip")
	extractedPath := filepath.Join(testDir, "zip_extract")

	if err := extract(missingZipPath, extractedPath); err == nil {
		t.Fatal("Expected Extract to fail due to missing zip file")
	}

	// Test with missing tar.gz file
	missingTarPath := filepath.Join(testDir, "missing_test.tar.gz")

	if err := extract(missingTarPath, extractedPath); err == nil {
		t.Fatal("Expected Extract to fail due to missing tar.gz file")
	}
}

func TestArchivePath(t *testing.T) {
	destDir := t.TempDir()
	for _, name := range []string{
		"",
		"..",
		filepath.Join("..", "file"),
		filepath.Join("dir", "..", "..", "file"),
		string(os.PathSeparator) + filepath.Join("tmp", "file"),
	} {
		if _, err := archivePath(destDir, name); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}

	name := filepath.Join("dir", "file")
	if path, err := archivePath(destDir, name); err != nil || path != filepath.Join(destDir, name) {
		t.Errorf("archivePath(%q) = %q, %v", name, path, err)
	}
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	testDir := t.TempDir()
	archivePath := filepath.Join(testDir, "malicious.tar.gz")
	destDir := filepath.Join(testDir, "extract")
	escapedPath := filepath.Join(testDir, "escaped")

	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../escaped", Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err := extract(archivePath, destDir); err == nil {
		t.Fatal("expected path traversal archive to be rejected")
	}
	if _, err := os.Stat(escapedPath); !os.IsNotExist(err) {
		t.Fatalf("path traversal wrote %s", escapedPath)
	}
}
