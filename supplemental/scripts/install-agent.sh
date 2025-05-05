#!/bin/sh

# Kiểm tra xem hệ thống có phải là Alpine Linux không
is_alpine() {
  [ -f /etc/alpine-release ]
}

# Kiểm tra xem hệ thống có phải là OpenWrt không
is_openwrt() {
  cat /etc/os-release | grep -q "OpenWrt"
}

# Đảm bảo URL proxy kết thúc bằng dấu /
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

# Khai báo các giá trị mặc định
PORT=45876
UNINSTALL=false
GITHUB_URL="https://github.com"
GITHUB_API_URL="https://api.github.com"
GITHUB_PROXY_URL=""
KEY=""
AUTO_UPDATE_FLAG=""
USER_NAME="cmonitor-agent"

# Kiểm tra cờ trợ giúp (-h hoặc --help)
case "$1" in
-h | --help)
  printf "CMonitor Agent installation script\n\n"
  printf "Usage: ./install-agent.sh [options]\n\n"
  printf "Options: \n"
  printf "  -k                    : SSH key (required, or interactive if not provided)\n"
  printf "  -p                    : Port (default: $PORT)\n"
  printf "  -u                    : Uninstall CMonitor Agent\n"
  printf "  --auto-update [VALUE] : Control automatic daily updates\n"
  printf "                          VALUE can be true (enable) or false (disable). If not specified, will prompt.\n"
  printf "  --china-mirrors [URL] : Use GitHub proxy to resolve network timeout issues in mainland China\n"
  printf "                          URL: optional custom proxy URL (default: https://gh.cmonitor.dev)\n"
  printf "  -h, --help            : Display this help message\n"
  exit 0
  ;;
esac

# Hàm xây dựng các đối số cho sudo với việc trích dẫn đúng
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

# Kiểm tra xem script có chạy với quyền root không, nếu không thì chạy lại với sudo
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

# Phân tích các tham số dòng lệnh
while [ $# -gt 0 ]; do
  case "$1" in
  -k) shift; KEY="$1" ;;
  -p) shift; PORT="$1" ;;
  -u) UNINSTALL=true ;;
  --china-mirrors*)
    if echo "$1" | grep -q "="; then
      CUSTOM_PROXY=$(echo "$1" | cut -d'=' -f2)
      if [ -n "$CUSTOM_PROXY" ]; then
        GITHUB_PROXY_URL="$CUSTOM_PROXY"
        GITHUB_URL="$(ensure_trailing_slash "$CUSTOM_PROXY")https://github.com"
      else
        GITHUB_PROXY_URL="https://gh.nguyendkn.dev"
        GITHUB_URL="$GITHUB_PROXY_URL"
      fi
    elif [ "$2" != "" ] && ! echo "$2" | grep -q '^-'; then
      GITHUB_PROXY_URL="$2"
      GITHUB_URL="$(ensure_trailing_slash "$2")https://github.com"
      shift
    else
      GITHUB_PROXY_URL="https://gh.nguyendkn.dev"
      GITHUB_URL="$GITHUB_PROXY_URL"
    fi
    ;;
  --auto-update*)
    if echo "$1" | grep -q "="; then
      AUTO_UPDATE_VALUE=$(echo "$1" | cut -d'=' -f2)
      if [ "$AUTO_UPDATE_VALUE" = "true" ]; then
        AUTO_UPDATE_FLAG="true"
      elif [ "$AUTO_UPDATE_VALUE" = "false" ]; then
        AUTO_UPDATE_FLAG="false"
      else
        echo "Invalid value for --auto-update flag: $AUTO_UPDATE_VALUE. Using default (prompt)."
      fi
    elif [ "$2" = "true" ] || [ "$2" = "false" ]; then
      AUTO_UPDATE_FLAG="$2"
      shift
    else
      AUTO_UPDATE_FLAG="true"
    fi
    ;;
  *)
    echo "Invalid option: $1" >&2
    exit 1
    ;;
  esac
  shift
