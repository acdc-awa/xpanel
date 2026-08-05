//go:build embedagent

// Package embed 用 go:embed 把 Linux agent 二进制内嵌进 master 二进制，
// 使 /api/v1/download/agent 开箱即用（无需 AGENT_BIN_PATH 或额外文件）。
// 构建前请运行 scripts/build.sh（或 build.ps1）生成 agent-linux。
package embed

import _ "embed"

//go:embed agent-linux
var AgentBinary []byte

// AgentVersion 内嵌 agent 的版本（构建期 -ldflags -X 注入；与 scripts/build.* 同步）。
var AgentVersion string
