#!/bin/bash
# 全量构建：先交叉编译 Linux agent 到 internal/master/embed/agent-linux，
# 再用 -tags embedagent 把 agent 内嵌进 master 二进制（下载端点开箱即用）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EMBED="$ROOT/internal/master/embed"

echo "==> 构建 Linux agent"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$EMBED/agent-linux" ./cmd/agent

echo "==> 构建 master（embedagent）"
go build -tags embedagent -o "$ROOT/master" ./cmd/master
echo "完成: $ROOT/master（已内嵌 agent-linux，$(du -h "$EMBED/agent-linux" | cut -f1)）"
