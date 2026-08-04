// Package xray 负责从「服务器 + 入站 + 用户」生成 Xray 配置。
// 支持协议：VLESS（tcp+reality+vision / ws+tls / xhttp+reality）；其他协议后续扩展。
package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// ValidateSettings 校验入站连接参数与传输层/TLS 的匹配关系。
func ValidateSettings(s *InboundSettings, network, tlsType string) error {
	if s == nil {
		s = &InboundSettings{}
	}
	switch network {
	case "tcp":
	case "ws":
		if s.WS == nil || s.WS.Path == "" {
			return errors.New("ws 传输需要配置 path")
		}
	case "xhttp":
		if s.XHTTP == nil || s.XHTTP.Mode == "" || s.XHTTP.Path == "" {
			return errors.New("xhttp 传输需要配置 mode 和 path")
		}
	default:
		return fmt.Errorf("暂不支持传输层 %q", network)
	}
	switch tlsType {
	case "none", "":
	case "reality":
		if s.Reality == nil || s.Reality.ServerName == "" || s.Reality.PublicKey == "" ||
			s.Reality.PrivateKey == "" || s.Reality.Dest == "" {
			return errors.New("reality 需要配置 server_name / public_key / private_key / dest")
		}
	case "tls":
		if s.TLS == nil || s.TLS.CertFile == "" || s.TLS.KeyFile == "" {
			return errors.New("tls 需要配置 cert_file 和 key_file")
		}
	default:
		return fmt.Errorf("暂不支持 TLS 类型 %q", tlsType)
	}
	return nil
}

// parseStringList 解析 JSON 数组字符串或逗号/换行/分号分隔字符串。
func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		var arr []string
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return arr
		}
	}
	lines := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	var res []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			res = append(res, l)
		}
	}
	return res
}

// Generate 生成完整 Xray 配置（含 api/policy/stats 三段 + 全部启用入站 + 节点自定义出站/路由规则 + 全部启用用户）。
func Generate(server *models.Server, inbounds []models.Inbound, outbounds []models.ServerOutbound, routingRules []models.ServerRoutingRule, users []models.User) ([]byte, error) {
	outboundList := make([]any, 0)
	hasFreedom := false
	hasBlackhole := false

	for _, ob := range outbounds {
		if !ob.Enabled {
			continue
		}
		item := make(map[string]any)

		if ob.SettingsJSON != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(ob.SettingsJSON), &parsed); err == nil {
				for k, v := range parsed {
					item[k] = v
				}
				_, hasProto := parsed["protocol"]
				_, hasSettings := parsed["settings"]
				if !hasProto && !hasSettings {
					item["settings"] = parsed
				}
			}
		}

		if ob.Tag != "" {
			item["tag"] = ob.Tag
		}
		if ob.Protocol != "" {
			item["protocol"] = ob.Protocol
		}
		if ob.StreamSettingsJSON != "" {
			var stream map[string]any
			if err := json.Unmarshal([]byte(ob.StreamSettingsJSON), &stream); err == nil {
				item["streamSettings"] = stream
			}
		}
		if ob.SendThrough != "" {
			item["sendThrough"] = ob.SendThrough
		}

		tagVal, _ := item["tag"].(string)
		protoVal, _ := item["protocol"].(string)
		if tagVal == "direct" || tagVal == "freedom" || protoVal == "freedom" {
			hasFreedom = true
		}
		if tagVal == "blocked" || tagVal == "blackhole" || protoVal == "blackhole" {
			hasBlackhole = true
		}

		outboundList = append(outboundList, item)
	}

	// 兜底 freedom (direct) 出站
	if !hasFreedom {
		outboundList = append(outboundList, map[string]any{
			"protocol": "freedom",
			"tag":      "direct",
			"settings": map[string]any{
				"finalRules": []any{
					map[string]any{"action": "allow", "ip": []string{"198.18.0.0/15"}},
				},
			},
		})
	}

	// 兜底 blackhole (blocked) 出站
	if !hasBlackhole {
		outboundList = append(outboundList, map[string]any{
			"protocol": "blackhole",
			"tag":      "blocked",
		})
	}

	// 构建 routing rules 列表
	rulesList := []any{
		map[string]any{
			"type":        "field",
			"inboundTag":  []string{"api"},
			"outboundTag": "api",
		},
	}

	for _, rule := range routingRules {
		if !rule.Enabled {
			continue
		}

		if rule.RuleJSON != "" {
			var rmap map[string]any
			if err := json.Unmarshal([]byte(rule.RuleJSON), &rmap); err == nil {
				if rmap["type"] == nil {
					rmap["type"] = "field"
				}
				if rule.OutboundTag != "" && rmap["outboundTag"] == nil {
					rmap["outboundTag"] = rule.OutboundTag
				}
				rulesList = append(rulesList, rmap)
				continue
			}
		}

		rmap := map[string]any{
			"type":        "field",
			"outboundTag": rule.OutboundTag,
		}

		if domains := parseStringList(rule.Domain); len(domains) > 0 {
			rmap["domain"] = domains
		}
		if ips := parseStringList(rule.IP); len(ips) > 0 {
			rmap["ip"] = ips
		}
		if rule.Port != "" {
			rmap["port"] = rule.Port
		}
		if rule.Network != "" {
			rmap["network"] = rule.Network
		}

		rulesList = append(rulesList, rmap)
	}

	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService", "RoutingService"},
		},
		"outbounds": outboundList,
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
			"rules": rulesList,
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