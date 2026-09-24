#!/usr/bin/env bash
# 一次命令完成全部部署:装二进制 → 写 unit → 启用密钥 → 启动服务 → 打防火墙。
# 用法:
#   sudo bash bootstrap.sh                 # 用包内同目录的二进制(amd64)
#   sudo bash bootstrap.sh --arch arm64   # 改用 arm64
# 幂等:可重复运行不会破坏已有配置。
set -euo pipefail

PKG_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/usr/local/bin"
CONF_DIR="/etc/go-reality"
ARCH="amd64"

for arg in "$@"; do
  case "$arg" in
    --arch)  shift; ARCH="${1:-amd64}" ;;
    --arch=*) ARCH="${arg#--arch=}"     ;;
    -h|--help)
      sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

BIN_SRC="$PKG_DIR/go-reality-server-linux-${ARCH}"
if [[ ! -f "$BIN_SRC" ]]; then
  echo "❌ 没找到 $BIN_SRC" >&2
  echo "   包内有:" >&2
  ls "$PKG_DIR"/go-reality-server-linux-* 2>/dev/null | sed 's/^/   - /' >&2 || echo "   - (无)" >&2
  exit 1
fi

[[ $EUID -eq 0 ]] || { echo "❌ 需 root"; exit 1; }

echo "==> 安装二进制到 $INSTALL_DIR"
install -m 0755 "$BIN_SRC" "$INSTALL_DIR/go-reality-server"

echo "==> 准备 /etc/go-reality"
mkdir -p "$CONF_DIR"
chmod 700 "$CONF_DIR"

# config: 仅在首次引导时安装包内预生成的 config.yaml
if [[ ! -f "$CONF_DIR/config.yaml" && -f "$PKG_DIR/config.yaml" ]]; then
  install -m 0644 "$PKG_DIR/config.yaml" "$CONF_DIR/config.yaml"
  echo "    写入预生成 config.yaml"
fi

# env: 仅在首次引导时安装(0600)
if [[ ! -f "$CONF_DIR/env" && -f "$PKG_DIR/env" ]]; then
  install -m 0600 "$PKG_DIR/env" "$CONF_DIR/env"
  echo "    写入预生成 env(密钥)"
fi
chmod 0600 "$CONF_DIR/env" 2>/dev/null || true

echo "==> 安装 systemd unit"
UNIT_SRC="$PKG_DIR/go-reality.service"
if [[ ! -f "$UNIT_SRC" ]]; then
  UNIT_SRC="$PKG_DIR/deploy/systemd/go-reality.service"
fi
if [[ ! -f "$UNIT_SRC" ]]; then
  echo "❌ 找不到 go-reality.service" >&2; exit 1
fi
install -m 0644 "$UNIT_SRC" /etc/systemd/system/go-reality.service

systemctl daemon-reload
systemctl enable go-reality
systemctl restart go-reality

echo "==> 防火墙(若启用 ufw)"
if command -v ufw >/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow 443/tcp
fi

echo ""
echo "✅ 部署完成"
echo "   状态:  systemctl status go-reality --no-pager"
echo "   就绪:  curl http://127.0.0.1:9090/ready  (期望: ready)"
echo "   日志:  journalctl -u go-reality -n 30"
echo "   链接:  见包内 keys.txt 或 client.clash.yaml"
