//go:build !embedagent

// Package embed 非 embedagent 构建：无内嵌二进制（下载端点走 AGENT_BIN_PATH / 本地文件兜底）。
package embed

// AgentBinary 为空表示未内嵌。
var AgentBinary []byte

// AgentVersion 为空表示非内嵌构建（与 embed.go 符号一致，构建期 -ldflags -X 注入）。
var AgentVersion string
