# 构建 master.exe（Windows）。
# agent 已迁至 XPanel-Node 仓库独立发布（GitHub Releases），面板不再内嵌/分发 agent。
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Version = git -C $Root describe --tags --always 2>$null
if ($LASTEXITCODE -ne 0) { $Version = "dev" }

Write-Host "==> 构建 master.exe（version=$Version）"
& go build -o (Join-Path $Root "master.exe") ./cmd/master
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "完成: $Root\master.exe"
