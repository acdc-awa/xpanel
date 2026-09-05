package xray

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ─── VLESS Encryption（安全层四选一中的 vlessenc：节点间加密传输）──────────────
//
// xray 层面该加密挂在协议 settings（decryption / encryption 字段），不属于 streamSettings；
// 面板将其抽象为安全层选项：选中时 stream 层写 security:"none"，settings_json 写 decryption，
// 上游中转出站的 encryption 由生成器按目标入站派生（VlessEncClientFromDecryption）。
// 格式（v26.6.27 infra/conf 强制完整点式，裸密钥写法会被拒）：
//
//	decryption: mlkem768x25519plus.{native|xorpub|random}.{Ns|N-Ms}[.padding...].<key32|key64>[.keyN...]
//	encryption: mlkem768x25519plus.{native|xorpub|random}.{0rtt|1rtt}[.padding...].<pub32|pub1184>
//
// 认证模式一期固定 ML-KEM-768（seed 64B → 封装公钥 1184B）；多密钥块为官方预留的轮换通道。
// 外观一期固定 native（random 会使核心拒绝穿透 Splice）。

const vlessEncHandshake = "mlkem768x25519plus"

// GenerateVlessEnc 生成 ML-KEM-768 认证的 decryption/encryption 配对（xray vlessenc 命令的
// 等价实现，纯标准库 crypto/mlkem，与官方二进制输出逐字节一致）。
func GenerateVlessEnc() (decryption, encryption string, err error) {
	seed := make([]byte, 64)
	if _, err = rand.Read(seed); err != nil {
		return "", "", fmt.Errorf("生成 ML-KEM-768 seed 失败: %w", err)
	}
	dk, err := mlkem.NewDecapsulationKey768(seed)
	if err != nil {
		return "", "", fmt.Errorf("生成 ML-KEM-768 密钥失败: %w", err)
	}
	seedB64 := base64.RawURLEncoding.EncodeToString(seed)
	ekB64 := base64.RawURLEncoding.EncodeToString(dk.EncapsulationKey().Bytes())
	return vlessEncHandshake + ".native.600s." + seedB64,
		vlessEncHandshake + ".native.0rtt." + ekB64, nil
}

// VlessDecryptionFromSettings 从入站 settings_json 提取 decryption 值（"" = 未设置 / none）。
func VlessDecryptionFromSettings(settingsJSON string) (string, error) {
	if settingsJSON == "" {
		return "", nil
	}
	var s struct {
		Decryption string `json:"decryption"`
	}
	if err := json.Unmarshal([]byte(settingsJSON), &s); err != nil {
		return "", fmt.Errorf("settings JSON 无效: %w", err)
	}
	if s.Decryption == "none" {
		return "", nil
	}
	return s.Decryption, nil
}

// VlessEncEnabled 判断 decryption 值是否为启用加密（非空且非 none）。
func VlessEncEnabled(decryption string) bool {
	return decryption != "" && decryption != "none"
}

type parsedVlessEnc struct {
	appearance string
	keys       [][]byte
}

// parseVlessEncDecryption 解析 decryption 完整点格式（对齐 v26.6.27 infra/conf 校验规则）：
// 块 0 = 握手名；块 1 = 外观；块 2 = 票据有效期；其后 len<20 的块为 padding（服务端→客户端方向，
// 派生客户端串时丢弃、双端用核心默认），再后为密钥块（base64 RawURL 的 32B x25519 私钥 / 64B
// mlkem768 seed），至少一把。
func parseVlessEncDecryption(s string) (*parsedVlessEnc, error) {
	blocks := strings.Split(s, ".")
	if len(blocks) < 4 || blocks[0] != vlessEncHandshake {
		return nil, fmt.Errorf("需为 %s.{native|xorpub|random}.{600s|100-500s}[.padding...].<密钥> 完整点格式（请用面板密钥生成或 xray vlessenc）", vlessEncHandshake)
	}
	switch blocks[1] {
	case "native", "xorpub", "random":
	default:
		return nil, fmt.Errorf("外观块 %q 非法（可选 native / xorpub / random）", blocks[1])
	}
	parts := strings.SplitN(strings.TrimSuffix(blocks[2], "s"), "-", 2)
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return nil, fmt.Errorf("票据有效期块 %q 非法（如 600s 或 100-500s）", blocks[2])
	}
	if len(parts) == 2 {
		if _, err := strconv.Atoi(parts[1]); err != nil {
			return nil, fmt.Errorf("票据有效期块 %q 非法（如 600s 或 100-500s）", blocks[2])
		}
	}
	p := &parsedVlessEnc{appearance: blocks[1]}
	seenKey := false
	for _, r := range blocks[3:] {
		if len(r) < 20 { // padding 块（对齐 infra/conf 长度判定），仅允许出现在密钥块之前
			if seenKey {
				return nil, fmt.Errorf("padding 块 %q 不能出现在密钥块之后", r)
			}
			continue
		}
		b, err := base64.RawURLEncoding.DecodeString(r)
		if err != nil || (len(b) != 32 && len(b) != 64) {
			return nil, fmt.Errorf("密钥块须为 base64 RawURL 的 32 字节(x25519 私钥)或 64 字节(mlkem768 seed)，块 %q 不合法", r)
		}
		seenKey = true
		p.keys = append(p.keys, b)
	}
	if !seenKey {
		return nil, fmt.Errorf("缺少认证密钥块")
	}
	return p, nil
}

// ValidateVlessEncDecryption 校验 decryption 串格式（保存时预检，避免配置推送后 agent 端
// xray -test 才暴露）。
func ValidateVlessEncDecryption(decryption string) error {
	_, err := parseVlessEncDecryption(decryption)
	return err
}

// VlessEncClientFromDecryption 由落地入站 decryption 派生上游中转出站 encryption 串：
// 外观沿用目标入站（双端必须一致），票据固定 0rtt，密钥派生为对应公钥（x25519 私钥→公钥 /
// mlkem768 seed→封装公钥）；padding 为服务端→客户端方向，客户端串不携带（双端各用核心默认）。
func VlessEncClientFromDecryption(decryption string) (string, error) {
	p, err := parseVlessEncDecryption(decryption)
	if err != nil {
		return "", err
	}
	pubs := make([]string, 0, len(p.keys))
	for i, k := range p.keys {
		switch len(k) {
		case 32:
			priv, err := ecdh.X25519().NewPrivateKey(k)
			if err != nil {
				return "", fmt.Errorf("第 %d 把 x25519 私钥非法: %w", i+1, err)
			}
			pubs = append(pubs, base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()))
		case 64:
			dk, err := mlkem.NewDecapsulationKey768(k)
			if err != nil {
				return "", fmt.Errorf("第 %d 把 mlkem768 seed 非法: %w", i+1, err)
			}
			pubs = append(pubs, base64.RawURLEncoding.EncodeToString(dk.EncapsulationKey().Bytes()))
		}
	}
	return vlessEncHandshake + "." + p.appearance + ".0rtt." + strings.Join(pubs, "."), nil
}
