package api

import (
	"testing"
)

// TestMaskSensitivePath 订阅 token 路径必须脱敏，其他路径原样保留。
func TestMaskSensitivePath(t *testing.T) {
	if got := maskSensitivePath("/api/v1/sub/EVAL_SECRET_TOKEN_123456"); got != "/api/v1/sub/***" {
		t.Fatalf("sub path = %q, want masked", got)
	}
	if got := maskSensitivePath("/api/v1/user/me"); got != "/api/v1/user/me" {
		t.Fatalf("normal path should not change, got %q", got)
	}
	if got := maskSensitivePath("/api/v1/sub"); got != "/api/v1/sub" {
		t.Fatalf("exact sub path should not change, got %q", got)
	}
}
