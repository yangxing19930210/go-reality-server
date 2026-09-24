#!/usr/bin/env bash
# 服务器一条命令部署:装二进制 →(首次)现场生成密钥 → systemd 启动 →
# 防火墙放行 → 验证 /ready → 打印客户端分享链接。
# 用法:
#   sudo ./bootstrap.sh                     # 架构自动识别,出口 IP 自动探测
#   sudo ./bootstrap.sh --arch arm64        # 手动指定架构
#   sudo ./bootstrap.sh --server 1.2.3.4    # 手动指定分享链接中的服务器地址
# 幂等:重复运行不会重置已有密钥与配置。
set -euo pipefail

PKG_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DST="/usr/local/bin/go-reality-server"
CONF_DIR="/etc/go-reality"
ARCH=""
SERVER_ADDR=""
SERVER_GIVEN=0

usage() { sed -n '3,7p' "$0" | sed 's/^# \{0,1\}//'; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)     ARCH="${2:?--arch 需要值}"; shift 2 ;;
    --arch=*)   ARCH="${1#--arch=}"; shift ;;
    --server)   SERVER_ADDR="${2:?--server 需要值}"; SERVER_GIVEN=1; shift 2 ;;
    --server=*) SERVER_ADDR="${1#--server=}"; SERVER_GIVEN=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

[[ $EUID -eq 0 ]] || { echo "❌ 请以 root 运行:sudo ./bootstrap.sh"; exit 1; }

if [[ -z "$ARCH" ]]; then
  case "$(uname -m)" in
    x86_64)  ARCH=amd64 ;;
    aarch64) ARCH=arm64 ;;
    *) echo "❌ 无法识别架构 $(uname -m),请用 --arch amd64 或 --arch arm64 指定" >&2; exit 1 ;;
  esac
fi
BIN_SRC="$PKG_DIR/go-reality-server-linux-$ARCH"
if [[ ! -f "$BIN_SRC" ]]; then
  echo "❌ 没找到 $BIN_SRC" >&2
  ls "$PKG_DIR"/go-reality-server-linux-* 2>/dev/null | sed 's/^/   - /' >&2
  exit 1
fi

echo "==> 安装二进制 $BIN_DST"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "==> 准备 $CONF_DIR(0755:DynamicUser 每次启动随机 uid,须可进入)"
mkdir -p "$CONF_DIR"
chmod 0755 "$CONF_DIR"

# config.yaml:内容无秘密(密钥全走 env),仅首次写入
if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  [[ -f "$PKG_DIR/config.yaml" ]] || { echo "❌ 包内缺 config.yaml" >&2; exit 1; }
  install -m 0644 "$PKG_DIR/config.yaml" "$CONF_DIR/config.yaml"
fi

# env:密钥唯一落点。仅首次生成:0600 root,systemd 降权前以 root 读取
if [[ ! -f "$CONF_DIR/env" ]]; then
  if [[ -z "$SERVER_ADDR" ]]; then
    SERVER_ADDR="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}')" || true
  fi
  if [[ -z "$SERVER_ADDR" ]]; then
    echo "❌ 无法探测服务器出口 IP,请显式指定:sudo ./bootstrap.sh --server <IP或域名>" >&2
    exit 1
  fi
  echo "==> 现场生成密钥(链接地址 $SERVER_ADDR)"
  umask 077
  "$BIN_DST" keys generate -format env -server "$SERVER_ADDR" > "$CONF_DIR/env.new"
  chmod 0600 "$CONF_DIR/env.new"
  mv "$CONF_DIR/env.new" "$CONF_DIR/env"
  # 客户端资料 = env 注释区剥去 '# ' 前缀,同样 0600 root
  grep '^#' "$CONF_DIR/env" | sed 's/^# \{0,1\}//' > "$CONF_DIR/client.txt" || true
  chmod 0600 "$CONF_DIR/client.txt"
  umask 022
fi

if [[ -f "$CONF_DIR/env" && "$SERVER_GIVEN" -eq 1 ]]; then
  echo "⚠️ 密钥已存在($CONF_DIR/env),--server $SERVER_ADDR 未生效;如需换新密钥与链接:rm $CONF_DIR/env 后重跑" >&2
fi

echo "==> 安装 systemd unit"
[[ -f "$PKG_DIR/go-reality.service" ]] || { echo "❌ 包内缺 go-reality.service" >&2; exit 1; }
install -m 0644 "$PKG_DIR/go-reality.service" /etc/systemd/system/go-reality.service
systemctl daemon-reload
if systemctl is-enabled --quiet go-reality 2>/dev/null; then
  systemctl restart go-reality
else
  systemctl enable --now go-reality
fi

echo "==> 防火墙放行 443/tcp(仅当防火墙在运行)"
if command -v ufw >/dev/null && LC_ALL=C ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow 443/tcp
elif command -v firewall-cmd >/dev/null && firewall-cmd --state 2>/dev/null | grep -q running; then
  firewall-cmd --permanent --add-port=443/tcp
  firewall-cmd --reload
fi

echo "==> 等待服务就绪(最多 15s)"
ok=""
for _ in $(seq 1 15); do
  if curl -fsS http://127.0.0.1:9090/ready >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done
if [[ -z "$ok" ]]; then
  echo "❌ 服务未就绪。日志:journalctl -u go-reality -n 50 --no-pager" >&2
  systemctl status go-reality --no-pager >&2 || true
  exit 1
fi

echo ""
echo "✅ 部署完成"
echo "   状态:   systemctl status go-reality --no-pager"
echo "   就绪:   curl http://127.0.0.1:9090/ready"
echo "   日志:   journalctl -u go-reality -f"
if [[ -f "$CONF_DIR/client.txt" ]]; then
  echo "   客户端: sudo cat $CONF_DIR/client.txt"
  SHARE="$(grep -m1 -o 'vless://[^ ]*' "$CONF_DIR/client.txt" || true)"
  if [[ -n "$SHARE" ]]; then
    echo ""
    echo "   分享链接(可直接导入 Clash Meta / v2rayN / NekoBox):"
    echo "   $SHARE"
  fi
fi
