// Package ghupdate implements a new command to self update the current
// executable with the latest GitHub release. This is based on PocketBase's
// ghupdate package with modifications.
package ghupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/henrygd/beszel"

	"github.com/blang/semver"
)

// Minimal color functions using ANSI escape codes
const (
	colorReset  = "\033[0m"
	ColorYellow = "\033[33m"
	ColorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

func ColorPrint(color, text string) {
	fmt.Println(color + text + colorReset)
}

func ColorPrintf(color, format string, args ...interface{}) {
	fmt.Printf(color+format+colorReset+"\n", args...)
}

// HttpClient is a base HTTP client interface (usually used for test purposes).
type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config defines the config options of the ghupdate plugin.
//
// NB! This plugin is considered experimental and its config options may change in the future.
type Config struct {
	// Owner specifies the account owner of the repository (default to "pocketbase").
	Owner string

	// Repo specifies the name of the repository (default to "pocketbase").
	Repo string

	// ArchiveExecutable specifies the name of the executable file in the release archive
	// (default to "pocketbase"; an additional ".exe" check is also performed as a fallback).
	ArchiveExecutable string

	// Optional context to use when fetching and downloading the latest release.
	Context context.Context

	// The HTTP client to use when fetching and downloading the latest release.
	// Defaults to `http.DefaultClient`.
	HttpClient HttpClient

	// The data directory to use when fetching and downloading the latest release.
	DataDir string

	// UseMirror specifies whether to use the beszel.dev mirror instead of GitHub API.
	// When false (default), always uses api.github.com. When true, uses gh.beszel.dev.
	UseMirror bool
}

type updater struct {
	config         Config
	currentVersion string
}

func Update(config Config) (updated bool, err error) {
	p := &updater{
		currentVersion: beszel.Version,
		config:         config,
	}

	return p.update()
}

func (p *updater) update() (updated bool, err error) {
	ColorPrint(ColorYellow, "Fetching release information...")

	if p.config.DataDir == "" {
		p.config.DataDir = os.TempDir()
	}

	if p.config.Owner == "" {
		p.config.Owner = "henrygd"
	}

	if p.config.Repo == "" {
		p.config.Repo = "beszel"
	}

	if p.config.Context == nil {
		p.config.Context = context.Background()
	}

	if p.config.HttpClient == nil {
		p.config.HttpClient = http.DefaultClient
	}

	var latest *release
	var useMirror bool

	// Determine the API endpoint based on UseMirror flag
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", p.config.Owner, p.config.Repo)
	if p.config.UseMirror {
		useMirror = true
		apiURL = fmt.Sprintf("https://gh.beszel.dev/repos/%s/%s/releases/latest?api=true", p.config.Owner, p.config.Repo)
		ColorPrint(ColorYellow, "Using mirror for update.")
	}

	latest, err = fetchLatestRelease(
		p.config.Context,
		p.config.HttpClient,
		apiURL,
	)
	if err != nil {
		return false, err
	}

	currentVersion := semver.MustParse(strings.TrimPrefix(p.currentVersion, "v"))
	newVersion := semver.MustParse(strings.TrimPrefix(latest.Tag, "v"))

	if newVersion.LTE(currentVersion) {
		ColorPrintf(ColorGreen, "You already have the latest version %s.", p.currentVersion)
		return false, nil
	}

	suffix := archiveSuffix(p.config.ArchiveExecutable, runtime.GOOS, runtime.GOARCH)
	asset, err := latest.findAssetBySuffix(suffix)
	if err != nil {
		return false, err
	}

	releaseDir := filepath.Join(p.config.DataDir, ".beszel_update")
	defer os.RemoveAll(releaseDir)

	ColorPrintf(ColorYellow, "Downloading %s...", asset.Name)

	// download the release asset
	assetPath := filepath.Join(releaseDir, asset.Name)
	if err := downloadFile(p.config.Context, p.config.HttpClient, asset.DownloadUrl, assetPath, useMirror); err != nil {
		return false, err
	}

	ColorPrintf(ColorYellow, "Extracting %s...", asset.Name)

	extractDir := filepath.Join(releaseDir, "extracted_"+asset.Name)
	defer os.RemoveAll(extractDir)

	// Extract the archive (automatically detects format)
	if err := extract(assetPath, extractDir); err != nil {
		return false, err
	}

	ColorPrint(ColorYellow, "Replacing the executable...")

	oldExec, err := os.Executable()
	if err != nil {
		return false, err
	}
	renamedOldExec := oldExec + ".old"
	defer os.Remove(renamedOldExec)

	newExec := filepath.Join(extractDir, p.config.ArchiveExecutable)
	if _, err := os.Stat(newExec); err != nil {
		// try again with an .exe extension
		newExec = newExec + ".exe"
		if _, fallbackErr := os.Stat(newExec); fallbackErr != nil {
			return false, fmt.Errorf("the executable in the extracted path is missing or it is inaccessible: %v, %v", err, fallbackErr)
		}
	}

	// rename the current executable
	if err := os.Rename(oldExec, renamedOldExec); err != nil {
		return false, fmt.Errorf("failed to rename the current executable: %w", err)
	}

	tryToRevertExecChanges := func() {
		if revertErr := os.Rename(renamedOldExec, oldExec); revertErr != nil {
			slog.Debug(
				"Failed to revert executable",
				slog.String("old", renamedOldExec),
				slog.String("new", oldExec),
				slog.String("error", revertErr.Error()),
			)
		}
	}

	// replace with the extracted binary
	if err := os.Rename(newExec, oldExec); err != nil {
		// If rename fails due to cross-device link, try copying instead
		if isCrossDeviceError(err) {
			if err := copyFile(newExec, oldExec); err != nil {
				tryToRevertExecChanges()
				return false, fmt.Errorf("failed replacing the executable: %w", err)
			}
		} else {
			tryToRevertExecChanges()
			return false, fmt.Errorf("failed replacing the executable: %w", err)
		}
	}

	ColorPrint(colorGray, "---")
	ColorPrint(ColorGreen, "Update completed successfully!")

	// print the release notes
	if latest.Body != "" {
		fmt.Print("\n")
		releaseNotes := strings.TrimSpace(strings.Replace(latest.Body, "> _To update the prebuilt executable you can run `./"+p.config.ArchiveExecutable+" update`._", "", 1))
		ColorPrint(colorCyan, releaseNotes)
		fmt.Print("\n")
	}

	return true, nil
}

func fetchLatestRelease(
	ctx context.Context,
	client HttpClient,
	url string,
) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	rawBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// http.Client doesn't treat non 2xx responses as error
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf(
			"(%d) failed to fetch latest releases:\n%s",
			res.StatusCode,
			string(rawBody),
		)
	}

	result := &release{}
	if err := json.Unmarshal(rawBody, result); err != nil {
		return nil, err
	}

	return result, nil
}

func downloadFile(
	ctx context.Context,
	client HttpClient,
	url string,
	destPath string,
	useMirror bool,
) error {
	if useMirror {
		url = strings.Replace(url, "github.com", "gh.beszel.dev", 1)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// http.Client doesn't treat non 2xx responses as error
	if res.StatusCode >= 400 {
		return fmt.Errorf("(%d) failed to send download file request", res.StatusCode)
	}

	// ensure that the dest parent dir(s) exist
	if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return err
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, res.Body); err != nil {
		return err
	}

	return nil
}

// isCrossDeviceError checks if the error is due to a cross-device link
func isCrossDeviceError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "cross-device") ||
		strings.Contains(err.Error(), "EXDEV"))
}

// copyFile copies a file from src to dst, preserving permissions
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy the file contents
	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Preserve the original file permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	return destFile.Chmod(sourceInfo.Mode())
}

func archiveSuffix(binaryName, goos, goarch string) string {
	if goos == "windows" {
		return fmt.Sprintf("%s_%s_%s.zip", binaryName, goos, goarch)
	}
	return fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, goos, goarch)
}
