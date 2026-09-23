#!/usr/bin/env bash
# 交叉编译 linux/amd64 与 linux/arm64 产物到 bin/。
set -euo pipefail
cd "$(dirname "$0")/.."

go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-amd64 ./cmd/server
echo "built bin/go-reality-server-linux-amd64"

go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-arm64 ./cmd/server
echo "built bin/go-reality-server-linux-arm64"
