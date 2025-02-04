#!/bin/sh

is_alpine() {
  [ -f /etc/alpine-release ]
}

is_openwrt() {
  cat /etc/os-release | grep -q "OpenWrt"
}

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

# Define default values
PORT=45876
UNINSTALL=false
GITHUB_URL="https://github.com"
GITHUB_API_URL="https://api.github.com" # not blocked in China currently
GITHUB_PROXY_URL=""
KEY=""

# Check for help flag
case "$1" in
-h | --help)
  printf "Beszel Agent installation script\n\n"
  printf "Usage: ./install-agent.sh [options]\n\n"
  printf "Options: \n"
  printf "  -k                    : SSH key (required, or interactive if not provided)\n"
  printf "  -p                    : Port (default: $PORT)\n"
  printf "  -u                    : Uninstall Beszel Agent\n"
  printf "  --china-mirrors [URL] : Use GitHub proxy (gh.beszel.dev) to resolve network timeout issues in mainland China\n"
  printf "                          optional: specify a custom proxy URL, e.g., \"https://ghfast.top\"\n"
  printf "  -h, --help            : Display this help message\n"
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
    if [ "$2" != "" ] && ! echo "$2" | grep -q '^-'; then
      # use cstom proxy URL if provided
      GITHUB_PROXY_URL="$2"
      GITHUB_URL="$(ensure_trailing_slash "$2")https://github.com"
      shift
    else
      GITHUB_PROXY_URL="https://gh.beszel.dev"
      GITHUB_URL="$GITHUB_PROXY_URL"
    fi
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
    echo "Stopping and disabling the agent service..."
    rc-service beszel-agent stop
    rc-update del beszel-agent default

    echo "Removing the OpenRC service files..."
    rm -f /etc/init.d/beszel-agent

    # Remove the update service if it exists
    echo "Removing the daily update service..."
    rc-service beszel-agent-update stop 2>/dev/null
    rc-update del beszel-agent-update default 2>/dev/null
    rm -f /etc/init.d/beszel-agent-update

    # Remove log files
    echo "Removing log files..."
    rm -f /var/log/beszel-agent.log /var/log/beszel-agent.err
  elif is_openwrt; then
    echo "Stopping and disabling the agent service..."
    service beszel-agent stop
    service beszel-agent disable

    echo "Removing the OpenWRT service files..."
    rm -f /etc/init.d/beszel-agent

    # Remove the update service if it exists
    echo "Removing the daily update service..."
    rm -f /etc/crontabs/beszel

  else
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
  fi

  echo "Removing the Beszel Agent directory..."
  rm -rf /opt/beszel-agent

  echo "Removing the dedicated user for the agent service..."
  killall beszel-agent 2>/dev/null
  if is_alpine || is_openwrt; then
    deluser beszel 2>/dev/null
  else
    userdel beszel 2>/dev/null
  fi

  echo "Beszel Agent has been uninstalled successfully!"
  exit 0
fi

# Confirm the use of GitHub mirrors for downloads
if [ -n "$GITHUB_PROXY_URL" ]; then
  printf "\nConfirm use of GitHub mirror (%s) for downloading beszel-agent?\nThis helps to install properly in mainland China. (Y/n): " "$GITHUB_PROXY_URL"
  read USE_MIRROR
  USE_MIRROR=${USE_MIRROR:-Y}
  if [ "$USE_MIRROR" = "Y" ] || [ "$USE_MIRROR" = "y" ]; then
    echo "Using GitHub Mirror ($GITHUB_PROXY_URL) for downloads..."
  else
    GITHUB_URL="https://github.com"
  fi
fi

# Function to check if a package is installed
package_installed() {
  command -v "$1" >/dev/null 2>&1
}

# Check for package manager and install necessary packages if not installed
if is_alpine; then
  if ! package_installed tar || ! package_installed curl || ! package_installed coreutils; then
    apk update
    apk add tar curl coreutils shadow
  fi
elif is_openwrt; then
  if ! package_installed tar || ! package_installed curl || ! package_installed coreutils; then
    opkg update
    opkg install tar curl coreutils
  fi
elif package_installed apt-get; then
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
if is_alpine; then
  if ! id -u beszel >/dev/null 2>&1; then
    echo "Creating a dedicated user for the Beszel Agent service..."
    adduser -D -H -s /sbin/nologin beszel
  fi
  # Add the user to the docker group to allow access to the Docker socket
  addgroup beszel docker
else
  if ! id -u beszel >/dev/null 2>&1; then
    echo "Creating a dedicated user for the Beszel Agent service..."
    useradd -M -s /bin/false beszel
  fi
  # Add the user to the docker group to allow access to the Docker socket
  usermod -aG docker beszel
fi

# Create the directory for the Beszel Agent
if [ ! -d "/opt/beszel-agent" ]; then
  echo "Creating the directory for the Beszel Agent..."
  mkdir -p /opt/beszel-agent
  chown beszel:beszel /opt/beszel-agent
  chmod 755 /opt/beszel-agent
fi

# Download and install the Beszel Agent
echo "Downloading and installing the agent..."

OS=$(uname -s | sed -e 'y/ABCDEFGHIJKLMNOPQRSTUVWXYZ/abcdefghijklmnopqrstuvwxyz/')
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/armv6l/arm/' -e 's/armv7l/arm/' -e 's/aarch64/arm64/')
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

