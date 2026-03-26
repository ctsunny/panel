#!/bin/bash
set -e

export LANG=en_US.UTF-8
export LC_ALL=C

INSTALL_DIR="/opt/panel"
SERVICE_FILE="/etc/systemd/system/panel.service"
ENV_FILE="$INSTALL_DIR/.env"
STATIC_DIR="$INSTALL_DIR/static"
DATA_DIR="$INSTALL_DIR/data"

REPO="ctsunny/panel"

# ─── 检测架构 ────────────────────────────────────────────────────────────────
get_arch() {
  case $(uname -m) in
    x86_64)        echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)             echo "amd64" ;;
  esac
}
ARCH=$(get_arch)

# ─── 检测国内环境，选择加速镜像 ──────────────────────────────────────────────
COUNTRY=$(curl -s --max-time 5 https://ipinfo.io/country 2>/dev/null || true)
proxy_url() {
  local url="$1"
  if [ "$COUNTRY" = "CN" ]; then
    echo "https://ghfast.top/${url}"
  else
    echo "$url"
  fi
}

# ─── 获取最新版本号 ────────────────────────────────────────────────────────────
get_latest_version() {
  curl -s "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"([^"]+)".*/\1/'
}

# ─── 构建下载地址 ──────────────────────────────────────────────────────────────
build_urls() {
  local ver="$1"
  PANEL_URL=$(proxy_url "https://github.com/${REPO}/releases/download/${ver}/panel-${ARCH}")
  FRONTEND_URL=$(proxy_url "https://github.com/${REPO}/releases/download/${ver}/frontend-dist.tar.gz")
  SERVICE_URL=$(proxy_url "https://raw.githubusercontent.com/${REPO}/main/panel.service")
}

# ─── 检查 root / sudo ─────────────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
  SUDO_CMD="sudo"
else
  SUDO_CMD=""
fi

# ─── 生成随机 JWT_SECRET ───────────────────────────────────────────────────────
generate_secret() {
  tr -dc 'A-Za-z0-9!@#$%^&*_+-' </dev/urandom | head -c 32
}

# ─── 生成随机管理员用户名（admin_ + 6位小写字母数字）─────────────────────────
generate_admin_user() {
  echo "admin_$(tr -dc 'a-z0-9' </dev/urandom | head -c 6)"
}

# ─── 生成随机管理员密码（16位大小写字母数字）──────────────────────────────────
generate_admin_pass() {
  tr -dc 'A-Za-z0-9' </dev/urandom | head -c 16
}

# ─── 生成随机面板访问路径（8位小写字母数字）───────────────────────────────────
generate_panel_path() {
  tr -dc 'a-z0-9' </dev/urandom | head -c 8
}

# ─── 显示菜单 ─────────────────────────────────────────────────────────────────
show_menu() {
  echo "==============================================="
  echo "          Panel 裸机安装管理脚本"
  echo "==============================================="
  echo "1. 安装"
  echo "2. 更新"
  echo "3. 卸载"
  echo "4. 退出"
  echo "==============================================="
}

# ─── 安装 ─────────────────────────────────────────────────────────────────────
install_panel() {
  echo "🚀 开始安装 Panel..."

  VERSION=$(get_latest_version)
  if [[ -z "$VERSION" ]]; then
    echo "❌ 无法获取最新版本号，请检查网络连接。"
    exit 1
  fi
  echo "📦 版本: $VERSION"
  build_urls "$VERSION"

  $SUDO_CMD mkdir -p "$INSTALL_DIR" "$STATIC_DIR" "$DATA_DIR/logs"

  # 停止已有服务
  if systemctl list-units --full -all 2>/dev/null | grep -Fq "panel.service"; then
    echo "🛑 停止已有 panel 服务..."
    $SUDO_CMD systemctl stop panel 2>/dev/null || true
    $SUDO_CMD systemctl disable panel 2>/dev/null || true
  fi

  # 下载二进制
  echo "⬇️ 下载 panel-${ARCH}..."
  $SUDO_CMD curl -fL "$PANEL_URL" -o "$INSTALL_DIR/panel"
  $SUDO_CMD chmod +x "$INSTALL_DIR/panel"
  echo "✅ panel 下载完成"

  # 下载并解压前端
  echo "⬇️ 下载前端静态文件..."
  TMP_FRONT=$(mktemp /tmp/frontend-dist.XXXXXX.tar.gz)
  curl -fL "$FRONTEND_URL" -o "$TMP_FRONT"
  $SUDO_CMD tar -xzf "$TMP_FRONT" -C "$STATIC_DIR"
  rm -f "$TMP_FRONT"
  echo "✅ 前端静态文件解压完成"

  # 生成或保留 JWT_SECRET
  if [[ -f "$ENV_FILE" ]]; then
    echo "⏭️ 保留已有环境配置: $ENV_FILE"
  else
    JWT_SECRET=$(generate_secret)
    ADMIN_USER=$(generate_admin_user)
    ADMIN_PASS=$(generate_admin_pass)
    PANEL_PATH=$(generate_panel_path)
    $SUDO_CMD tee "$ENV_FILE" > /dev/null <<EOF
PORT=6365
DB_PATH=${DATA_DIR}/gost.db
STATIC_DIR=${STATIC_DIR}
JWT_SECRET=${JWT_SECRET}
LOG_DIR=${DATA_DIR}/logs
ADMIN_USER=${ADMIN_USER}
ADMIN_PASS=${ADMIN_PASS}
PANEL_PATH=${PANEL_PATH}
EOF
    $SUDO_CMD chmod 600 "$ENV_FILE"
    echo "✅ 环境配置已写入: $ENV_FILE"
  fi

  # 写入 systemd service
  $SUDO_CMD tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=Panel Service
After=network.target

[Service]
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/panel
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

  $SUDO_CMD systemctl daemon-reload
  $SUDO_CMD systemctl enable panel
  $SUDO_CMD systemctl start panel

  if $SUDO_CMD systemctl is-active --quiet panel; then
    # Read credentials from .env for display (handles both new and existing installs)
    DISPLAY_USER=$(grep '^ADMIN_USER=' "$ENV_FILE" 2>/dev/null | cut -d= -f2)
    DISPLAY_PASS=$(grep '^ADMIN_PASS=' "$ENV_FILE" 2>/dev/null | cut -d= -f2)
    DISPLAY_PATH=$(grep '^PANEL_PATH=' "$ENV_FILE" 2>/dev/null | cut -d= -f2)
    DISPLAY_PORT=$(grep '^PORT=' "$ENV_FILE" 2>/dev/null | cut -d= -f2)
    DISPLAY_PORT="${DISPLAY_PORT:-6365}"
    SERVER_IP=$(hostname -I | awk '{print $1}')

    echo ""
    echo "✅ 安装完成！Panel 已启动并设置为开机自启。"
    echo "📁 安装目录: $INSTALL_DIR"
    echo "🗄️ 数据目录: $DATA_DIR"
    if [[ -n "$DISPLAY_PATH" ]]; then
      echo "🌐 访问地址: http://${SERVER_IP}:${DISPLAY_PORT}/${DISPLAY_PATH}/"
    else
      echo "🌐 访问地址: http://${SERVER_IP}:${DISPLAY_PORT}/"
    fi
    if [[ -n "$DISPLAY_USER" ]]; then
      echo "👤 管理员账号: $DISPLAY_USER"
      echo "🔑 管理员密码: $DISPLAY_PASS"
      echo "⚠️  请立即记录以上账号密码，安装完成后无法再次查看！"
    fi
    echo "🔧 查看日志: journalctl -u panel -f"
    echo "🔧 服务状态: systemctl status panel"
  else
    echo "❌ Panel 服务启动失败，请执行以下命令查看日志："
    echo "journalctl -u panel -f"
    exit 1
  fi
}

# ─── 更新 ─────────────────────────────────────────────────────────────────────
update_panel() {
  echo "🔄 开始更新 Panel..."

  if [[ ! -f "$INSTALL_DIR/panel" ]]; then
    echo "❌ Panel 未安装，请先选择安装。"
    return 1
  fi

  VERSION=$(get_latest_version)
  if [[ -z "$VERSION" ]]; then
    echo "❌ 无法获取最新版本号，请检查网络连接。"
    exit 1
  fi
  echo "📦 最新版本: $VERSION"
  build_urls "$VERSION"

  # 下载新二进制到临时文件
  echo "⬇️ 下载 panel-${ARCH}..."
  TMP_BIN=$(mktemp /tmp/panel.XXXXXX)
  curl -fL "$PANEL_URL" -o "$TMP_BIN"
  chmod +x "$TMP_BIN"

  # 下载并解压前端
  echo "⬇️ 下载前端静态文件..."
  TMP_FRONT=$(mktemp /tmp/frontend-dist.XXXXXX.tar.gz)
  curl -fL "$FRONTEND_URL" -o "$TMP_FRONT"

  # 停止服务
  if systemctl list-units --full -all 2>/dev/null | grep -Fq "panel.service"; then
    echo "🛑 停止 panel 服务..."
    $SUDO_CMD systemctl stop panel
  fi

  # 替换文件
  $SUDO_CMD mv "$TMP_BIN" "$INSTALL_DIR/panel"
  $SUDO_CMD chmod +x "$INSTALL_DIR/panel"
  $SUDO_CMD tar -xzf "$TMP_FRONT" -C "$STATIC_DIR"
  rm -f "$TMP_FRONT"
  echo "✅ 文件替换完成"

  # 重启服务
  $SUDO_CMD systemctl start panel
  echo "✅ 更新完成，服务已重启。"
}

# ─── 卸载 ─────────────────────────────────────────────────────────────────────
uninstall_panel() {
  echo "🗑️ 开始卸载 Panel..."

  read -p "确认卸载 Panel 吗？此操作将删除所有相关文件 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  if systemctl list-units --full -all 2>/dev/null | grep -Fq "panel.service"; then
    echo "🛑 停止并禁用服务..."
    $SUDO_CMD systemctl stop panel 2>/dev/null || true
    $SUDO_CMD systemctl disable panel 2>/dev/null || true
  fi

  [[ -f "$SERVICE_FILE" ]] && $SUDO_CMD rm -f "$SERVICE_FILE" && echo "🧹 删除服务文件"
  $SUDO_CMD systemctl daemon-reload

  if [[ -d "$INSTALL_DIR" ]]; then
    $SUDO_CMD rm -rf "$INSTALL_DIR"
    echo "🧹 删除安装目录: $INSTALL_DIR"
  fi

  echo "✅ 卸载完成"
}

# ─── 主逻辑 ───────────────────────────────────────────────────────────────────
main() {
  # 支持命令行参数直接调用
  case "${1:-}" in
    install)   install_panel; exit 0 ;;
    update)    update_panel;  exit 0 ;;
    uninstall) uninstall_panel; exit 0 ;;
  esac

  while true; do
    show_menu
    read -p "请输入选项 (1-4): " choice
    case $choice in
      1) install_panel;   exit 0 ;;
      2) update_panel;    exit 0 ;;
      3) uninstall_panel; exit 0 ;;
      4) echo "👋 退出脚本"; exit 0 ;;
      *) echo "❌ 无效选项，请输入 1-4"; echo "" ;;
    esac
  done
}

main "$@"
