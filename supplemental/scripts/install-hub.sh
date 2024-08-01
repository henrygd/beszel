#!/bin/bash
version=0.0.1
# Define default values

# Read command line options
while getopts ":uh" opt; do
  case $opt in
    u) UNINSTALL="true";;
    h) echo "Beszel Hub installation script"; echo ""
       echo "Usage: ./install.sh [options]"; echo ""
       echo "Options: "
       echo "  -u  : Uninstall the Beszel Hub"; echo ""
       echo "  -h  : Display this help message"; echo ""
       exit 0;;
    \?) echo "Invalid option: -$OPTARG"; exit 1;;
  esac
done

if [ "$UNINSTALL" = "true" ]; then
  # Stop and disable the Beszel Hub service
  echo "Stopping and disabling the Beszel Hub service..."
  sudo systemctl stop beszel-hub.service
  sudo systemctl disable beszel-hub.service

  # Remove the systemd service file
  echo "Removing the systemd service file..."
  sudo rm /etc/systemd/system/beszel-hub.service

  # Reload the systemd daemon
  echo "Reloading the systemd daemon..."
  sudo systemctl daemon-reload

  # Remove the Beszel Hub binary
  echo "Removing the Beszel Hub binary..."
  sudo rm /opt/beszel/beszel

  # Remove the Beszel Hub directory
  echo "Removing the Beszel Hub directory..."
  sudo rm -rf /opt/beszel

  # Remove the dedicated user
  echo "Removing the dedicated user..."
  sudo userdel beszel

  echo "The Beszel Hub has been uninstalled successfully!"
else
  # Check if the distribution is supported
  if [ "$(cat /etc/os-release | grep '^ID=')" != "ID=debian" ] && [ "$(cat /etc/os-release | grep '^ID=')" != "ID=ubuntu" ] && [ "$(cat /etc/os-release | grep '^ID_LIKE=')" != "ID_LIKE=debian" ]; then
    echo "Error: This script only supports Debian and Ubuntu distributions."
    exit 1
  fi

  # Create a dedicated user for the service
  if ! id -u beszel > /dev/null 2>&1; then
    echo "Creating a dedicated user for the Beszel Hub service..."
    sudo useradd -m -s /bin/false beszel
  fi

  # Download and install the Beszel Hub
  echo "Downloading and installing the Beszel Hub..."
  curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel | tee ./beszel >/dev/null && chmod +x beszel
  sudo mkdir -p /opt/beszel
  sudo mv ./beszel /opt/beszel/beszel
  sudo chown beszel:beszel /opt/beszel/beszel

  # Create the systemd service
  echo "Creating the systemd service for the Beszel Hub..."
  sudo tee /etc/systemd/system/beszel-hub.service <<EOF
[Unit]
Description=Beszel Hub Service
After=network.target

[Service]
ExecStart=/opt/beszel/beszel
User=beszel
Restart=always

[Install]
WantedBy=multi-user.target
EOF

  # Load and start the service
  echo "Loading and starting the Beszel Hub service..."
  sudo systemctl daemon-reload
  sudo systemctl enable beszel-hub.service
  sudo systemctl start beszel-hub.service

  # Check if the service is running
  if [ "$(systemctl is-active beszel-hub.service)" != "active" ]; then
    echo "Error: The Beszel Hub service is not running."
    exit 1
  fi

  echo "The Beszel Hub has been installed and configured successfully! It is now accessible on port 8090."
fi