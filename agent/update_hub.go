package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/ghupdate"
)

type hubAgentMeta struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

func hubBaseURL() string {
	u := strings.TrimSpace(os.Getenv("HUB_URL"))
	return strings.TrimRight(u, "/")
}

func localFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// updateFromHub downloads the hub-hosted agent binary when SHA differs.
func updateFromHub(hubURL string) (updated bool, err error) {
	exePath, err := os.Executable()
	if err != nil {
		return false, err
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	metaURL := fmt.Sprintf("%s/api/beszel/agent/meta?os=%s&arch=%s", hubURL, runtime.GOOS, runtime.GOARCH)
	req, err := http.NewRequest(http.MethodGet, metaURL, nil)
	if err != nil {
		return false, err
	}
	res, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return false, fmt.Errorf("hub meta %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var meta hubAgentMeta
	if err := json.NewDecoder(res.Body).Decode(&meta); err != nil {
		return false, err
	}
	if meta.SHA256 == "" {
		return false, fmt.Errorf("hub meta missing sha256")
	}

	cur, err := localFileSHA256(exePath)
	if err == nil && strings.EqualFold(cur, meta.SHA256) {
		ghupdate.ColorPrint(ghupdate.ColorGreen, "Already up to date (hub).")
		return false, nil
	}

	binURL := fmt.Sprintf("%s/api/beszel/agent/binary?os=%s&arch=%s", hubURL, runtime.GOOS, runtime.GOARCH)
	ghupdate.ColorPrintf(ghupdate.ColorYellow, "Downloading agent from hub (%s)...\n", hubURL)
	binReq, err := http.NewRequest(http.MethodGet, binURL, nil)
	if err != nil {
		return false, err
	}
	binRes, err := client.Do(binReq)
	if err != nil {
		return false, err
	}
	defer binRes.Body.Close()
	if binRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(binRes.Body, 512))
		return false, fmt.Errorf("hub binary %d: %s", binRes.StatusCode, strings.TrimSpace(string(body)))
	}

	tmp := exePath + ".hub-new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return false, err
	}
	hasher := sha256.New()
	w := io.MultiWriter(out, hasher)
	if _, err := io.Copy(w, binRes.Body); err != nil {
		out.Close()
		os.Remove(tmp)
		return false, err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return false, err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(sum, meta.SHA256) {
		os.Remove(tmp)
		return false, fmt.Errorf("checksum mismatch: got %s want %s", sum, meta.SHA256)
	}

	bak := exePath + ".bak"
	_ = os.Remove(bak)
	if err := os.Rename(exePath, bak); err != nil {
		// may fail if busy; try direct overwrite via rename tmp
		if err2 := os.Rename(tmp, exePath); err2 != nil {
			os.Remove(tmp)
			return false, err
		}
	} else {
		if err := os.Rename(tmp, exePath); err != nil {
			_ = os.Rename(bak, exePath)
			os.Remove(tmp)
			return false, err
		}
	}

	_ = os.Chmod(exePath, 0755)
	if chownPath, err := exec.LookPath("chown"); err == nil {
		_ = exec.Command(chownPath, "beszel:beszel", exePath).Run()
	}
	_ = ghupdate.HandleSELinuxContext(exePath)

	ghupdate.ColorPrintf(ghupdate.ColorGreen, "Updated from hub (sha %s).\n", sum[:12])
	return true, nil
}