done

# Quá trình gỡ cài đặt
if [ "$UNINSTALL" = true ]; then
  # Clean up SELinux contexts before removing files
  cleanup_selinux_context

  if is_alpine; then
    echo "Stopping and disabling the agent service..."
    rc-service cmonitor-agent stop 2>/dev/null || true
    rc-update del cmonitor-agent default 2>/dev/null || true
    echo "Removing the OpenRC service files..."
    rm -f /etc/init.d/cmonitor-agentAssertions
    echo "Removing the daily update service..."
    rc-service cmonitor-agent-update stop 2>/dev/null || true
    rc-update del cmonitor-agent-update default 2>/dev/null || true
    rm -f /etc/init.d/cmonitor-agent-update
    echo "Removing log files..."
    rm -f /var/log/cmonitor-agent.log /var/log/cmonitor-agent.err
  elif is_openwrt; then
    echo "Stopping and disabling the agent service..."
    service cmonitor-agent stop 2>/dev/null || true
    service cmonitor-agent disable 2>/dev/null || true
    echo "Removing the OpenWRT service files..."
    rm -f /etc/init.d/cmonitor-agent
    echo "Removing the daily update service..."
    rm -f /etc/crontabs/cmonitor-agent
  else
    echo "Stopping and disabling the agent service..."
    systemctl stop cmonitor-agent.service 2>/dev/null || true
    systemctl disable cmonitor-agent.service 2>/dev/null || true
    echo "Removing the systemd service file..."
    rm -f /etc/systemd/system/cmonitor-agent.service
    echo "Removing the daily update service and timer..."
    systemctl stop cmonitor-agent-update.timer 2>/dev/null || true
    systemctl disable cmonitor-agent-update.timer 2>/dev/null || true
    rm -f /etc/systemd/system/cmonitor-agent-update.service
    rm -f /etc/systemd/system/cmonitor-agent-update.timer
    systemctl daemon-reload
  fi

  echo "Removing the CMonitor Agent directory..."
  rm -rf /opt/cmonitor-agent

  echo "Removing the dedicated user for the agent service..."
  pkill -u "$USER_NAME" 2>/dev/null || true
  if is_alpine; then
    if id -u "$USER_NAME" >/dev/null 2>&1; then
      deluser "$USER_NAME" 2>/dev/null || true
    fi
  elif is_openwrt; then
    if id -u "$USER_NAME" >/dev/null 2>&1; then
      deluser "$USER_NAME" 2>/dev/null || true
    fi
  else
    if id -u "$USER_NAME" >/dev/null 2>&1; then
      userdel "$USER_NAME" 2>/dev/null || true
    fi
  fi

  echo "CMonitor Agent has been uninstalled successfully!"
  exit 0
fi

# Xác nhận sử dụng proxy GitHub để tải xuống
if [ -n "$GITHUB_PROXY_URL" ]; then
  printf "\nConfirm use of GitHub mirror (%s) for downloading cmonitor-agent?\nThis helps to install properly in mainland China. (Y/n): " "$GITHUB_PROXY_URL"
  read USE_MIRROR
  USE_MIRROR=${USE_MIRROR:-Y}
  if [ "$USE_MIRROR" = "Y" ] || [ "$USE_MIRROR" = "y" ]; then
    echo "Using GitHub Mirror ($GITHUB_PROXY_URL) for downloads..."
  else
    GITHUB_URL="https://github.com"
  fi
fi

# Hàm kiểm tra xem một gói đã được cài đặt chưa
package_installed() {
  command -v "$1" >/dev/null 2>&1
}

# Kiểm tra trình quản lý gói và cài đặt các gói cần thiết nếu chưa có
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

# Nếu không cung cấp khóa SSH, yêu cầu người dùng nhập
if [ -z "$KEY" ]; then
  printf "Enter your SSH key: "
  read KEY
