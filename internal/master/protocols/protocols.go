// Package protocols 协议插件注册表：同一协议的服务端注入与客户端映射内聚于一个插件。
// 新增协议 = 新增一个实现 contracts.ProtocolPlugin 的文件并在 init 注册，
// 生成（xray driver）、订阅（subscribe）、取数（services）三侧自动获得该协议能力。
package protocols

import (
	"fmt"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
)

var registry = contracts.NewPluginRegistry()

func init() {
	registry.Register(VLESSPlugin{})
	registry.Register(HysteriaPlugin{})
}

// Find 按协议名查找插件；未注册返回 nil。
func Find(proto string) contracts.ProtocolPlugin {
	return registry.Find(proto)
}

// AllDescriptors 获取所有已注册协议的描述符。
func AllDescriptors() []contracts.ProtocolDescriptor {
	return registry.AllDescriptors()
}

// RegisteredProtocols 获取所有已注册的协议名列表。
func RegisteredProtocols() []string {
	return registry.RegisteredProtocols()
}

// ServerUsers 按协议生成服务端入站用户数组及挂载键名。
// 未注册协议显式报错拒绝（Fail-Closed 铁律，坚决杜绝静默注入假 UUID 或丢节点）。
func ServerUsers(proto string, users []protocol.User, spec *contracts.InboundSpec) (string, []any, error) {
	p := Find(proto)
	if p == nil {
		return "", nil, fmt.Errorf("协议 %q 未注册协议插件", proto)
	}
	return p.ServerUsers(users, spec)
}

// ServerClients 兼容保留：按协议生成服务端入站 settings.clients。
func ServerClients(proto string, users []protocol.User, spec *contracts.InboundSpec) []any {
	_, items, _ := ServerUsers(proto, users, spec)
	return items
}

// ResolveFlow 按协议决议用户 flow（生成侧盖戳与订阅侧同源）；
// 未注册协议返回空（flow 是 VLESS 语义，不应泄漏到其他协议）。
func ResolveFlow(proto string, spec *contracts.InboundSpec, inboundFlow string) string {
	if p := Find(proto); p != nil {
		return p.ResolveFlow(spec, inboundFlow)
	}
	return ""
}
