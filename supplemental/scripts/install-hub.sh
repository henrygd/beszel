#!/bin/bash

# Kiểm tra xem script có chạy với quyền root không
if [ "$(id -u)" != "0" ]; then
  if command -v sudo >/dev/null 2>&1; then
    exec sudo "$0" "$@"
  else
    echo "Script này phải được chạy với quyền root. Vui lòng:"
    echo "1. Chạy script này với tư cách root (su root)"
    echo "2. Cài đặt sudo và chạy với sudo"
    exit 1
  fi
fi

# Định nghĩa các giá trị mặc định
version=0.0.1
PORT=8090                              # Cổng mặc định
GITHUB_PROXY_URL="https://ghfast.top/" # URL proxy mặc định

# Hàm để đảm bảo URL proxy kết thúc bằng dấu /
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

# Đảm bảo URL proxy kết thúc bằng dấu /
GITHUB_PROXY_URL=$(ensure_trailing_slash "$GITHUB_PROXY_URL")

# Đọc các tùy chọn dòng lệnh
while getopts ":uhp:c:" opt; do
  case $opt in
  u) UNINSTALL="true" ;;
  h)
    printf "Script cài đặt CMonitor Hub\n\n"
    printf "Cách sử dụng: ./install-hub.sh [tùy chọn]\n\n"
    printf "Tùy chọn: \n"
    printf "  -u  : Gỡ cài đặt CMonitor Hub\n"
    printf "  -p <port> : Chỉ định số cổng (mặc định: 8090)\n"
    printf "  -c <url>  : Sử dụng URL mirror GitHub tùy chỉnh (ví dụ: https://ghfast.top/)\n"
    echo "  -h  : Hiển thị thông báo trợ giúp này"
    exit 0
    ;;
  p) PORT=$OPTARG ;;
  c) GITHUB_PROXY_URL=$(ensure_trailing_slash "$OPTARG") ;;
  \?)
    echo "Tùy chọn không hợp lệ: -$OPTARG"
    exit 1
    ;;
  esac
done

# Quá trình gỡ cài đặt nếu được yêu cầu
if [ "$UNINSTALL" = "true" ]; then
  # Dừng và vô hiệu hóa dịch vụ CMonitor Hub
  echo "Dừng và vô hiệu hóa dịch vụ CMonitor Hub..."
  systemctl stop beszel-hub.service
  systemctl disable beszel-hub.service

  # Xóa tệp dịch vụ systemd
  echo "Xóa tệp dịch vụ systemd..."
  rm /etc/systemd/system/beszel-hub.service

  # Tải lại systemd daemon
  echo "Tải lại systemd daemon..."
  systemctl daemon-reload

  # Xóa binary và dữ liệu của CMonitor Hub
  echo "Xóa binary và dữ liệu của CMonitor Hub..."
  rm -rf /opt/beszel

  # Xóa người dùng dành riêng
  echo "Xóa người dùng dành riêng..."
  userdel cmonitor-hub

  echo "CMonitor Hub đã được gỡ cài đặt thành công!"
  exit 0
fi

# Hàm kiểm tra xem một gói đã được cài đặt chưa
package_installed() {
  command -v "$1" >/dev/null 2>&1
}

# Kiểm tra trình quản lý gói và cài đặt các gói cần thiết nếu chưa có
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
  echo "Cảnh báo: Vui lòng đảm bảo 'tar' và 'curl' đã được cài đặt."
fi

# Tạo người dùng dành riêng cho dịch vụ nếu chưa tồn tại
if ! id -u cmonitor-hub >/dev/null 2>&1; then
  echo "Tạo người dùng dành riêng cho dịch vụ CMonitor Hub..."
  useradd -M -s /bin/false cmonitor-hub
fi

# Tải xuống và cài đặt CMonitor Hub
echo "Tải xuống và cài đặt CMonitor Hub..."
curl -sL "${GITHUB_PROXY_URL}https://github.com/nguyendkn/cmonitor/releases/latest/download/cmonitor_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/armv7l/arm/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O beszel | tee ./beszel >/dev/null && chmod +x beszel
mkdir -p /opt/cmonitor/cmonitor_data
mv ./cmonitor /opt/cmonitor/cmonitor
chown -R cmonitor-hub:cmonitor-hub /opt/cmonitor

# Tạo dịch vụ systemd
printf "Tạo dịch vụ systemd cho CMonitor Hub...\n\n"
tee /etc/systemd/system/cmonitor-hub.service <<EOF
[Unit]
Description=CMonitor Hub Service
After=network.target

[Service]
ExecStart=/opt/cmonitor/cmonitor serve --http "0.0.0.0:$PORT"
WorkingDirectory=/opt/cmonitor
User=cmonitor-hub
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Tải và khởi động dịch vụ
printf "\nTải và khởi động dịch vụ CMonitor Hub...\n"
systemctl daemon-reload
systemctl enable cmonitor-hub.service
systemctl start cmonitor-hub.service

# Chờ dịch vụ khởi động hoặc thất bại
sleep 2

# Kiểm tra xem dịch vụ có đang chạy không
if [ "$(systemctl is-active cmonitor-hub.service)" != "active" ]; then
  echo "Lỗi: Dịch vụ CMonitor Hub không chạy."
  echo "$(systemctl status cmonitor-hub.service)"
  exit 1
fi

echo "CMonitor Hub đã được cài đặt và cấu hình thành công! Hiện đang chạy trên cổng $PORT."