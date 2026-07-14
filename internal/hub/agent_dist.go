package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/pocketbase/pocketbase/core"
)

// agentBinaryPath resolves the custom agent binary for os/arch.
// Looks in dataDir/agent-binaries then /agent-binaries (image path).
func (h *Hub) agentBinaryPath(goos, goarch string) string {
	name := fmt.Sprintf("beszel-agent_%s_%s", goos, goarch)
	candidates := []string{
		filepath.Join(h.DataDir(), "agent-binaries", name),
		filepath.Join(h.DataDir(), "agent-binaries", "beszel-agent"),
		filepath.Join("/agent-binaries", name),
		filepath.Join("/agent-binaries", "beszel-agent"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
			return p
		}
	}
	return ""
}

func normalizeAgentOSArch(goos, goarch string) (string, string) {
	goos = strings.ToLower(strings.TrimSpace(goos))
	goarch = strings.ToLower(strings.TrimSpace(goarch))
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = "amd64"
	}
	switch goarch {
	case "x86_64", "x64":
		goarch = "amd64"
	case "aarch64":
		goarch = "arm64"
	}
	return goos, goarch
}

func fileSHA256(path string) (string, error) {
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

// getAgentMeta returns version + sha256 for the hub-hosted agent binary.
func (h *Hub) getAgentMeta(e *core.RequestEvent) error {
	goos, goarch := normalizeAgentOSArch(e.Request.URL.Query().Get("os"), e.Request.URL.Query().Get("arch"))
	path := h.agentBinaryPath(goos, goarch)
	if path == "" {
		return e.JSON(http.StatusNotFound, map[string]any{
			"error": "agent binary not found on hub; place beszel-agent_linux_amd64 under agent-binaries/",
			"os":    goos,
			"arch":  goarch,
		})
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return e.InternalServerError("hash failed", err)
	}
	st, _ := os.Stat(path)
	return e.JSON(http.StatusOK, map[string]any{
		"version":  beszel.Version,
		"sha256":   sum,
		"size":     st.Size(),
		"os":       goos,
		"arch":     goarch,
		"filename": filepath.Base(path),
	})
}

// downloadAgentBinary streams the hub-hosted agent binary.
func (h *Hub) downloadAgentBinary(e *core.RequestEvent) error {
	goos, goarch := normalizeAgentOSArch(e.Request.URL.Query().Get("os"), e.Request.URL.Query().Get("arch"))
	path := h.agentBinaryPath(goos, goarch)
	if path == "" {
		return e.NotFoundError("agent binary not found", nil)
	}
	e.Response.Header().Set("Content-Type", "application/octet-stream")
	e.Response.Header().Set("Content-Disposition", `attachment; filename="beszel-agent"`)
	e.Response.Header().Set("X-Beszel-Version", beszel.Version)
	if sum, err := fileSHA256(path); err == nil {
		e.Response.Header().Set("X-Beszel-SHA256", sum)
	}
	return e.FileFS(os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// getAgentInstallScript serves a Linux install script that installs the hub-hosted agent.
func (h *Hub) getAgentInstallScript(e *core.RequestEvent) error {
	// Default hub URL for script self-reference; client may still pass -url.
	hubURL := strings.TrimRight(h.appURL, "/")
	if hubURL == "" {
		// best-effort from request
		scheme := "https"
		if e.Request.TLS == nil {
			// respect reverse proxy headers if present
			if xf := e.Request.Header.Get("X-Forwarded-Proto"); xf != "" {
				scheme = xf
			} else {
				scheme = "http"
			}
		}
		host := e.Request.Host
		if host == "" {
			host = "127.0.0.1:8090"
		}
		hubURL = scheme + "://" + host
	}
	script := agentInstallScript(hubURL)
	e.Response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	e.Response.Header().Set("Cache-Control", "no-store")
	return e.String(http.StatusOK, script)
}

func agentInstallScript(defaultHubURL string) string {
	return `#!/bin/sh
set -e
# Install beszel-agent from this Hub (includes latency probing).
PORT=45876
KEY=""
TOKEN=""
HUB_URL="` + defaultHubURL + `"
AUTO_UPDATE=true
UNINSTALL=false

usage() {
  echo "Usage: $0 -k KEY -t TOKEN -url HUB_URL [-p PORT] [--auto-update true|false] [--uninstall]"
  exit 0
}

while [ $# -gt 0 ]; do
  case "$1" in
    -k) shift; KEY="$1" ;;
    -p) shift; PORT="$1" ;;
    -t) shift; TOKEN="$1" ;;
    -url) shift; HUB_URL="$1" ;;
    --auto-update) shift; AUTO_UPDATE="$1" ;;
    --china-mirrors) ;; # accepted for UI compat, ignored
    --uninstall|-u) UNINSTALL=true ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1" >&2; usage ;;
  esac
  shift
done

HUB_URL=$(echo "$HUB_URL" | sed 's:/*$::')
BIN_DIR=/opt/beszel-agent
BIN_PATH="$BIN_DIR/beszel-agent"
AGENT_USER=beszel

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root" >&2
  exit 1
fi

if [ "$UNINSTALL" = true ]; then
  systemctl disable --now beszel-agent.service 2>/dev/null || true
  systemctl disable --now beszel-agent-update.timer 2>/dev/null || true
  rm -f /etc/systemd/system/beszel-agent.service /etc/systemd/system/beszel-agent-update.service /etc/systemd/system/beszel-agent-update.timer
  systemctl daemon-reload 2>/dev/null || true
  rm -rf "$BIN_DIR"
  userdel "$AGENT_USER" 2>/dev/null || true
  echo "Uninstalled beszel-agent"
  exit 0
fi

if [ -z "$KEY" ] || [ -z "$TOKEN" ] || [ -z "$HUB_URL" ]; then
  echo "KEY, TOKEN and HUB_URL are required" >&2
  exit 1
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  armv7l) ARCH=arm ;;
  *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then apt-get update -y && apt-get install -y curl
  elif command -v yum >/dev/null 2>&1; then yum install -y curl
  else echo "curl required" >&2; exit 1; fi
fi

if ! id "$AGENT_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$AGENT_USER" 2>/dev/null \
    || useradd -r -s /sbin/nologin "$AGENT_USER" 2>/dev/null \
    || true
fi

mkdir -p "$BIN_DIR"
chown "$AGENT_USER:$AGENT_USER" "$BIN_DIR"
chmod 755 "$BIN_DIR"

echo "Downloading agent from hub ($HUB_URL) ..."
TMP=$(mktemp)
curl -fL --retry 3 --retry-delay 2 --connect-timeout 15 \
  "$HUB_URL/api/beszel/agent/binary?os=$OS&arch=$ARCH" -o "$TMP"
chmod 755 "$TMP"
if [ -f "$BIN_PATH" ]; then cp "$BIN_PATH" "$BIN_PATH.bak"; fi
mv "$TMP" "$BIN_PATH"
chown "$AGENT_USER:$AGENT_USER" "$BIN_PATH"
chmod 755 "$BIN_PATH"

# Grant docker.sock group if present
if [ -S /var/run/docker.sock ]; then
  DOCKER_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || true)
  if [ -n "$DOCKER_GID" ]; then
    groupadd -g "$DOCKER_GID" docker 2>/dev/null || true
    usermod -aG docker "$AGENT_USER" 2>/dev/null || true
  fi
fi

cat >/etc/systemd/system/beszel-agent.service <<EOF
[Unit]
Description=Beszel Agent Service
Wants=network-online.target
After=network-online.target

[Service]
Environment="PORT=$PORT"
Environment="KEY=$KEY"
Environment="TOKEN=$TOKEN"
Environment="HUB_URL=$HUB_URL"
ExecStart=$BIN_PATH
User=$AGENT_USER
Restart=on-failure
RestartSec=5
StateDirectory=beszel-agent
SupplementaryGroups=docker
KeyringMode=private
LockPersonality=yes
ProtectClock=yes
ProtectHome=read-only
ProtectHostname=yes
ProtectKernelLogs=yes
ProtectSystem=strict
RemoveIPC=yes
RestrictSUIDSGID=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable beszel-agent.service >/dev/null 2>&1
systemctl restart beszel-agent.service

case "$AUTO_UPDATE" in
  true|yes|y|Y)
    cat >/etc/systemd/system/beszel-agent-update.service <<EOF
[Unit]
Description=Update beszel-agent from hub
Wants=beszel-agent.service

[Service]
Type=oneshot
Environment="HUB_URL=$HUB_URL"
ExecStart=$BIN_PATH update
EOF
    cat >/etc/systemd/system/beszel-agent-update.timer <<EOF
[Unit]
Description=Run beszel-agent update daily

[Timer]
OnCalendar=daily
Persistent=true
RandomizedDelaySec=4h

[Install]
WantedBy=timers.target
EOF
    systemctl daemon-reload
    systemctl enable --now beszel-agent-update.timer >/dev/null 2>&1
    echo "Daily hub updates enabled."
    ;;
esac

if [ "$(systemctl is-active beszel-agent.service)" != "active" ]; then
  echo "Error: service failed to start" >&2
  systemctl status beszel-agent.service --no-pager || true
  exit 1
fi

echo "Installed. Agent reports latency via hub Settings → Latency."
`
}
