// Package protocols 协议插件注册表：同一协议的服务端注入与客户端映射内聚于一个插件。
// 新增协议 = 新增一个实现 contracts.ProtocolPlugin 的文件并在 init 注册，
// 生成（xray driver）、订阅（subscribe）、取数（services）三侧自动获得该协议能力。
package protocols

import (
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

var registry = contracts.NewPluginRegistry()

func init() {
	registry.Register(VLESSPlugin{})
}

// Find 按协议名查找插件；未注册返回 nil。
func Find(proto string) contracts.ProtocolPlugin {
	return registry.Find(proto)
}

// ServerClients 按协议生成服务端入站 settings.clients。
// 未注册协议回退最小通用注入（id/email/limit，不含协议专有字段如 flow）。
func ServerClients(proto string, users []protocol.User, spec *contracts.InboundSpec) []any {
	if p := Find(proto); p != nil {
		return p.ServerClients(users, spec)
	}
	clients := make([]any, 0, len(users))
	for _, u := range users {
		if u.UUID == "" {
			continue
		}
		c := map[string]any{
			"id":    u.UUID,
			"email": u.Email,
		}
		if u.Limit > 0 {
			c["limit"] = u.Limit
		}
		clients = append(clients, c)
	}
	return clients
}

// ResolveFlow 按协议决议用户 flow（生成侧盖戳与订阅侧同源）；
// 未注册协议返回空（flow 是 VLESS 语义，不应泄漏到其他协议）。
func ResolveFlow(proto string, spec *contracts.InboundSpec, inboundFlow string) string {
	if p := Find(proto); p != nil {
		return p.ResolveFlow(spec, inboundFlow)
	}
	return ""
}