# Modify service installation part, add Alpine check before systemd service creation
if is_alpine; then
  echo "Creating OpenRC service for Alpine Linux..."
  cat >/etc/init.d/beszel-agent <<EOF
#!/sbin/openrc-run

name="beszel-agent"
description="Beszel Agent Service"
command="/opt/beszel-agent/beszel-agent"
command_user="beszel"
command_background="yes"
pidfile="/run/\${RC_SVCNAME}.pid"
output_log="/var/log/beszel-agent.log"
error_log="/var/log/beszel-agent.err"

start_pre() {
    checkpath -f -m 0644 -o beszel:beszel "\$output_log" "\$error_log"
}

export PORT="$PORT"
export KEY="$KEY"

depend() {
    need net
    after firewall
}
EOF

  chmod +x /etc/init.d/beszel-agent
  rc-update add beszel-agent default

  # Create log files with proper permissions
  touch /var/log/beszel-agent.log /var/log/beszel-agent.err
  chown beszel:beszel /var/log/beszel-agent.log /var/log/beszel-agent.err

  # Start the service
  rc-service beszel-agent restart

  # Check if service started successfully
  sleep 2
  if ! rc-service beszel-agent status | grep -q "started"; then
    echo "Error: The Beszel Agent service failed to start. Checking logs..."
    tail -n 20 /var/log/beszel-agent.err
    exit 1
  fi

  # Auto-update service for Alpine
  printf "\nWould you like to enable automatic daily updates for beszel-agent? (y/n): "
  read AUTO_UPDATE
  case "$AUTO_UPDATE" in
  [Yy]*)
    echo "Setting up daily automatic updates for beszel-agent..."

    cat >/etc/init.d/beszel-agent-update <<EOF
#!/sbin/openrc-run

name="beszel-agent-update"
description="Update beszel-agent if needed"

depend() {
    need beszel-agent
}

start() {
    ebegin "Checking for beszel-agent updates"
    if /opt/beszel-agent/beszel-agent update | grep -q "Successfully updated"; then
        rc-service beszel-agent restart
    fi
    eend $?
}
EOF

    chmod +x /etc/init.d/beszel-agent-update
    rc-update add beszel-agent-update default
    rc-service beszel-agent-update start

    printf "\nAutomatic daily updates have been enabled.\n"
    ;;
  esac

  # Check service status
  if ! rc-service beszel-agent status >/dev/null 2>&1; then
    echo "Error: The Beszel Agent service is not running."
    rc-service beszel-agent status
    exit 1
  fi

elif is_openwrt; then
  echo "Creating procd init script service for OpenWRT..."
  cat >/etc/init.d/beszel-agent <<EOF
#!/bin/sh /etc/rc.common

USE_PROCD=1
START=99

start_service() {
    procd_open_instance
    procd_set_param command /opt/beszel-agent/beszel-agent
    procd_set_param user beszel
    procd_set_param pidfile /var/run/beszel-agent.pid
    procd_set_param env PORT="$PORT"
    procd_set_param env KEY="$KEY"
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}

stop_service() {
    killall beszel-agent
}

# Extra command to trigger agent update
EXTRA_COMMANDS="update"
EXTRA_HELP="        update          Update the Beszel agent"

update() {
    if /opt/beszel-agent/beszel-agent update | grep -q "Successfully updated"; then
        start_service
    fi
}

EOF

  # Enable the service
  chmod +x /etc/init.d/beszel-agent
  service beszel-agent enable

  # Start the service
  service beszel-agent restart

  # Auto-update service for OpenWRT using a crontab job
  printf "\nWould you like to enable automatic daily updates for beszel-agent? (y/n): "
  read AUTO_UPDATE
  case "$AUTO_UPDATE" in
  [Yy]*)
    echo "Setting up daily automatic updates for beszel-agent..."

    cat >/etc/crontabs/beszel <<EOF
0 0 * * * /etc/init.d/beszel-agent update
EOF

    /etc/init.d/cron restart

    printf "\nAutomatic daily updates have been enabled.\n"
    ;;
  esac

  # Check service status
  if ! service beszel-agent running >/dev/null 2>&1; then
    echo "Error: The Beszel Agent service is not running."
    service beszel-agent status
    exit 1
  fi

else
  # Original systemd service installation code
  echo "Creating the systemd service for the agent..."
  cat >/etc/systemd/system/beszel-agent.service <<EOF
[Unit]
Description=Beszel Agent Service
Wants=network-online.target
After=network-online.target

[Service]
Environment="PORT=$PORT"
Environment="KEY=$KEY"
# Environment="EXTRA_FILESYSTEMS=sdb"
ExecStart=/opt/beszel-agent/beszel-agent
User=beszel
Restart=on-failure
RestartSec=5
StateDirectory=beszel-agent

# Security/sandboxing settings
KeyringMode=private
LockPersonality=yes
NoNewPrivileges=yes
PrivateTmp=yes
ProtectClock=yes
ProtectHome=read-only
ProtectHostname=yes
ProtectKernelLogs=yes
ProtectKernelTunables=yes
ProtectSystem=strict
RemoveIPC=yes
RestrictSUIDSGID=true
SystemCallArchitectures=native

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
ExecStart=/bin/sh -c '/opt/beszel-agent/beszel-agent update | grep -q "Successfully updated" && (echo "Update found, restarting beszel-agent" && systemctl restart beszel-agent) || echo "No updates found"'
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
fi

printf "\n\033[32mBeszel Agent has been installed successfully! It is now running on port $PORT.\033[0m\n"
