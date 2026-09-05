package xray_test

import (
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/xray"
)

// 与官方 xray v26.6.27 二进制逐字节对齐的金样（xray mlkem768 / xray x25519 实测输出）：
// 零值 seed 的派生结果与固定 x25519 私钥的公钥，钉死面板生成/派生与官方实现一致。
const (
	goldenMlkemSeed = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      // 64B 零值 seed（86 字符）
	goldenMlkemEK   = "JUp5eIXGOxRAqjicZTQO8zUgzAOaqNdJrnCVuoSFokRPgHAHQTJ8NjpFe4U4sTtu1vE8KbIyUYxwThKGp0hn06q2ByldGnSDh2WT3OgDsfpCZWy7U1Ux07dtGPkw89Gd9KAtTGiI1VlrP7OCJXpB4-JS60hl2RBeh9eIj2Q0hfWzAL11XicF6dNmxzeG7acdELFRZGHI0cuRz5chSYZyEoyTXgRRLgciN3K4BocRI7CMQFmnp1QVxLqF_QdgPThhPgG5hnIDw6EqGfhO-5uOaXs1gUVYM8xIQ5UzUgytE7uwEXGGNkGzLiIx-IcOUGVbnCWMtUetp9eHIqzOWonLu9sWJzx3bHakU6p6HpOhA1CU6ftfeQl1VnE4QUHPwmgPT3dR-aHB37e55WNYHrl1JVWxqxhlp2kBI2ZKblYPhAe--GvE2hjACMaGSkdYvKYtpaGLqjMciXtJ_LAsK0cVIWMvWfHPAxZoYrEkoaw1gfO_ijUex5yHQoRjNksLO9FdNZdg2auPq7F76QeHQaGimvxapHh3Lss-M-CwgRlcEuXBWUNNKbwpqxINbRhOEWhG2oeba_ipuWcCYSYTqaohTkuit7G6f7QI0VQdiYO1Cgy7TghGfzVyxJtNyCqkoRqiaoUJcGibTumqlIe2DtZTYlcXhbRMPewygrmJeEOmjEN6LDgbZglf_3lZf_EHzcsYE7EA7aI9vfaiOfQEtIpX2mYjS6fAcPVp8Pi54SWsiIcH8XB9K0ViiTon5NxbqRtytltzV7pcPDOf6pw-eLQh9DHDFEIeURdoNOWeK4mamcOOSEeS07woc-h4QtPFX2hKSpQMamOioWij6zNo--g_jeU1JOeHwYJCGrKGF7Eg0Al4ERu1jgGw77Rj0-t2n8ZuSlm5Svanq-RU6WCuPgs7TmuRO7c__HsrNFN3uSUbxmE6dNAYd1QbjMMTjymabycoquuJxdIIo0cfcaiGSpvMUwWAJRpnQVNaYtig4XGBK_mIRqKEBXBJnbQZMmQEiEh7EIxy1Ct6fcVEuXGjL9qAZnactHAJBeImW3pDcNjAPuJ_jBl2pJnBNMglEkE3ePiEsXcannmHMhiQxkqhnNePTRkj9ZSUSASPpecTWpND22aR5rGAVneQRGZxFfvBRDtudKkjJ5WGzydimuKFyhAOEGLM9WyWudcg7akugbiuGZSlCFiKUDMPc9RvmYGZ9zhwEUak3bQeeFyYj8UetxuOSSdbf3ul5ZMArhLKD9FS0DnHbfBSbONNvquhfRGmsAYMphQBzsNf6bF14nUXpBxLigcFb8EOxvYmd_YrdoQL2RqVIMSoGcg9gEMJQSR6WWRilsy-p9waswFw3jhiTooztMK6Lcl79zpx2aw1pICoPxux4cR3dAnQoGV4KhpwewaNS_IgUQEgL8vHGrlLKSnLROLFR-wQtactjIdZ-ykgrcgLhXE5dUFLeyC5g-hI06DDvEM7vya3AKGkAyKoTeXIfrmHetaKvpiG99tpiZZAAupWbMYWRjsX4OeD5aliF6IAbTrkBnY8UEV9FIFAKq_H4j9D-dHXwK9wYKwdqp7LDmc" // 同 seed 的封装公钥（1184B，1579 字符）

	goldenX25519Priv = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA" // bytes 1..32
	goldenX25519Pub  = "B6N8vBQgk8i3VdwbEOhstCY3StFqqFPtC9_AsrhtHHw" // clamp 后公钥（32B，43 字符）
)

