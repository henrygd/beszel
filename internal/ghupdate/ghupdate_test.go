package ghupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver"
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

func TestReleaseVersion(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{"with v prefix", "v0.19.0", "0.19.0"},
		{"without prefix", "0.19.0", "0.19.0"},
		{"prerelease", "v0.19.0-beta1", "0.19.0-beta1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := releaseVersion(&release{Tag: tt.tag})
			if err != nil {
				t.Fatalf("releaseVersion(%q) returned error: %v", tt.tag, err)
			}
			if v.String() != tt.want {
				t.Errorf("releaseVersion(%q) = %q, want %q", tt.tag, v.String(), tt.want)
			}
		})
	}

	if _, err := releaseVersion(&release{Tag: "not-a-version"}); err == nil {
		t.Error("expected error for invalid tag")
	}
}

func TestCapVersion(t *testing.T) {
	latest := semver.MustParse("4.0.0")
	tests := []struct {
		name      string
		max       string
		want      string
		wantCapped bool
		wantErr   bool
	}{
		{"no max version means no cap", "", "4.0.0", false, false},
		{"max version above latest is ignored", "5.0.0", "4.0.0", false, false},
		{"max version equal to latest is ignored", "4.0.0", "4.0.0", false, false},
		{"max version below latest caps", "3.2.0", "3.2.0", true, false},
		{"max version with v prefix caps", "v3.2.0", "3.2.0", true, false},
		{"prerelease max version caps", "3.2.0-beta.1", "3.2.0-beta.1", true, false},
		{"invalid max version errors", "abc", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, capped, err := capVersion(latest, tt.max)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if capped != tt.wantCapped {
				t.Errorf("capped = %v, want %v", capped, tt.wantCapped)
			}
			if got.String() != tt.want {
				t.Errorf("result version = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestGetAPIURLs(t *testing.T) {
	latestURLs := map[bool]string{
		false: "https://api.github.com/repos/henrygd/beszel/releases/latest",
		true:  "https://gh.beszel.dev/repos/henrygd/beszel/releases/latest?api=true",
	}
	for mirror, want := range latestURLs {
		if got := getApiURL(mirror, "henrygd", "beszel"); got != want {
			t.Errorf("getApiURL(%v) = %q, want %q", mirror, got, want)
		}
	}

	tagURLs := map[bool]string{
		false: "https://api.github.com/repos/henrygd/beszel/releases/tags/v3.2.0",
		true:  "https://gh.beszel.dev/repos/henrygd/beszel/releases/tags/v3.2.0?api=true",
	}
	for mirror, want := range tagURLs {
		if got := getTagReleaseURL(mirror, "henrygd", "beszel", "v3.2.0"); got != want {
			t.Errorf("getTagReleaseURL(%v) = %q, want %q", mirror, got, want)
		}
	}
}

func TestFetchReleaseByTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/henrygd/beszel/releases/tags/v3.2.0" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v3.2.0","assets":[]}`)
	}))
	defer server.Close()

	rel, err := FetchLatestRelease(context.Background(), &http.Client{}, server.URL+"/repos/henrygd/beszel/releases/tags/v3.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.Tag != "v3.2.0" {
		t.Errorf("tag = %q, want %q", rel.Tag, "v3.2.0")
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
