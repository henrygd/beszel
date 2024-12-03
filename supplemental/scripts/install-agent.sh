#!/bin/sh

version=0.0.1
# Define default values
PORT=45876
UNINSTALL=false
CHINA_MAINLAND=false
GITHUB_URL="https://github.com"
GITHUB_API_URL="https://api.github.com"
KEY=""

# Check for help flag first
case "$1" in
-h | --help)
  printf "Beszel Agent installation script\n\n"
  printf "Usage: ./install-agent.sh [options]\n\n"
  printf "Options: \n"
  printf "  -k                : SSH key (required, or interactive if not provided)\n"
  printf "  -p                : Port (default: $PORT)\n"
  printf "  -u                : Uninstall Beszel Agent\n"
  printf "  --china-mirrors   : Using GitHub mirror sources to resolve network timeout issues in mainland China\n"
  printf "  -h, --help        : Display this help message\n"
  exit 0
  ;;
esac

# Build sudo args by properly quoting everything
build_sudo_args() {
  QUOTED_ARGS=""
  while [ $# -gt 0 ]; do
    if [ -n "$QUOTED_ARGS" ]; then
      QUOTED_ARGS="$QUOTED_ARGS "
    fi
    QUOTED_ARGS="$QUOTED_ARGS'$(echo "$1" | sed "s/'/'\\\\''/g")'"
    shift
  done
  echo "$QUOTED_ARGS"
}

# Check if running as root and re-execute with sudo if needed
if [ "$(id -u)" != "0" ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO_ARGS=$(build_sudo_args "$@")
    eval "exec sudo $0 $SUDO_ARGS"
  else
    echo "This script must be run as root. Please either:"
    echo "1. Run this script as root (su root)"
    echo "2. Install sudo and run with sudo"
    exit 1
  fi
fi

# Parse arguments
while [ $# -gt 0 ]; do
  case "$1" in
  -k)
    shift
    KEY="$1"
    ;;
  -p)
    shift
    PORT="$1"
    ;;
  -u)
    UNINSTALL=true
    ;;
  --china-mirrors)
    CHINA_MAINLAND=true
    ;;
  *)
    echo "Invalid option: $1" >&2
    exit 1
    ;;
  esac
  shift
done

# Uninstall process
if [ "$UNINSTALL" = true ]; then
  echo "Stopping and disabling the agent service..."
  systemctl stop beszel-agent.service
  systemctl disable beszel-agent.service

  echo "Removing the systemd service file..."
  rm /etc/systemd/system/beszel-agent.service

  # Remove the update timer and service if they exist
  echo "Removing the daily update service and timer..."
  systemctl stop beszel-agent-update.timer 2>/dev/null
  systemctl disable beszel-agent-update.timer 2>/dev/null
  rm -f /etc/systemd/system/beszel-agent-update.service
  rm -f /etc/systemd/system/beszel-agent-update.timer

  systemctl daemon-reload

  echo "Removing the Beszel Agent directory..."
  rm -rf /opt/beszel-agent

  echo "Removing the dedicated user for the agent service..."
  killall beszel-agent
  userdel beszel

  echo "Beszel Agent has been uninstalled successfully!"
  exit 0
fi

if [ "$CHINA_MAINLAND" = true ]; then
  printf "\nConfirmed to use GitHub mirrors (ghp.ci) for download beszel-agent?\nThis helps to install Agent properly in mainland China. (Y/n): "
  read USE_MIRROR
  USE_MIRROR=${USE_MIRROR:-Y}
  if [ "$USE_MIRROR" = "Y" ] || [ "$USE_MIRROR" = "y" ]; then
    GITHUB_URL="https://ghp.ci/https://github.com"
    # In China, only github.com is blocked, while api.github.com is not (for now).
    # GITHUB_API_URL="https://api.github.com"
    echo "Using GitHub Mirror for downloads..."
  else
    echo "GitHub mirrors will not be used for installation."
  fi
fi

# Function to check if a package is installed
package_installed() {
  command -v "$1" >/dev/null 2>&1
}

# Check for package manager and install necessary packages if not installed
if package_installed apt-get; then
  if ! package_installed tar || ! package_installed curl || ! package_installed sha256sum; then
    apt-get update
    apt-get install -y tar curl coreutils
  fi
elif package_installed yum; then
  if ! package_installed tar || ! package_installed curl || ! package_installed sha256sum; then
    yum install -y tar curl coreutils
  fi
elif package_installed pacman; then
  if ! package_installed tar || ! package_installed curl || ! package_installed sha256sum; then
    pacman -Sy --noconfirm tar curl coreutils
  fi
else
  echo "Warning: Please ensure 'tar' and 'curl' and 'sha256sum (coreutils)' are installed."
fi

# If no SSH key is provided, ask for the SSH key interactively
if [ -z "$KEY" ]; then
  printf "Enter your SSH key: "
  read KEY
fi