func TestGenerateVlessEnc(t *testing.T) {
	decryption, encryption, err := xray.GenerateVlessEnc()
	if err != nil {
		t.Fatalf("GenerateVlessEnc: %v", err)
	}
	if err := xray.ValidateVlessEncDecryption(decryption); err != nil {
		t.Fatalf("生成的 decryption 未通过格式自校验: %v", err)
	}
	got, err := xray.VlessEncClientFromDecryption(decryption)
	if err != nil {
		t.Fatalf("配对派生失败: %v", err)
	}
	if got != encryption {
		t.Errorf("decryption 派生结果与 encryption 不一致:\n got %s\nwant %s", got, encryption)
	}
	// ML-KEM-768 尺寸：seed 64B → 86 字符；封装公钥 1184B → 1579 字符
	if !strings.HasPrefix(decryption, "mlkem768x25519plus.native.600s.") || len(decryption) != len("mlkem768x25519plus.native.600s.")+86 {
		t.Errorf("decryption 形状异常: %s (len=%d)", decryption, len(decryption))
	}
	if !strings.HasPrefix(encryption, "mlkem768x25519plus.native.0rtt.") || len(encryption) != len("mlkem768x25519plus.native.0rtt.")+1579 {
		t.Errorf("encryption 形状异常: len=%d", len(encryption))
	}
}

func TestVlessEncClientFromDecryption_Golden(t *testing.T) {
	cases := []struct {
		name       string
		decryption string
		want       string
	}{
		{
			name:       "mlkem768",
			decryption: "mlkem768x25519plus.native.600s." + goldenMlkemSeed,
			want:       "mlkem768x25519plus.native.0rtt." + goldenMlkemEK,
		},
		{
			name:       "x25519（二期认证通道，派生逻辑保持与官方一致）",
			decryption: "mlkem768x25519plus.native.600s." + goldenX25519Priv,
			want:       "mlkem768x25519plus.native.0rtt." + goldenX25519Pub,
		},
		{
			name:       "票据区间格式 100-500s",
			decryption: "mlkem768x25519plus.native.100-500s." + goldenX25519Priv,
			want:       "mlkem768x25519plus.native.0rtt." + goldenX25519Pub,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := xray.VlessEncClientFromDecryption(c.decryption)
			if err != nil {
				t.Fatalf("派生失败: %v", err)
			}
			if got != c.want {
				t.Errorf("派生不一致:\n got %s\nwant %s", got, c.want)
			}
			if err := xray.ValidateVlessEncDecryption(c.decryption); err != nil {
				t.Errorf("合法串被校验拒绝: %v", err)
			}
		})
	}
}

func TestValidateVlessEncDecryption(t *testing.T) {
	valid := "mlkem768x25519plus.native.600s." + goldenX25519Priv
	multi := "mlkem768x25519plus.native.600s." + goldenX25519Priv + "." + goldenMlkemSeed
	bad := []struct {
		name  string
		value string
	}{
		{"裸密钥（v26.6.27 拒绝）", goldenX25519Priv},
		{"缺握手名", "native.600s." + goldenX25519Priv},
		{"外观非法", "mlkem768x25519plus.plain.600s." + goldenX25519Priv},
		{"票据块非法", "mlkem768x25519plus.native.abc." + goldenX25519Priv},
		{"密钥块非 base64", "mlkem768x25519plus.native.600s.不是base64!!"},
		{"密钥块长度非法", "mlkem768x25519plus.native.600s." + strings.Repeat("A", 40)}, // 40 字符 = 30B，长度非法
		{"只有 padding 无密钥", "mlkem768x25519plus.native.600s.100-111-1111"},
		{"padding 在密钥后", "mlkem768x25519plus.native.600s." + goldenX25519Priv + ".100-111-1111"},
		{"块数不足", "mlkem768x25519plus.native.600s"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if err := xray.ValidateVlessEncDecryption(c.value); err == nil {
				t.Errorf("应拒绝: %s", c.value)
			}
		})
	}
	for _, v := range []string{valid, multi} {
		if err := xray.ValidateVlessEncDecryption(v); err != nil {
			t.Errorf("应通过: %s → %v", v, err)
		}
	}
}

func TestVlessEncMultiKeyDerive(t *testing.T) {
	// 多密钥轮换通道：两把密钥 → 两个派生公钥
	got, err := xray.VlessEncClientFromDecryption("mlkem768x25519plus.native.600s." + goldenX25519Priv + "." + goldenMlkemSeed)
	if err != nil {
		t.Fatalf("多密钥派生失败: %v", err)
	}
	want := "mlkem768x25519plus.native.0rtt." + goldenX25519Pub + "." + goldenMlkemEK
	if got != want {
		t.Errorf("多密钥派生不一致:\n got %s\nwant %s", got, want)
	}
}

func TestVlessDecryptionFromSettings(t *testing.T) {
	cases := []struct {
		name   string
		json   string
		want   string
		wantOK bool
	}{
		{"空 settings", "", "", true},
		{"显式 none", `{"decryption":"none"}`, "", true},
		{"缺字段", `{"fallbacks":[]}`, "", true},
		{"正常", `{"decryption":"mlkem768x25519plus.native.600s.` + goldenX25519Priv + `"}`, "mlkem768x25519plus.native.600s." + goldenX25519Priv, true},
		{"非法 JSON", `{`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := xray.VlessDecryptionFromSettings(c.json)
			if c.wantOK && err != nil {
				t.Fatalf("应成功: %v", err)
			}
			if !c.wantOK && err == nil {
				t.Fatal("应报错")
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}
