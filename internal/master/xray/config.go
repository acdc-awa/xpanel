// Package xray 负责从「服务器 + 入站 + 用户」生成 Xray 配置。
// 支持协议：VLESS（tcp+reality+vision / tcp+tls+fallbacks / ws+tls / xhttp+reality / grpc+tls / grpc+reality）；其他协议后续扩展。
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
	GRPC    *GRPCSettings    `json:"grpc,omitempty"`
	TLS     *TLSSettings     `json:"tls,omitempty"`
	// Fallbacks 仅应用于 tcp 传输（VLESS settings.fallbacks 线格式）。
	Fallbacks []FallbackSettings `json:"fallbacks,omitempty"`
	// Sniffing 入站流量嗅探（存储层 snake_case；前端表单以 camelCase 发送，UnmarshalJSON 双写兼容）。
	Sniffing *SniffingSettings `json:"sniffing,omitempty"`
}

// FallbackSettings 对应 xray-core VLESS settings.fallbacks 条目（infra/conf/vless.go 的 VLessInboundFallback）。
type FallbackSettings struct {
	Dest string `json:"dest"`
	Path string `json:"path,omitempty"`
	Xver int    `json:"xver,omitempty"`
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

type GRPCSettings struct {
	ServiceName string `json:"serviceName,omitempty"`
	Authority   string `json:"authority,omitempty"`
	MultiMode   bool   `json:"multiMode,omitempty"`
}

// MarshalJSON 以 snake_case 输出（service_name/multi_mode/authority），与 InboundSettings 其余字段
// 及 TEST_INFRA §4.1 载荷一致，保证 marshalSettings 落库后前端表单（只认 snake_case）编辑不丢字段。
// xray 线格式不受影响：buildInbound 直接构造 grpcSettings map（camelCase，xray 要求）。
func (g GRPCSettings) MarshalJSON() ([]byte, error) {
	wire := struct {
		ServiceName string `json:"service_name,omitempty"`
		Authority   string `json:"authority,omitempty"`
		MultiMode   bool   `json:"multi_mode,omitempty"`
	}{
		ServiceName: g.ServiceName,
		Authority:   g.Authority,
		MultiMode:   g.MultiMode,
	}
	return json.Marshal(wire)
}

// UnmarshalJSON 支持 camelCase (serviceName/multiMode) 与 snake_case (service_name/multi_mode) 双写；
// authority 两种写法相同。两套键同时出现时显式 camelCase 优先。
func (g *GRPCSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		ServiceName    string `json:"serviceName"`
		Authority      string `json:"authority"`
		MultiMode      *bool  `json:"multiMode"`
		ServiceNameAlt string `json:"service_name"`
		MultiModeAlt   *bool  `json:"multi_mode"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.ServiceName = raw.ServiceName
	if g.ServiceName == "" {
		g.ServiceName = raw.ServiceNameAlt
	}
	g.Authority = raw.Authority
	g.MultiMode = false
	if raw.MultiMode != nil {
		g.MultiMode = *raw.MultiMode
	} else if raw.MultiModeAlt != nil {
		g.MultiMode = *raw.MultiModeAlt
	}
	return nil
}

// SniffingSettings 入站流量嗅探（存储层 snake_case；前端表单以 camelCase 发送，UnmarshalJSON 双写兼容）。
type SniffingSettings struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"dest_override,omitempty"`
	MetadataOnly bool     `json:"metadata_only,omitempty"`
	RouteOnly    bool     `json:"route_only,omitempty"`
}

// UnmarshalJSON 支持 camelCase (destOverride/metadataOnly/routeOnly) 与 snake_case (dest_override/
// metadata_only/route_only) 双写；enabled 两写法相同。前端表单按 camelCase 发送，存储按 snake_case。
// 两套键同时出现时显式 camelCase 优先（与 GRPCSettings 一致）。
func (s *SniffingSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		Enabled         bool     `json:"enabled"`
		DestOverride    []string `json:"destOverride"`
		DestOverrideAlt []string `json:"dest_override"`
		MetadataOnly    *bool    `json:"metadataOnly"`
		MetadataOnlyAlt *bool    `json:"metadata_only"`
		RouteOnly       *bool    `json:"routeOnly"`
		RouteOnlyAlt    *bool    `json:"route_only"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Enabled = raw.Enabled
	s.DestOverride = raw.DestOverride
	if len(s.DestOverride) == 0 {
		s.DestOverride = raw.DestOverrideAlt
	}
	s.MetadataOnly = false
	if raw.MetadataOnly != nil {
		s.MetadataOnly = *raw.MetadataOnly
	} else if raw.MetadataOnlyAlt != nil {
		s.MetadataOnly = *raw.MetadataOnlyAlt
	}
	s.RouteOnly = false
	if raw.RouteOnly != nil {
		s.RouteOnly = *raw.RouteOnly
	} else if raw.RouteOnlyAlt != nil {
		s.RouteOnly = *raw.RouteOnlyAlt
	}
	return nil
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

// ValidateInbound 校验入站 JSON 基本有效性（透传模式：仅检查 JSON 可解析）。
func ValidateInbound(settingsJSON, streamSettings, sniffing string) error {
	if settingsJSON != "" {
		var v any
		if err := json.Unmarshal([]byte(settingsJSON), &v); err != nil {
			return fmt.Errorf("settings JSON 无效: %w", err)
		}
	}
	if streamSettings != "" {
		var v any
		if err := json.Unmarshal([]byte(streamSettings), &v); err != nil {
			return fmt.Errorf("streamSettings JSON 无效: %w", err)
		}
	}
	if sniffing != "" {
		var v any
		if err := json.Unmarshal([]byte(sniffing), &v); err != nil {
			return fmt.Errorf("sniffing JSON 无效: %w", err)
		}
	}
	return nil
}

// ValidateSettings 保留旧签名（向后兼容，委托给 ValidateInbound）。
func ValidateSettings(s *InboundSettings, network, tlsType string) error {
	return nil
}

// checkGRPCServiceName 校验 grpc 传输必填字段 serviceName：去空白后非空，且不含控制字符。
func checkGRPCServiceName(g *GRPCSettings) error {
	if g == nil || strings.TrimSpace(g.ServiceName) == "" {
		return errors.New("grpc 传输需要配置 serviceName")
	}
	if strings.ContainsFunc(g.ServiceName, isControlChar) {
		return errors.New("grpc serviceName 不能包含控制字符")
	}
	return nil
}

func isControlChar(r rune) bool {
	return r < 0x20 || r == 0x7f
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
		if rule.InboundTag != "" {
			rmap["inboundTag"] = parseStringList(rule.InboundTag)
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

// buildInbound 透传模式：解析 SettingsJSON / StreamSettings / Sniffing 原文，
// 仅动态注入 clients 列表（从 users 表生成）。其余字段完全透传。
func buildInbound(inb *models.Inbound, users []models.User) (map[string]any, error) {
	// 1. 解析协议 settings JSON → 注入 clients
	settings := map[string]any{}
	if inb.SettingsJSON != "" {
		if err := json.Unmarshal([]byte(inb.SettingsJSON), &settings); err != nil {
			return nil, fmt.Errorf("入站 %s settings 解析失败: %w", inb.Tag, err)
		}
	}
	// VLESS: xray 拒绝入站级别的 encryption 字段（运行时固定为 "none"）
	if inb.Protocol == "vless" {
		delete(settings, "encryption")
	}

	clients := buildClients(inb, users)
	if len(clients) == 0 {
		return nil, fmt.Errorf("无可用用户（全部未启用或无 UUID）")
	}
	settings["clients"] = clients

	// 2. 解析 streamSettings JSON（完全透传）
	item := map[string]any{
		"tag":      inb.Tag,
		"listen":   inb.Listen,
		"port":     inb.Port,
		"protocol": inb.Protocol,
		"settings": settings,
	}
	if item["listen"] == "" || item["listen"] == nil {
		item["listen"] = "0.0.0.0"
	}

	if inb.StreamSettings != "" {
		var stream map[string]any
		if err := json.Unmarshal([]byte(inb.StreamSettings), &stream); err != nil {
			return nil, fmt.Errorf("入站 %s streamSettings 解析失败: %w", inb.Tag, err)
		}
		item["streamSettings"] = stream
	}

	// 3. sniffing（透传）
	if inb.Sniffing != "" {
		var sniff map[string]any
		if err := json.Unmarshal([]byte(inb.Sniffing), &sniff); err == nil {
			item["sniffing"] = sniff
		}
	}

	return item, nil
}

// buildClients 从用户列表构建 Xray clients JSON。
func buildClients(inb *models.Inbound, users []models.User) []any {
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
		// TCP+REALITY 自动加 vision 流控
		if StreamHasReality(inb.StreamSettings) {
			c["flow"] = "xtls-rprx-vision"
		}
		clients = append(clients, c)
	}
	return clients
}

// StreamHasReality 判断 streamSettings JSON 是否启用了 REALITY。
func StreamHasReality(raw string) bool {
	return StreamSecurity(raw) == "reality"
}

// StreamNetwork 从 streamSettings JSON 中提取 network 字段。
func StreamNetwork(raw string) string {
	if raw == "" {
		return ""
	}
	var s struct {
		Network string `json:"network"`
	}
	json.Unmarshal([]byte(raw), &s)
	return s.Network
}

// StreamSecurity 从 streamSettings JSON 中提取 security 字段。
func StreamSecurity(raw string) string {
	if raw == "" {
		return ""
	}
	var s struct {
		Security string `json:"security"`
	}
	json.Unmarshal([]byte(raw), &s)
	return s.Security
}

// StreamReality 从 streamSettings JSON 中提取 realitySettings。
func StreamReality(raw string) *RealitySettings {
	if raw == "" {
		return nil
	}
	var s struct {
		RealitySettings RealitySettings `json:"realitySettings"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil
	}
	if s.RealitySettings.ServerName == "" {
		return nil
	}
	return &s.RealitySettings
}

// StreamWS 从 streamSettings JSON 中提取 wsSettings。
func StreamWS(raw string) *WSSettings {
	if raw == "" {
		return nil
	}
	var s struct {
		WSSettings WSSettings `json:"wsSettings"`
	}
	json.Unmarshal([]byte(raw), &s)
	if s.WSSettings.Path == "" {
		return nil
	}
	return &s.WSSettings
}

// StreamXHTTP 从 streamSettings JSON 中提取 xhttpSettings。
func StreamXHTTP(raw string) *XHTTPSettings {
	if raw == "" {
		return nil
	}
	var s struct {
		XHTTPSettings XHTTPSettings `json:"xhttpSettings"`
	}
	json.Unmarshal([]byte(raw), &s)
	if s.XHTTPSettings.Mode == "" {
		return nil
	}
	return &s.XHTTPSettings
}
