# 全量构建（Windows）：先交叉编译 Linux agent 到 internal/master/embed/agent-linux，
# 再用 -tags embedagent 把 agent 内嵌进 master.exe（下载端点开箱即用）。
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$EmbedDir = Join-Path $Root "internal/master/embed"

Write-Host "==> 构建 Linux agent"
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
& go build -o (Join-Path $EmbedDir "agent-linux") ./cmd/agent
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> 构建 master.exe（embedagent）"
$env:GOOS = ""
$env:GOARCH = ""
& go build -tags embedagent -o (Join-Path $Root "master.exe") ./cmd/master
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "完成: $Root\master.exe（已内嵌 agent-linux）"
