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

# Function to check if running on Alpine Linux
is_alpine() {
  [ -f /etc/alpine-release ]
}

# Build sudo arguments by properly quoting everything
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
    echo "Stopping and removing the OpenRC service..."
    rc-service beszel-agent stop
    rc-update del beszel-agent
    rm -f /etc/init.d/beszel-agent
  else
    echo "Stopping and disabling the systemd service..."
    systemctl stop beszel-agent.service
    systemctl disable beszel-agent.service
    rm -f /etc/systemd/system/beszel-agent.service
    systemctl daemon-reload
  fi

  echo "Removing the Beszel Agent directory..."
  rm -rf /opt/beszel-agent

  echo "Removing the dedicated user for the agent service..."
  killall beszel-agent
  userdel beszel

  echo "Beszel Agent has been uninstalled successfully!"
  exit 0
fi

# Check if in mainland China and use GitHub mirrors if confirmed
if [ "$CHINA_MAINLAND" = true ]; then
  printf "\nConfirmed to use GitHub mirrors (ghp.ci) for download beszel-agent?\nThis helps to install Agent properly in mainland China. (Y/n): "
  read USE_MIRROR
  USE_MIRROR=${USE_MIRROR:-Y}
  if [ "$USE_MIRROR" = "Y" ] || [ "$USE_MIRROR" = "y" ]; then
    GITHUB_URL="https://ghp.ci/https://github.com"
    echo "Using GitHub Mirror for downloads..."
  else
    echo "GitHub mirrors will not be used for installation."
  fi
fi

# Function to setup service based on the environment
setup_service() {
  if is_alpine; then
    # Configure OpenRC service
    echo "Creating the OpenRC service for the Beszel Agent..."
    cat >/etc/init.d/beszel-agent <<EOF
#!/sbin/openrc-run
description="Beszel Agent Service"
command="/opt/beszel-agent/beszel-agent"
command_background=true
pidfile="/var/run/beszel-agent.pid"
output_log="/var/log/beszel-agent.log"
error_log="/var/log/beszel-agent.err"
depend() {
  need net
}
EOF
    chmod +x /etc/init.d/beszel-agent
    rc-update add beszel-agent default
    rc-service beszel-agent start
  else
    # Configure systemd service
    echo "Creating the systemd service for the agent..."
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
}

# Call setup_service after installation
setup_service

printf "\nBeszel Agent has been installed successfully!\n"
