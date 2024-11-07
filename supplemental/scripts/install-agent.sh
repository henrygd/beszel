#!/bin/sh

version=0.0.1
# Define default values
PORT=45876
UNINSTALL=false

# Read command line options
while getopts "k:p:uh" opt; do
  case $opt in
    k) KEY="$OPTARG";;
    p) PORT="$OPTARG";;
    u) UNINSTALL=true;;
    h) printf "Beszel Agent installation script\n\n"
       printf "Usage: ./install-agent.sh [options]\n\n"
       printf "Options: \n"
       printf "  -k  : SSH key (required, or interactive if not provided)\n"
       printf "  -p  : Port (default: $PORT)\n"
       printf "  -u  : Uninstall Beszel Agent\n"
       printf "  -h  : Display this help message\n"
       exit 0;;
    ?) echo "Invalid option: -$OPTARG"; exit 1;;
  esac
done

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

# If no SSH key is provided, ask for the SSH key interactively
if [ -z "$KEY" ]; then
  printf "Enter your SSH key: "
  read KEY
fi

# Create a dedicated user for the service if it doesn't exist
if ! id -u beszel > /dev/null 2>&1; then
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
curl -sL "https://github.com/henrygd/beszel/releases/latest/download/beszel-agent_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" \
  | tar -xz -O beszel-agent | tee ./beszel-agent >/dev/null
mv ./beszel-agent /opt/beszel-agent/beszel-agent
chown beszel:beszel /opt/beszel-agent/beszel-agent
chmod 755 /opt/beszel-agent/beszel-agent

# Create the systemd service
echo "Creating the systemd service for the agent..."
cat > /etc/systemd/system/beszel-agent.service << EOF
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
    cat > /etc/systemd/system/beszel-agent-update.service << EOF
[Unit]
Description=Update beszel-agent if needed
Wants=beszel-agent.service

[Service]
Type=oneshot
ExecStart=/bin/sh -c '/opt/beszel-agent/beszel-agent update | grep -q "Successfully updated" && systemctl restart beszel-agent'
EOF

    # Create systemd timer for the daily update
    cat > /etc/systemd/system/beszel-agent-update.timer << EOF
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