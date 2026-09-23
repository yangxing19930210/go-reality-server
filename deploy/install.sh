#!/usr/bin/env bash
# 服务器端一键安装:把交叉编译产物装为 systemd 服务。
# 用法: sudo bash install.sh /path/to/go-reality-server-linux-amd64
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="${1:-go-reality-server-linux-amd64}"
CONF_DIR="/etc/go-reality"

echo "==> 安装二进制到 /usr/local/bin"
install -m 0755 "$BIN" /usr/local/bin/go-reality-server

echo "==> 创建配置目录 $CONF_DIR"
mkdir -p "$CONF_DIR"
if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  install -m 0600 "$SCRIPT_DIR/config.example.yaml" "$CONF_DIR/config.yaml"
  echo "    已写入示例配置,请编辑 $CONF_DIR/config.yaml 填入密钥"
fi
if [[ ! -f "$CONF_DIR/env" ]]; then
  touch "$CONF_DIR/env"
fi
chmod 0600 "$CONF_DIR/env"

echo "==> 安装 systemd unit"
UNIT_SRC="$SCRIPT_DIR/go-reality.service"
if [[ ! -f "$UNIT_SRC" ]]; then
  UNIT_SRC="$SCRIPT_DIR/deploy/systemd/go-reality.service"
fi
install -m 0644 "$UNIT_SRC" /etc/systemd/system/go-reality.service
systemctl daemon-reload
systemctl enable --now go-reality

echo "==> 完成。填入密钥后执行: sudo systemctl restart go-reality"
echo "    查看日志: journalctl -u go-reality -f -n 50"