fi

# Xác minh công cụ checksum
if command -v sha256sum >/dev/null; then
  CHECK_CMD="sha256sum"
elif command -v md5 >/dev/null; then
  CHECK_CMD="md5 -q"
else
  echo "No MD5 checksum utility found"
  exit 1
fi

# Tạo người dùng dành riêng cho dịch vụ nếu chưa tồn tại
if is_alpine; then
  if ! id -u "$USER_NAME" >/dev/null 2>&1; then
    echo "Creating a dedicated user for the CMonitor Agent service..."
    adduser -D -H -s /sbin/nologin "$USER_NAME"
  fi
  addgroup "$USER_NAME" docker 2>/dev/null || true
else
  if ! id -u "$USER_NAME" >/dev/null 2>&1; then
    echo "Creating a dedicated user for the CMonitor Agent service..."
    useradd -M -s /bin/false "$USER_NAME"
  fi
  usermod -aG docker "$USER_NAME" 2>/dev/null || true
fi

# Tạo thư mục cho CMonitor Agent
if [ ! -d "/opt/cmonitor-agent" ]; then
  echo "Creating the directory for the CMonitor Agent..."
  mkdir -p /opt/cmonitor-agent
  chown "$USER_NAME:$USER_NAME" /opt/cmonitor-agent
  chmod 755 /opt/cmonitor-agent
fi

# Tải xuống và cài đặt CMonitor Agent
echo "Downloading and installing the agent..."
OS=$(uname -s | sed -e 'y/ABCDEFGHIJKLMNOPQRSTUVWXYZ/abcdefghijklmnopqrstuvwxyz/')
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/armv6l/arm/' -e 's/armv7l/arm/' -e 's/aarch64/arm64/')
FILE_NAME="cmonitor-agent_${OS}_${ARCH}.tar.gz"
LATEST_VERSION=$(curl -s "$GITHUB_API_URL/repos/nguyendkn/cmonitor/releases/latest" | grep -o '"tag_name": "v[^"]*"' | cut -d'"' -f4 | tr -d 'v')
if [ -z "$LATEST_VERSION" ]; then
  echo "Failed to get latest version"
  exit 1
fi
echo "Downloading and installing agent version ${LATEST_VERSION} from ${GITHUB_URL} ..."
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR" || exit 1
CHECKSUM=$(curl -sL "$GITHUB_URL/nguyendkn/cmonitor/releases/download/v${LATEST_VERSION}/beszel_${LATEST_VERSION}_checksums.txt" | grep "$FILE_NAME" | cut -d' ' -f1)
if [ -z "$CHECKSUM" ] || ! echo "$CHECKSUM" | grep -qE "^[a-fA-F0-9]{64}$"; then
  echo "Failed to get checksum or invalid checksum format"
  exit 1
fi
if ! curl -#L "$GITHUB_URL/nguyendkn/cmonitor/releases/download/v${LATEST_VERSION}/$FILE_NAME" -o "$FILE_NAME"; then
  echo "Failed to download the agent from $GITHUB_URL/nguyendkn/cmonitor/releases/download/v${LATEST_VERSION}/$FILE_NAME"
  rm -rf "$TEMP_DIR"
  exit 1
fi
if [ "$($CHECK_CMD "$FILE_NAME" | cut -d' ' -f1)" != "$CHECKSUM" ]; then
  echo "Checksum verification failed: $($CHECK_CMD "$FILE_NAME" | cut -d' ' -f1) & $CHECKSUM"
  rm -rf "$TEMP_DIR"
  exit 1
fi
if ! tar -xzf "$FILE_NAME" cmonitor-agent; then
  echo "Failed to extract the agent"
  rm -rf "$TEMP_DIR"
  exit 1
