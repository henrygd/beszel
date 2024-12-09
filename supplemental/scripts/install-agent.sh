#!/bin/sh

version=0.0.1
# Define default values
PORT=45876
UNINSTALL=false
CHINA_MAINLAND=false
GITHUB_URL="https://github.com"
GITHUB_API_URL="https://api.github.com"
KEY=""

# Function to detect if the system is Alpine Linux
is_alpine() {
  [ -f /etc/alpine-release ]
}

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
  if is_alpine; then
    echo "Stopping the agent service..."
    rc-service beszel-agent stop
    rc-update del beszel-agent default
  else
    echo "Stopping and disabling the agent service..."
    systemctl stop beszel-agent.service
    systemctl disable beszel-agent.service
  fi

  echo "Removing the Beszel Agent files and user..."
  rm -rf /opt/beszel-agent
  if ! is_alpine; then
    rm /etc/systemd/system/beszel-agent.service
    systemctl daemon-reload
  fi
  killall beszel-agent
  userdel beszel
  echo "Beszel Agent has been uninstalled successfully!"
  exit 0
fi

# Install necessary packages
if is_alpine; then
  if ! package_installed tar || ! package_installed curl || ! package_installed sha256sum; then
    apk add --no-cache tar curl coreutils
  fi
else
  if package_installed apt-get; then
    apt-get update
    apt-get install -y tar curl coreutils
  elif package_installed yum; then
    yum install -y tar curl coreutils
  elif package_installed pacman; then
    pacman -Sy --noconfirm tar curl coreutils
  else
    echo "Warning: Please ensure 'tar', 'curl', and 'sha256sum' are installed."
  fi
fi

# Main installation logic...
# Skipping the download and checksum validation for brevity

# Create the service
if is_alpine; then
  echo "Creating OpenRC service for the agent..."
  cat >/etc/init.d/beszel-agent <<EOF
#!/sbin/openrc-run

description="Beszel Agent Service"

command="/opt/beszel-agent/beszel-agent"
command_user="beszel"
pidfile="/var/run/beszel-agent.pid"

depend() {
  need net
}
EOF
  chmod +x /etc/init.d/beszel-agent
  rc-update add beszel-agent default
  rc-service beszel-agent start
else
  echo "Creating systemd service for the agent..."
  cat >/etc/systemd/system/beszel-agent.service <<EOF
[Unit]
Description=Beszel Agent Service
After=network.target

[Service]
Environment="PORT=$PORT"
Environment="KEY=$KEY"
ExecStart=/opt/beszel-agent/beszel-agent
User=beszel
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable beszel-agent.service
  systemctl start beszel-agent.service
fi

# Output installation success message with the port
echo
echo "Beszel Agent has been installed successfully!"
echo "It is now running on port $PORT."
