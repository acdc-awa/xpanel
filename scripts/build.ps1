# 全量构建（Windows）：先交叉编译 Linux agent 到 internal/master/embed/agent-linux，
# 再用 -tags embedagent 把 agent 内嵌进 master.exe（下载端点开箱即用）。
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$EmbedDir = Join-Path $Root "internal/master/embed"
$Version = git -C $Root describe --tags --always 2>$null
if ($LASTEXITCODE -ne 0) { $Version = "dev" }

Write-Host "==> 构建 Linux agent（version=$Version）"
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
& go build "-ldflags" "-X github.com/acdc/xray-panel/internal/agent/upgrade.Version=$Version" `
  -o (Join-Path $EmbedDir "agent-linux") ./cmd/agent
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> 构建 master.exe（embedagent）"
$env:GOOS = ""
$env:GOARCH = ""
& go build -tags embedagent `
  "-ldflags" "-X github.com/acdc/xray-panel/internal/master/embed.AgentVersion=$Version" `
  -o (Join-Path $Root "master.exe") ./cmd/master
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "完成: $Root\master.exe（内嵌 agent $Version）"
