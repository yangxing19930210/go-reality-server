#!/usr/bin/env bash
# 组装服务器部署包:双架构二进制 + bootstrap.sh + systemd unit + 无秘密
# config.yaml,打成单文件 dist/go-reality-server-linux.tar.gz(一个 scp)。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> 交叉编译 linux/amd64 与 linux/arm64"
./scripts/cross-build.sh

PKG=dist/go-reality-server
echo "==> 组装 $PKG/"
rm -rf "$PKG" dist/go-reality-server-linux.tar.gz
mkdir -p "$PKG"
# 注意:不用 install——开发机可能是 git-bash(无 coreutils install)
cp bin/go-reality-server-linux-amd64 bin/go-reality-server-linux-arm64 "$PKG/"
chmod 0755 "$PKG"/go-reality-server-linux-*
cp deploy/bootstrap.sh "$PKG/bootstrap.sh"
chmod 0755 "$PKG/bootstrap.sh"
cp deploy/systemd/go-reality.service "$PKG/"
cp config.example.yaml "$PKG/config.yaml"

echo "==> 打包 dist/go-reality-server-linux.tar.gz"
tar czf dist/go-reality-server-linux.tar.gz -C dist go-reality-server

echo "✅ dist/go-reality-server-linux.tar.gz"
echo "   服务器: scp 上去后 tar xzf && cd go-reality-server && sudo ./bootstrap.sh"