# Verify checksum
if command -v sha256sum >/dev/null; then
  CHECK_CMD="sha256sum"
elif command -v md5 >/dev/null; then
  CHECK_CMD="md5 -q"
else
  echo "No MD5 checksum utility found"
  exit 1
fi

# Create a dedicated user for the service if it doesn't exist
if ! id -u beszel >/dev/null 2>&1; then
  echo "Creating a dedicated user for the Beszel Agent service..."
  useradd -M -s /bin/false beszel
fi
# Add the user to the docker group to allow access to the Docker socket
usermod -aG docker beszel

# Create the directory for the Beszel Agent
if [ ! -d "/opt/beszel-agent" ]; then
  echo "Creating the directory for the Beszel Agent..."
  mkdir -p /opt/beszel-agent
  chown beszel:beszel /opt/beszel-agent
  chmod 755 /opt/beszel-agent
fi

# Download and install the Beszel Agent
echo "Downloading and installing the agent..."

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/')
FILE_NAME="beszel-agent_${OS}_${ARCH}.tar.gz"
LATEST_VERSION=$(curl -s "$GITHUB_API_URL""/repos/henrygd/beszel/releases/latest" | grep -o '"tag_name": "v[^"]*"' | cut -d'"' -f4 | tr -d 'v')
if [ -z "$LATEST_VERSION" ]; then
  echo "Failed to get latest version"
  exit 1
fi

echo "Downloading and installing agent version ${LATEST_VERSION} from ${GITHUB_URL} ..."

# Download checksums file
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR" || exit 1
CHECKSUM=$(curl -sL "$GITHUB_URL/henrygd/beszel/releases/download/v${LATEST_VERSION}/beszel_${LATEST_VERSION}_checksums.txt" | grep "$FILE_NAME" | cut -d' ' -f1)
if [ -z "$CHECKSUM" ] || ! echo "$CHECKSUM" | grep -qE "^[a-fA-F0-9]{64}$"; then
  echo "Failed to get checksum or invalid checksum format"
  exit 1
fi

if ! curl -#L "$GITHUB_URL/henrygd/beszel/releases/download/v${LATEST_VERSION}/$FILE_NAME" -o "$FILE_NAME"; then
  echo "Failed to download the agent from ""$GITHUB_URL/henrygd/beszel/releases/download/v${LATEST_VERSION}/$FILE_NAME"
  rm -rf "$TEMP_DIR"
  exit 1
fi

if [ "$($CHECK_CMD "$FILE_NAME" | cut -d' ' -f1)" != "$CHECKSUM" ]; then
  echo "Checksum verification failed: $($CHECK_CMD "$FILE_NAME" | cut -d' ' -f1) & $CHECKSUM"
  rm -rf "$TEMP_DIR"
  exit 1
fi

if ! tar -xzf "$FILE_NAME" beszel-agent; then
  echo "Failed to extract the agent"
  rm -rf "$TEMP_DIR"
  exit 1
fi

mv beszel-agent /opt/beszel-agent/beszel-agent
chown beszel:beszel /opt/beszel-agent/beszel-agent
chmod 755 /opt/beszel-agent/beszel-agent

# Cleanup
rm -rf "$TEMP_DIR"

# Create the systemd service
echo "Creating the systemd service for the agent..."
cat >/etc/systemd/system/beszel-agent.service <<EOF
[Unit]
Description=Beszel Agent Service
After=network.target

[Service]
Environment="PORT=$PORT"
Environment="KEY=$KEY"
# Environment="EXTRA_FILESYSTEMS=sdb"
ExecStart=/opt/beszel-agent/beszel-agent
User=beszel
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Load and start the service
printf "\nLoading and starting the agent service...\n"
systemctl daemon-reload
systemctl enable beszel-agent.service
systemctl start beszel-agent.service

# Prompt for auto-update setup
printf "\nWould you like to enable automatic daily updates for beszel-agent? (y/n): "
read AUTO_UPDATE
case "$AUTO_UPDATE" in
[Yy]*)
  echo "Setting up daily automatic updates for beszel-agent..."

  # Create systemd service for the daily update
  cat >/etc/systemd/system/beszel-agent-update.service <<EOF
[Unit]
Description=Update beszel-agent if needed
Wants=beszel-agent.service

[Service]
Type=oneshot
ExecStart=/bin/sh -c '/opt/beszel-agent/beszel-agent update | grep -q "Successfully updated" && systemctl restart beszel-agent'
EOF

  # Create systemd timer for the daily update
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
  systemctl enable --now beszel-agent-update.timer

  printf "\nAutomatic daily updates have been enabled.\n"
  ;;
esac

# Wait for the service to start or fail
if [ "$(systemctl is-active beszel-agent.service)" != "active" ]; then
  echo "Error: The Beszel Agent service is not running."
  echo "$(systemctl status beszel-agent.service)"
  exit 1
fi

printf "\n\033[32mBeszel Agent has been installed successfully! It is now running on port $PORT.\033[0m\n"