fi
mv cmonitor-agent /opt/cmonitor-agent/cmonitor-agent
chown "$USER_NAME:$USER_NAME" /opt/cmonitor-agent/cmonitor-agent
chmod 755 /opt/cmonitor-agent/cmonitor-agent
rm -rf "$TEMP_DIR"

# Cài đặt dịch vụ
if is_alpine; then
  echo "Creating OpenRC service for Alpine Linux..."
  cat >/etc/init.d/cmonitor-agent <<EOF
#!/sbin/openrc-run
name="cmonitor-agent"
description="CMonitor Agent Service"
command="/opt/cmonitor-agent/cmonitor-agent"
command_user="$USER_NAME"
command_background="yes"
pidfile="/run/\${RC_SVCNAME}.pid"
output_log="/var/log/cmonitor-agent.log"
error_log="/var/log/cmonitor-agent.err"
start_pre() {
    checkpath -f -m 0644 -o $USER_NAME:$USER_NAME "\$output_log" "\$error_log"
}
export PORT="$PORT"
export KEY="$KEY"
depend() {
    need net
    after firewall
}
EOF
  chmod +x /etc/init.d/cmonitor-agent
  rc-update add cmonitor-agent default
  mkdir -p /var/log
  touch /var/log/cmonitor-agent.log /var/log/cmonitor-agent.err
  chown "$USER_NAME:$USER_NAME" /var/log/cmonitor-agent.log /var/log/cmonitor-agent.err
  rc-service cmonitor-agent restart
  sleep 2
  if ! rc-service cmonitor-agent status | grep -q "started"; then
    echo "Error: The CMonitor Agent service failed to start. Checking logs..."
    tail -n 20 /var/log/cmonitor-agent.err
    exit 1
  fi
  if [ "$AUTO_UPDATE_FLAG" = "true" ]; then
    AUTO_UPDATE="y"
  elif [ "$AUTO_UPDATE_FLAG" = "false" ]; then
    AUTO_UPDATE="n"
  else
    printf "\nWould you like to enable automatic daily updates for cmonitor-agent? (y/n): "
    read AUTO_UPDATE
  fi
  case "$AUTO_UPDATE" in
  [Yy]*)
    echo "Setting up daily automatic updates for cmonitor-agent..."
    cat >/etc/init.d/cmonitor-agent-update <<EOF
#!/sbin/openrc-run
name="cmonitor-agent-update"
description="Update cmonitor-agent if needed"
depend() {
    need cmonitor-agent
}
start() {
    ebegin "Checking for cmonitor-agent updates"
    if /opt/cmonitor-agent/cmonitor-agent update | grep -q "Successfully updated"; then
        rc-service cmonitor-agent restart
    fi
    eend $?
}
EOF
    chmod +x /etc/init.d/cmonitor-agent-update
    rc-update add cmonitor-agent-update default
    rc-service cmonitor-agent-update start
    printf "\nAutomatic daily updates have been enabled.\n"
    ;;
  esac
  if ! rc-service cmonitor-agent status >/dev/null 2>&1; then
    echo "Error: The CMonitor Agent service is not running."
    rc-service cmonitor-agent status
    exit 1
  fi
elif is_openwrt; then
  echo "Creating procd init script service for OpenWRT..."
  cat >/etc/init.d/cmonitor-agent <<EOF
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=99
start_service() {
    procd_open_instance
    procd_set_param command /opt/cmonitor-agent/cmonitor-agent
    procd_set_param user $USER_NAME
    procd_set_param pidfile /var/run/cmonitor-agent.pid
    procd_set_param env PORT="$PORT"
    procd_set_param env KEY="$KEY"
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
stop_service() {
    killall cmonitor-agent
}
EXTRA_COMMANDS="update"
EXTRA_HELP="        update          Update the CMonitor agent"
update() {
    if /opt/cmonitor-agent/cmonitor-agent update | grep -q "Successfully updated"; then
        start_service
    fi
}
EOF
  chmod +x /etc/init.d/cmonitor-agent
  service cmonitor-agent enable
  service cmonitor-agent restart
  if [ "$AUTO_UPDATE_FLAG" = "true" ]; then
    AUTO_UPDATE="y"
    sleep 1
  elif [ "$AUTO_UPDATE_FLAG" = "false" ]; then
    AUTO_UPDATE="n"
    sleep 1
  else
    printf "\nWould you like to enable automatic daily updates for cmonitor-agent? (y/n): "
    read AUTO_UPDATE
  fi
  case "$AUTO_UPDATE" in
  [Yy]*)
    echo "Setting up daily automatic updates for cmonitor-agent..."
    cat >/etc/crontabs/cmonitor-agent <<EOF
