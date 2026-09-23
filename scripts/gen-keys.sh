#!/usr/bin/env bash
# 生成 UUID / short_id / X25519 密钥对,可透传 -server 等参数生成分享链接。
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./cmd/server keys generate "$@"
