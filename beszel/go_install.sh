#!/bin/bash

# 定义颜色
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# 定义常量
readonly GO_VERSION=${1:-"1.23.4"}  # 默认版本为 1.23.4
readonly GO_ROOT="/usr/local/go"
readonly GO_WORKSPACE="/usr/local/go_workspace"
readonly GO_URL="https://dl.google.com/go"
readonly TEMP_DIR="/tmp/go_install_$$"  # 使用 PID 创建唯一临时目录
readonly LOG_FILE="/tmp/go_install_$$.log"

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $1" >> "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [WARN] $1" >> "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $1" >> "$LOG_FILE"
}

# 清理函数
cleanup() {
    log_info "清理临时文件..."
    rm -rf "$TEMP_DIR"
    if [ $? -ne 0 ]; then
        log_warn "清理临时文件失败，请手动删除: $TEMP_DIR"
    fi
}

# 错误处理
handle_error() {
    log_error "安装过程中发生错误，请检查日志文件: $LOG_FILE"
    cleanup
    exit 1
}

# 设置错误处理
trap 'handle_error' ERR

# 检查是否为 root 用户
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "请使用 root 权限运行此脚本"
        echo "使用: sudo bash $0 [version]"
        exit 1
    fi
}

# 检查版本格式
check_version() {
    if [ -z "$1" ]; then
        log_info "未指定版本，将安装默认版本: $GO_VERSION"
        return
    fi
    
    if ! echo "$GO_VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
        log_error "无效的版本格式: $GO_VERSION"
        echo "正确的格式例如: 1.23.4, 1.22.0"
        exit 1
    fi
}

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME=$NAME
        OS_VERSION=$VERSION_ID
        log_info "检测到系统: $OS_NAME $OS_VERSION"
    else
        log_error "不支持的系统类型"
        exit 1
    fi
}

# 安装依赖
install_dependencies() {
    log_info "安装必要的依赖..."
    case "$OS_NAME" in
        *"Ubuntu"*|*"Debian"*)
            apt update -qq
            apt install -y wget tar curl
            ;;
        *"CentOS"*|*"Red Hat"*)
            yum install -y wget tar curl
            ;;
        *"Fedora"*)
            dnf install -y wget tar curl
            ;;
        *"openSUSE"*)
            zypper install -y wget tar curl
            ;;
        *"Arch"*)
            pacman -Sy --noconfirm wget tar curl
            ;;
        *)
            log_error "不支持的系统类型: $OS_NAME"
            exit 1
            ;;
    esac
}

# 下载 Go
download_go() {
    log_info "准备下载 Go $GO_VERSION..."
    
    # 检查版本是否存在
    local check_url="$GO_URL/go$GO_VERSION.linux-amd64.tar.gz"
    if ! curl --output /dev/null --silent --head --fail "$check_url"; then
        log_error "版本 $GO_VERSION 不存在或无法访问"
        echo "请检查版本号是否正确，或访问 https://golang.org/dl/ 查看可用版本"
        exit 1
    fi

    mkdir -p "$TEMP_DIR"
    cd "$TEMP_DIR" || exit 1
    
    local archive="go$GO_VERSION.linux-amd64.tar.gz"
    log_info "正在下载 $archive ..."
    if ! wget -q "$GO_URL/$archive"; then
        log_error "下载失败，请检查网络连接或版本号是否正确"
        exit 1
    fi
    
    # 验证下载文件
    if [ ! -f "$archive" ]; then
        log_error "下载文件不存在"
        exit 1
    fi
    
    log_info "下载完成"
}

# 在主要函数之前添加检测函数
check_existing_go() {
    if command -v go &> /dev/null; then
        local current_version
        current_version=$(go version | awk '{print $3}' | sed 's/go//')
        echo -e "${YELLOW}检测到系统已安装 Go ${current_version}${NC}"
        echo -n "是否继续安装 Go ${GO_VERSION}？[y/N] "
        read -r answer
        if [[ ! "$answer" =~ ^[Yy]$ ]]; then
            echo -e "${GREEN}安装已取消${NC}"
            exit 0
        fi
        
        echo -e "${YELLOW}将备份现有安装到: ${GO_ROOT}_backup_$(date '+%Y%m%d_%H%M%S')${NC}"
        echo -n "是否继续？[y/N] "
        read -r answer
        if [[ ! "$answer" =~ ^[Yy]$ ]]; then
            echo -e "${GREEN}安装已取消${NC}"
            exit 0
        fi
    fi
}

# 修改安装函数
install_go() {
    log_info "安装 Go..."
    
    # 备份已存在的 Go 安装
    if [ -d "$GO_ROOT" ]; then
        local backup_dir="${GO_ROOT}_backup_$(date '+%Y%m%d_%H%M%S')"
        log_info "备份现有安装到: $backup_dir"
        if ! mv "$GO_ROOT" "$backup_dir"; then
            log_error "备份失败，无法继续安装"
            exit 1
        fi
        log_info "备份完成"
    fi

    # 解压安装
    log_info "正在解压安装..."
    tar -C /usr/local -xzf "go$GO_VERSION.linux-amd64.tar.gz"
    if [ ! -d "$GO_ROOT" ]; then
        log_error "安装失败，目录不存在: $GO_ROOT"
        exit 1
    fi

    # 创建工作空间
    mkdir -p "$GO_WORKSPACE"/{src,pkg,bin}
    chmod -R 777 "$GO_WORKSPACE"
}

# 配置环境变量
setup_environment() {
    log_info "配置环境变量..."
    
    # 创建环境变量配置文件
    cat > /etc/profile.d/go.sh << EOF
export GOROOT=$GO_ROOT
export GOPATH=$GO_WORKSPACE
export PATH=\$PATH:\$GOROOT/bin:\$GOPATH/bin
EOF

    chmod 644 /etc/profile.d/go.sh
    
    # 立即生效
    source /etc/profile.d/go.sh
}

# 验证安装
verify_installation() {
    log_info "验证安装..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go 命令不可用，安装似乎失败了"
        exit 1
    fi
    
    local installed_version
    installed_version=$(go version | awk '{print $3}' | sed 's/go//')
    if [ "$installed_version" != "$GO_VERSION" ]; then
        log_error "版本不匹配: 预期 $GO_VERSION, 实际 $installed_version"
        exit 1
    fi
    
    log_info "Go $GO_VERSION 安装成功!"
}

# 显示完成信息
show_completion() {
    echo
    echo -e "${GREEN}===============================================${NC}"
    echo -e "${BLUE}Go $GO_VERSION 安装完成！${NC}"
    echo -e "${GREEN}-----------------------------------------------${NC}"
    echo -e "请执行以下步骤完成配置："
    echo
    echo -e "1. 重新加载环境变量:"
    echo -e "   ${YELLOW}source /etc/profile${NC}"
    echo
    echo -e "2. 配置 Go 模块和代理 (推荐):"
    echo -e "   ${YELLOW}go env -w GO111MODULE=on${NC}"
    echo -e "   ${YELLOW}go env -w GOPROXY=https://goproxy.cn,direct${NC}"
    echo
    echo -e "3. 验证安装:"
    echo -e "   ${YELLOW}go version${NC}"
    echo -e "${GREEN}===============================================${NC}"
    
    log_info "安装日志已保存至: $LOG_FILE"
}

# 主函数
main() {
    log_info "开始安装 Go $GO_VERSION..."
    
    check_root
    check_version
    check_existing_go
    detect_os
    install_dependencies
    download_go
    install_go
    setup_environment
    verify_installation
    cleanup
    show_completion
}

# 执行主函数
main

