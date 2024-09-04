#!/bin/bash
version=0.0.1
# Define default values
PORT=45876

# Read command line options
while getopts ":k:p:uh" opt; do
  case $opt in
    k) KEY="$OPTARG";;
    p) PORT="$OPTARG";;
    u) UNINSTALL="true";;
    h) printf "Beszel Agent installation script\n\n"
       printf "Usage: ./install-agent.sh [options]\n\n"
       printf "Options: \n"
       printf "  -k  : SSH key (required, or interactive if not provided)\n"
       printf "  -p  : Port (default: $PORT)\n"
       printf "  -u  : Uninstall the Beszel Agent\n"
       printf "  -h  : Display this help message\n"
       exit 0;;
    \?) echo "Invalid option: -$OPTARG"; exit 1;;
  esac
done

if [ "$UNINSTALL" = "true" ]; then
  # Stop and disable the Beszel Agent service
  echo "Stopping and disabling the Beszel Agent service..."
  sudo systemctl stop beszel-agent.service
  sudo systemctl disable beszel-agent.service

  # Remove the systemd service file
  echo "Removing the systemd service file..."
  sudo rm /etc/systemd/system/beszel-agent.service

  # Reload the systemd daemon
  echo "Reloading the systemd daemon..."
  sudo systemctl daemon-reload

  # Remove the Beszel Agent directory
  echo "Removing the Beszel Agent directory..."
  sudo rm -rf /opt/beszel-agent

  # Remove the dedicated user for the Beszel Agent service
  echo "Removing the dedicated user for the Beszel Agent service..."
  sudo userdel beszel

  echo "The Beszel Agent has been uninstalled successfully!"
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

  # If no SSH key is provided, ask for the SSH key interactively
  if [ -z "$KEY" ]; then
    read -p "Enter your SSH key: " KEY
  fi

  # Create a dedicated user for the service if it doesn't exist
  if ! id -u beszel > /dev/null 2>&1; then
    echo "Creating a dedicated user for the Beszel Agent service..."
    sudo useradd -M -s /bin/false beszel
  fi
  # Add the user to the docker group to allow access to the Docker socket
  sudo usermod -aG docker beszel

  # Create the directory for the Beszel Agent
  if [ ! -d "/opt/beszel-agent" ]; then
    echo "Creating the directory for the Beszel Agent..."
    sudo mkdir -p /opt/beszel-agent
    sudo chown beszel:beszel /opt/beszel-agent
    sudo chmod 755 /opt/beszel-agent
  fi

  # Download and install the Beszel Agent
  echo "Downloading and installing the Beszel Agent..."
  curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel-agent_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel-agent | tee ./beszel-agent >/dev/null
  sudo mv ./beszel-agent /opt/beszel-agent/beszel-agent
  sudo chown beszel:beszel /opt/beszel-agent/beszel-agent
  sudo chmod 755 /opt/beszel-agent/beszel-agent

  # Create the systemd service
  echo "Creating the systemd service for the Beszel Agent..."
  sudo tee /etc/systemd/system/beszel-agent.service <<EOF
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

[Install]
WantedBy=multi-user.target
EOF

  # Load and start the service
  printf "\nLoading and starting the Beszel Agent service...\n"
  sudo systemctl daemon-reload
  sudo systemctl enable beszel-agent.service
  sudo systemctl start beszel-agent.service

  # Wait for the service to start or fail
  sleep 1

  # Check if the service is running
  if [ "$(systemctl is-active beszel-agent.service)" != "active" ]; then
    echo "Error: The Beszel Agent service is not running."
    echo "$(systemctl status beszel-agent.service)"
    exit 1
  fi

  echo "The Beszel Agent has been installed and configured successfully! It is now running on port $PORT."
fi
