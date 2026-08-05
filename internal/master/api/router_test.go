package api

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestAgentDownloadHeaders 头部计算纯函数：版本头为空/非空两分支、sha256 与
// sha256.Sum256(data) 一致、空数据也正确（agent 升级契约的承重缝）。
func TestAgentDownloadHeaders(t *testing.T) {
	data := []byte("agent-binary")
	sum := sha256.Sum256(data)
	wantSHA := hex.EncodeToString(sum[:])

	// 非空版本：返回版本头 + sha256 头
	vh, sh := agentDownloadHeaders(data, "v1.2.3")
	if vh != "v1.2.3" {
		t.Errorf("版本头 = %q, want %q", vh, "v1.2.3")
	}
	if sh != wantSHA {
		t.Errorf("sha256 头 = %q, want %q", sh, wantSHA)
	}

	// 空版本（如非内嵌构建）：不发送版本头（返回空串），sha256 头仍应给出
	vh, sh = agentDownloadHeaders(data, "")
	if vh != "" {
		t.Errorf("空版本应返回空版本头, got %q", vh)
	}
	if sh != wantSHA {
		t.Errorf("sha256 头 = %q, want %q", sh, wantSHA)
	}

	// 空数据：sha256 应为 e3b0c442...（空串哈希），版本头仍为空
	vh, sh = agentDownloadHeaders(nil, "")
	if vh != "" {
		t.Errorf("空版本应返回空版本头, got %q", vh)
	}
	emptySum := sha256.Sum256(nil)
	if sh != hex.EncodeToString(emptySum[:]) {
		t.Errorf("空数据 sha256 = %q, want %q", sh, hex.EncodeToString(emptySum[:]))
	}
}
