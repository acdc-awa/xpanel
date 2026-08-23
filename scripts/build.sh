#!/bin/bash
# 构建 master 面板二进制。
# agent 已迁至 XPanel-Node 仓库独立发布（GitHub Releases），面板不再内嵌/分发 agent。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)"

echo "==> 构建 master（version=$VERSION）"
go build -o "$ROOT/master" ./cmd/master
echo "完成: $ROOT/master"
