// Package xray 负责从「服务器 + 入站 + 用户」生成 Xray 配置。
// 支持协议：VLESS（tcp+reality+vision / ws+tls / xhttp+reality）；其他协议后续扩展。
package xray

import (
	"encoding/json"
	"fmt"

	"github.com/zhx/xray-panel/internal/models"
)

// InboundSettings 对应 inbounds.settings_json（主控既用于生成服务端配置，也用于生成订阅）。
type InboundSettings struct {
	Reality *RealitySettings `json:"reality,omitempty"`
	WS      *WSSettings      `json:"ws,omitempty"`
	XHTTP   *XHTTPSettings   `json:"xhttp,omitempty"`
	TLS     *TLSSettings     `json:"tls,omitempty"`
}

type RealitySettings struct {
	ServerName string `json:"server_name"` // 客户端 SNI
	PublicKey  string `json:"public_key"`  // 客户端公钥
	ShortID    string `json:"short_id"`    // 客户端 shortId
	PrivateKey string `json:"private_key"` // 服务端私钥（不出现在订阅）
	Dest       string `json:"dest"`        // 服务端借壳目标 host:port
}

type WSSettings struct {
	Path string `json:"path"`
	Host string `json:"host"`
}

type XHTTPSettings struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
}

type TLSSettings struct {
	ServerName string `json:"server_name"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
}

// UserEmail 用户在 Xray 中的 email（固定格式，stats 上报按此回查 user_id）。
func UserEmail(userID uint64) string {
	return fmt.Sprintf("user-%d@panel.local", userID)
}

// ParseSettings 解析入站 settings_json。
func ParseSettings(inb *models.Inbound) (*InboundSettings, error) {
	s := &InboundSettings{}
	if inb.SettingsJSON == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(inb.SettingsJSON), s); err != nil {
		return nil, fmt.Errorf("入站 %s settings_json 解析失败: %w", inb.Tag, err)
	}
	return s, nil
}

// Generate 生成完整 Xray 配置（含 api/policy/stats 三段 + 全部启用入站 + 全部启用用户）。
func Generate(server *models.Server, inbounds []models.Inbound, users []models.User) ([]byte, error) {
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService", "RoutingService"},
		},
		"outbounds": []any{
			map[string]any{"protocol": "freedom", "tag": "direct",
				"settings": map[string]any{"finalRules": []any{
					map[string]any{"action": "allow", "ip": []string{"198.18.0.0/15"}},
				}}},
			map[string]any{"protocol": "blackhole", "tag": "blocked"},
		},
		"policy": map[string]any{
			"levels": map[string]any{"0": map[string]any{
				"statsUserUplink":   true,
				"statsUserDownlink": true,
				"statsUserOnline":   true,
			}},
			"system": map[string]any{
				"statsInboundUplink":   true,
				"statsInboundDownlink": true,
			},
		},
		"routing": map[string]any{
			"rules": []any{map[string]any{
				"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api",
			}},
		},
		"stats": map[string]any{},
	}

	inboundList := []any{}
	for _, inb := range inbounds {
		if !inb.Enabled {
			continue
		}
		item, err := buildInbound(&inb, users)
		if err != nil {
			return nil, fmt.Errorf("生成入站 %s 失败: %w", inb.Tag, err)
		}
		inboundList = append(inboundList, item)
	}
	// api 入站（gRPC）
	inboundList = append(inboundList, map[string]any{
		"tag": "api", "listen": "127.0.0.1", "port": 10085,
		"protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"},
	})
	cfg["inbounds"] = inboundList

	return json.MarshalIndent(cfg, "", "  ")
}

// buildInbound 生成单个入站段（VLESS）。
func buildInbound(inb *models.Inbound, users []models.User) (map[string]any, error) {
	if inb.Protocol != "vless" {
		return nil, fmt.Errorf("暂不支持协议 %q（当前仅 VLESS）", inb.Protocol)
	}
	settings, err := ParseSettings(inb)
	if err != nil {
		return nil, err
	}

	clients := make([]any, 0, len(users))
	for _, u := range users {
		if u.Status != models.StatusActive || u.UUID == "" {
			continue
		}
		c := map[string]any{
			"id":    u.UUID,
			"email": UserEmail(u.ID),
			"level": 0,
		}
		// TCP+REALITY 用 vision 流控
		if inb.Network == "tcp" && inb.TLSType == "reality" {
			c["flow"] = "xtls-rprx-vision"
		}
		clients = append(clients, c)
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("无可用用户（全部未启用或无 UUID）")
	}

	stream := map[string]any{
		"network": inb.Network,
		"security": inb.TLSType,
	}
	switch inb.Network {
	case "tcp":
		// 无额外传输设置
	case "ws":
		if settings.WS != nil {
			ws := map[string]any{"path": settings.WS.Path}
			if settings.WS.Host != "" {
				ws["headers"] = map[string]any{"Host": settings.WS.Host}
			}
			stream["wsSettings"] = ws
		}
	case "xhttp":
		if settings.XHTTP != nil {
			stream["xhttpSettings"] = map[string]any{
				"mode": settings.XHTTP.Mode,
				"path": settings.XHTTP.Path,
			}
		}
	default:
		return nil, fmt.Errorf("暂不支持传输层 %q", inb.Network)
	}

	switch inb.TLSType {
	case "reality":
		if settings.Reality == nil || settings.Reality.PrivateKey == "" {
			return nil, fmt.Errorf("reality 配置缺失 private_key/dest/server_name")
		}
		stream["realitySettings"] = map[string]any{
			"show":        false,
			"dest":        settings.Reality.Dest,
			"serverNames": []string{settings.Reality.ServerName},
			"privateKey":  settings.Reality.PrivateKey,
			"shortIds":    []string{settings.Reality.ShortID},
		}
	case "tls":
		if settings.TLS == nil || settings.TLS.CertFile == "" {
			return nil, fmt.Errorf("tls 配置缺失 cert_file/key_file")
		}
		stream["tlsSettings"] = map[string]any{
			"serverName": settings.TLS.ServerName,
			"certificates": []any{map[string]any{
				"certificateFile": settings.TLS.CertFile,
				"keyFile":         settings.TLS.KeyFile,
			}},
		}
	case "none", "":
		stream["security"] = "none"
	default:
		return nil, fmt.Errorf("暂不支持 TLS 类型 %q", inb.TLSType)
	}

	return map[string]any{
		"tag":            inb.Tag,
		"listen":         "0.0.0.0",
		"port":           inb.Port,
		"protocol":       "vless",
		"settings":       map[string]any{"clients": clients, "decryption": "none"},
		"streamSettings": stream,
	}, nil
}