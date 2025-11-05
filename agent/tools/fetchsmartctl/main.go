package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Download smartctl.exe from the given URL and save it to the given destination.
// This is used to embed smartctl.exe in the Windows build.

func main() {
	url := flag.String("url", "", "URL to download smartctl.exe from (required)")
	out := flag.String("out", "", "Destination path for smartctl.exe (required)")
	sha := flag.String("sha", "", "Optional SHA1/SHA256 checksum for integrity validation")
	force := flag.Bool("force", false, "Force re-download even if destination exists")
	flag.Parse()

	if *url == "" || *out == "" {
		fatalf("-url and -out are required")
	}

	if !*force {
		if info, err := os.Stat(*out); err == nil && info.Size() > 0 {
			fmt.Println("smartctl.exe already present, skipping download")
			return
		}
	}

	if err := downloadFile(*url, *out, *sha); err != nil {
		fatalf("download failed: %v", err)
	}
}

func downloadFile(url, dest, shaHex string) error {
	// Prepare destination
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	// HTTP client
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", "beszel-fetchsmartctl/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}

	// Determine hash algorithm based on length (SHA1=40, SHA256=64)
	var hasher hash.Hash
	if shaHex := strings.TrimSpace(shaHex); shaHex != "" {
		cleanSha := strings.ToLower(strings.ReplaceAll(shaHex, " ", ""))
		switch len(cleanSha) {
		case 40:
			hasher = sha1.New()
		case 64:
			hasher = sha256.New()
		default:
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("unsupported hash length: %d (expected 40 for SHA1 or 64 for SHA256)", len(cleanSha))
		}
	}

	var mw io.Writer = f
	if hasher != nil {
		mw = io.MultiWriter(f, hasher)
	}
	if _, err := io.Copy(mw, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close tmp: %w", err)
	}

	if hasher != nil && shaHex != "" {
		cleanSha := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(shaHex), " ", ""))
		got := strings.ToLower(hex.EncodeToString(hasher.Sum(nil)))
		if got != cleanSha {
			os.Remove(tmp)
			return fmt.Errorf("hash mismatch: got %s want %s", got, cleanSha)
		}
	}

	// Make executable and move into place
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	fmt.Println("smartctl.exe downloaded to", dest)
	return nil
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
