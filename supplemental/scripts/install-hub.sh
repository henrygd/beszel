#!/bin/bash

# Check if running as root
if [ "$(id -u)" != "0" ]; then
  if command -v sudo >/dev/null 2>&1; then
    exec sudo "$0" "$@"
  else
    echo "This script must be run as root. Please either:"
    echo "1. Run this script as root (su root)"
    echo "2. Install sudo and run with sudo"
    exit 1
  fi
fi

# Define default values
version=0.0.1
PORT=8090                              # Default port
GITHUB_PROXY_URL="https://ghfast.top/" # Default proxy URL

# Function to ensure the proxy URL ends with a /
ensure_trailing_slash() {
  if [ -n "$1" ]; then
    case "$1" in
    */) echo "$1" ;;
    *) echo "$1/" ;;
    esac
  else
    echo "$1"
  fi
}

# Ensure the proxy URL ends with a /
GITHUB_PROXY_URL=$(ensure_trailing_slash "$GITHUB_PROXY_URL")

# Read command line options
while getopts ":uhp:c:" opt; do
  case $opt in
  u) UNINSTALL="true" ;;
  h)
    printf "Beszel Hub installation script\n\n"
    printf "Usage: ./install-hub.sh [options]\n\n"
    printf "Options: \n"
    printf "  -u  : Uninstall the Beszel Hub\n"
    printf "  -p <port> : Specify a port number (default: 8090)\n"
    printf "  -c <url>  : Use a custom GitHub mirror URL (e.g., https://ghfast.top/)\n"
    echo "  -h  : Display this help message"
    exit 0
    ;;
  p) PORT=$OPTARG ;;
  c) GITHUB_PROXY_URL=$(ensure_trailing_slash "$OPTARG") ;;
  \?)
    echo "Invalid option: -$OPTARG"
    exit 1
    ;;
  esac
done

if [ "$UNINSTALL" = "true" ]; then
  # Stop and disable the Beszel Hub service
  echo "Stopping and disabling the Beszel Hub service..."
  systemctl stop beszel-hub.service
  systemctl disable beszel-hub.service

  # Remove the systemd service file
  echo "Removing the systemd service file..."
  rm /etc/systemd/system/beszel-hub.service

  # Reload the systemd daemon
  echo "Reloading the systemd daemon..."
  systemctl daemon-reload

  # Remove the Beszel Hub binary and data
  echo "Removing the Beszel Hub binary and data..."
  rm -rf /opt/beszel

  # Remove the dedicated user
  echo "Removing the dedicated user..."
  userdel beszel

  echo "The Beszel Hub has been uninstalled successfully!"
  exit 0
fi

# Function to check if a package is installed
package_installed() {
  command -v "$1" >/dev/null 2>&1
}

# Check for package manager and install necessary packages if not installed
if package_installed apt-get; then
  if ! package_installed tar || ! package_installed curl; then
    apt-get update
    apt-get install -y tar curl
  fi
elif package_installed yum; then
  if ! package_installed tar || ! package_installed curl; then
    yum install -y tar curl
  fi
elif package_installed pacman; then
  if ! package_installed tar || ! package_installed curl; then
    pacman -Sy --noconfirm tar curl
  fi
else
  echo "Warning: Please ensure 'tar' and 'curl' are installed."
fi

# Create a dedicated user for the service if it doesn't exist
if ! id -u beszel >/dev/null 2>&1; then
  echo "Creating a dedicated user for the Beszel Hub service..."
  useradd -M -s /bin/false beszel
fi

# Download and install the Beszel Hub
echo "Downloading and installing the Beszel Hub..."
curl -sL "${GITHUB_PROXY_URL}https://github.com/henrygd/beszel/releases/latest/download/beszel_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel | tee ./beszel >/dev/null && chmod +x beszel
mkdir -p /opt/beszel/beszel_data
mv ./beszel /opt/beszel/beszel
chown -R beszel:beszel /opt/beszel

# Create the systemd service
printf "Creating the systemd service for the Beszel Hub...\n\n"
tee /etc/systemd/system/beszel-hub.service <<EOF
[Unit]
Description=Beszel Hub Service
After=network.target

[Service]
ExecStart=/opt/beszel/beszel serve --http "0.0.0.0:$PORT"
WorkingDirectory=/opt/beszel
User=beszel
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Load and start the service
printf "\nLoading and starting the Beszel Hub service...\n"
systemctl daemon-reload
systemctl enable beszel-hub.service
systemctl start beszel-hub.service

# Wait for the service to start or fail
sleep 2

# Check if the service is running
if [ "$(systemctl is-active beszel-hub.service)" != "active" ]; then
  echo "Error: The Beszel Hub service is not running."
  echo "$(systemctl status beszel-hub.service)"
  exit 1
fi

echo "The Beszel Hub has been installed and configured successfully! It is now accessible on port $PORT."
