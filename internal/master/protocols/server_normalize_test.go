package protocols

import (
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
)

// S1：服务端协议语义全部收敛到插件的 NormalizeServer / ResolveRelayFlow。
// 生成器不得再出现协议分支；这些用例锁定插件侧的强约束与派生默认。

func TestVLESSNormalizeServer(t *testing.T) {
	const validDec = "mlkem768x25519plus.native.600s.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	cases := []struct {
		name        string
		settings    map[string]any
		wantDec     string
		wantErr     bool
		wantKeyGone string
	}{
		{name: "默认补 none", settings: nil, wantDec: "none"},
		{name: "剥掉入站级 encryption", settings: map[string]any{"encryption": "legacy", "decryption": "none"}, wantDec: "none", wantKeyGone: "encryption"},
		{name: "合法 vlessenc 原样保留", settings: map[string]any{"decryption": validDec}, wantDec: validDec},
		{name: "非法 vlessenc 报错", settings: map[string]any{"decryption": "cNUvTuN33QybBBHanFzy8LFC0wyvAsa__HSc0-KRRWo"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeDec, beforeEnc, beforeHasEnc := "", "", false
			if tc.settings != nil {
				beforeDec, _ = tc.settings["decryption"].(string)
				beforeEnc, beforeHasEnc = tc.settings["encryption"].(string)
			}
			spec := &contracts.InboundSpec{Protocol: "vless", Settings: tc.settings}
			ov, err := (VLESSPlugin{}).NormalizeServer(spec)
			if tc.wantErr {
				if err == nil {
					t.Fatal("期望报错")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeServer: %v", err)
			}
			if ov == nil || ov.Settings == nil {
				t.Fatal("应返回 settings 覆盖")
			}
			if got, _ := ov.Settings["decryption"].(string); got != tc.wantDec {
				t.Errorf("decryption = %q, want %q", got, tc.wantDec)
			}
			if tc.wantKeyGone != "" {
				if _, has := ov.Settings[tc.wantKeyGone]; has {
					t.Errorf("%s 应被剥离: %+v", tc.wantKeyGone, ov.Settings)
				}
			}
			// 纯函数：不得修改入参
			if tc.settings != nil {
				afterDec, _ := tc.settings["decryption"].(string)
				afterEnc, afterHasEnc := tc.settings["encryption"].(string)
				if afterDec != beforeDec || afterEnc != beforeEnc || afterHasEnc != beforeHasEnc {
					t.Errorf("入参 spec.Settings 被修改: %+v", tc.settings)
				}
			}
		})
	}
}

func TestVLESSResolveRelayFlow(t *testing.T) {
	const validDec = "mlkem768x25519plus.native.600s.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	p := VLESSPlugin{}
	cases := []struct {
		name     string
		network  string
		security string
		dec      string
		inbFlow  string
		want     string
	}{
		{"tcp+reality 自动 vision", "tcp", "reality", "", "", "xtls-rprx-vision"},
		{"tcp+tls 中继自动 vision", "tcp", "tls", "", "", "xtls-rprx-vision"},
		{"tcp+vlessenc 中继自动 vision", "tcp", "none", validDec, "", "xtls-rprx-vision"},
		{"tcp+none 无 flow", "tcp", "none", "", "", ""},
		{"中继覆盖显式 none（历史行为）", "tcp", "tls", "", "none", "xtls-rprx-vision"},
		{"显式 flow 原样保留", "tcp", "none", "", "xtls-rprx-vision", "xtls-rprx-vision"},
		{"xhttp 不自动注入", "xhttp", "tls", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := &contracts.InboundSpec{Network: tc.network, Security: tc.security, Settings: map[string]any{"decryption": tc.dec}}
			if got := p.ResolveRelayFlow(spec, tc.inbFlow); got != tc.want {
				t.Errorf("ResolveRelayFlow = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHysteriaNormalizeServer(t *testing.T) {
	t.Run("默认补齐", func(t *testing.T) {
		spec := &contracts.InboundSpec{Protocol: "hysteria"}
		ov, err := (HysteriaPlugin{}).NormalizeServer(spec)
		if err != nil {
			t.Fatalf("NormalizeServer: %v", err)
		}
		if ov.Settings["version"] != 2 {
			t.Errorf("settings.version = %v, want 2", ov.Settings["version"])
		}
		if ov.Stream["network"] != "hysteria" || ov.Stream["security"] != "tls" {
			t.Errorf("stream 强制项缺失: %+v", ov.Stream)
		}
		hy, _ := ov.Stream["hysteriaSettings"].(map[string]any)
		if hy["version"] != 2 || hy["udpIdleTimeout"] != 60 {
			t.Errorf("hysteriaSettings 默认值错误: %+v", hy)
		}
	})

	t.Run("合并已有并剥离 auth", func(t *testing.T) {
		spec := &contracts.InboundSpec{
			Protocol: "hysteria",
			Stream: map[string]any{
				"hysteriaSettings": map[string]any{"version": 1, "auth": "shared", "udpIdleTimeout": 30},
			},
		}
		ov, err := (HysteriaPlugin{}).NormalizeServer(spec)
		if err != nil {
			t.Fatalf("NormalizeServer: %v", err)
		}
		hy, _ := ov.Stream["hysteriaSettings"].(map[string]any)
		if hy["version"] != 2 || hy["udpIdleTimeout"] != 30 {
			t.Errorf("已有字段应保留、version 强制 2: %+v", hy)
		}
		if _, has := hy["auth"]; has {
			t.Error("服务端 hysteriaSettings 不得保留共享密码 auth")
		}
		if _, has := spec.Stream["hysteriaSettings"].(map[string]any)["auth"]; !has {
			t.Error("入参 spec.Stream 被修改")
		}
	})

	if got := (HysteriaPlugin{}).ResolveRelayFlow(&contracts.InboundSpec{}, "none"); got != "" {
		t.Errorf("Hysteria 无 flow, got %q", got)
	}
}

func TestNormalizeServer_UnregisteredProtocolPassthrough(t *testing.T) {
	ov, err := NormalizeServer("vmess", &contracts.InboundSpec{Protocol: "vmess"})
	if err != nil || ov != nil {
		t.Errorf("未注册协议应原样透传: ov=%+v err=%v", ov, err)
	}
	if got := ResolveRelayFlow("vmess", &contracts.InboundSpec{}, ""); got != "" {
		t.Errorf("未注册协议 flow 应为空, got %q", got)
	}
}