0 0 * * * /etc/init.d/cmonitor-agent update
EOF
    /etc/init.d/cron restart
    printf "\nAutomatic daily updates have been enabled.\n"
    ;;
  esac
  if ! service cmonitor-agent running >/dev/null 2>&1; then
    echo "Error: The CMonitor Agent service is not running."
    service cmonitor-agent status
    exit 1
  fi
else
  echo "Creating the systemd service for the agent..."
  cat >/etc/systemd/system/cmonitor-agent.service <<EOF
[Unit]
Description=CMonitor Agent Service
Wants=network-online.target
After=network-online.target
[Service]
Environment="PORT=$PORT"
Environment="KEY=$KEY"
ExecStart=/opt/cmonitor-agent/cmonitor-agent
User=$USER_NAME
Restart=on-failure
RestartSec=5
StateDirectory=cmonitor-agent
KeyringMode=private
LockPersonality=yes
NoNewPrivileges=yes
ProtectClock=yes
ProtectHome=read-only
ProtectHostname=yes
ProtectKernelLogs=yes
ProtectSystem=strict
RemoveIPC=yes
RestrictSUIDSGID=true
SystemCallArchitectures=native
[Install]
WantedBy=multi-user.target
EOF
  printf "\nLoading and starting the agent service...\n"
  systemctl daemon-reload
  systemctl enable cmonitor-agent.service
  systemctl start cmonitor-agent.service
  if [ "$AUTO_UPDATE_FLAG" = "true" ]; then
    AUTO_UPDATE="y"
    sleep 1
  elif [ "$AUTO_UPDATE_FLAG" = "false" ]; then
    AUTO_UPDATE="n"
    sleep 1
  else
    printf "\nWould you like to enable automatic daily updates for cmonitor-agent? (y/n): "
    read AUTO_UPDATE
  fi
  case "$AUTO_UPDATE" in
  [Yy]*)
    echo "Setting up daily automatic updates for cmonitor-agent..."
    cat >/etc/systemd/system/cmonitor-agent-update.service <<EOF
[Unit]
Description=Update cmonitor-agent if needed
Wants=cmonitor-agent.service
[Service]
Type=oneshot
ExecStart=/bin/sh -c '/opt/cmonitor-agent/cmonitor-agent update | grep -q "Successfully updated" && (echo "Update found, restarting cmonitor-agent" && systemctl restart cmonitor-agent) || echo "No updates found"'
EOF
    cat >/etc/systemd/system/cmonitor-agent-update.timer <<EOF
[Unit]
Description=Run cmonitor-agent update daily
[Timer]
OnCalendar=daily
Persistent=true
RandomizedDelaySec=4h
[Install]
WantedBy=timers.target
EOF
    systemctl daemon-reload
    systemctl enable --now cmonitor-agent-update.timer
    printf "\nAutomatic daily updates have been enabled.\n"
    ;;
  esac
  if [ "$(systemctl is-active cmonitor-agent.service)" != "active" ]; then
    echo "Error: The CMonitor Agent service is not running."
    echo "$(systemctl status cmonitor-agent.service)"
    exit 1
  fi
fi

printf "\n\033[32mCMonitor Agent has been installed successfully! It is now running on port $PORT.\033[0m\n"