#!/bin/bash
# 全量构建：先交叉编译 Linux agent 到 internal/master/embed/agent-linux，
# 再用 -tags embedagent 把 agent 内嵌进 master 二进制（下载端点开箱即用）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EMBED="$ROOT/internal/master/embed"
VERSION="$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)"

echo "==> 构建 Linux agent（version=$VERSION）"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags "-X github.com/zhx/xray-panel/internal/agent/upgrade.Version=$VERSION" \
  -o "$EMBED/agent-linux" ./cmd/agent

echo "==> 构建 master（embedagent）"
go build -tags embedagent \
  -ldflags "-X github.com/zhx/xray-panel/internal/master/embed.AgentVersion=$VERSION" \
  -o "$ROOT/master" ./cmd/master
echo "完成: $ROOT/master（内嵌 agent $VERSION，$(du -h "$EMBED/agent-linux" | cut -f1)）"
