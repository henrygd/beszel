#!/bin/bash
version=0.0.1
# Define default values

# Read command line options
while getopts ":uh" opt; do
  case $opt in
    u) UNINSTALL="true";;
    h) printf "Beszel Hub installation script\n\n";
       printf "Usage: ./install-hub.sh [options]\n\n";
       printf "Options: \n"
       printf "  -u  : Uninstall the Beszel Hub\n";
       echo "  -h  : Display this help message";
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
  # Function to check if a package is installed
  package_installed() {
    command -v "$1" >/dev/null 2>&1
  }

  # Check for package manager and install necessary packages if not installed
  if package_installed apt-get; then
    if ! package_installed tar || ! package_installed curl; then
      sudo apt-get update
      sudo apt-get install -y tar curl
    fi
  elif package_installed yum; then
    if ! package_installed tar || ! package_installed curl; then
      sudo yum install -y tar curl
    fi
  elif package_installed pacman; then
    if ! package_installed tar || ! package_installed curl; then
      sudo pacman -Sy --noconfirm tar curl
    fi
  else
    echo "Warning: Please ensure 'tar' and 'curl' are installed."
  fi

  # Create a dedicated user for the service if it doesn't exist
  if ! id -u beszel > /dev/null 2>&1; then
    echo "Creating a dedicated user for the Beszel Hub service..."
    sudo useradd -M -s /bin/false beszel
  fi

  # Download and install the Beszel Hub
  echo "Downloading and installing the Beszel Hub..."
  curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel | tee ./beszel >/dev/null && chmod +x beszel
  sudo mkdir -p /opt/beszel/beszel_data
  sudo mv ./beszel /opt/beszel/beszel
  sudo chown -R beszel:beszel /opt/beszel

  # Create the systemd service
  printf "Creating the systemd service for the Beszel Hub...\n\n"
  sudo tee /etc/systemd/system/beszel-hub.service <<EOF
[Unit]
Description=Beszel Hub Service
After=network.target

[Service]
ExecStart=/opt/beszel/beszel serve
WorkingDirectory=/opt/beszel
User=beszel
Restart=always

[Install]
WantedBy=multi-user.target
EOF

  # Load and start the service
  printf "\nLoading and starting the Beszel Hub service...\n"
  sudo systemctl daemon-reload
  sudo systemctl enable beszel-hub.service
  sudo systemctl start beszel-hub.service

  # Wait for the service to start or fail
  sleep 2

  # Check if the service is running
  if [ "$(systemctl is-active beszel-hub.service)" != "active" ]; then
    echo "Error: The Beszel Hub service is not running."
    echo "$(systemctl status beszel-hub.service)"
    exit 1
  fi

  echo "The Beszel Hub has been installed and configured successfully! It is now accessible on port 8090."
fi