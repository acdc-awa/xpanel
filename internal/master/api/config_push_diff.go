package api

import (
	"encoding/json"
	"reflect"

	"github.com/acdc-awa/xpanel/internal/models"
)

// xrayConfigChanged 判断本次入站更新是否**真的**改动了影响 Xray 运行期配置的字段。
//
// 只看「请求里出现了哪些键」会误判：入站编辑器每次保存都整表回传，
// 只改倍率 / 额度 / 备注时也会带着 port、settings_json 等键进来。按出现即推送的旧逻辑，
// 一次纯计费编辑就是一次冷更——节点侧 RestartWithConfig 没有内容短路，每次都 Stop+Start。
//
// old 必须是**写入前**的库内快照（updates 里的值来自请求/归一化后的终态）。
// 纯计费与门面字段（ratio / total / expiry_time / remark / traffic_reset / share_*）不在判断内。
func xrayConfigChanged(old models.Inbound, updates map[string]any) bool {
	str := func(v any) (string, bool) {
		s, ok := v.(string)
		return s, ok
	}
	num := func(v any) (int, bool) {
		n, ok := v.(int)
		return n, ok
	}
	for key, v := range updates {
		switch key {
		case "tag":
			if s, ok := str(v); !ok || s != old.Tag {
				return true
			}
		case "protocol":
			if s, ok := str(v); !ok || s != old.Protocol {
				return true
			}
		case "port":
			if n, ok := num(v); !ok || n != old.Port {
				return true
			}
		case "listen":
			if s, ok := str(v); !ok || s != old.Listen {
				return true
			}
		case "settings_json":
			s, ok := str(v)
			if !ok || !jsonEqual(old.SettingsJSON, s) {
				return true
			}
		case "stream_settings":
			s, ok := str(v)
			if !ok || !jsonEqual(old.StreamSettings, s) {
				return true
			}
		case "sniffing":
			s, ok := str(v)
			if !ok || !jsonEqual(old.Sniffing, s) {
				return true
			}
		case "enabled":
			if b, ok := v.(bool); !ok || b != old.Enabled {
				return true
			}
		case "type":
			if s, ok := str(v); !ok || s != old.Type {
				return true
			}
		case "target_inbound_id":
			if !sameOptionalID(old.TargetInboundID, v) {
				return true
			}
		case "target_address":
			if s, ok := str(v); !ok || s != old.TargetAddress {
				return true
			}
		case "target_port":
			if n, ok := num(v); !ok || n != old.TargetPort {
				return true
			}
		case "cert_id":
			if !sameOptionalID(old.CertID, v) {
				return true
			}
		case "flow":
			if s, ok := str(v); !ok || s != old.Flow {
				return true
			}
		case "layer_id":
			if !sameOptionalID(old.LayerID, v) {
				return true
			}
		}
	}
	return false
}

// sameOptionalID 比较「可空 ID 列」：updates 里存的是解引用后的 uint64，或解绑时写入的 nil。
func sameOptionalID(old *uint64, v any) bool {
	if v == nil {
		return old == nil
	}
	n, ok := v.(uint64)
	if !ok {
		return false
	}
	return old != nil && *old == n
}

// outboundConfigChanged 判断出站更新是否真的改动了影响 Xray 运行期配置的字段。
// 出站编辑器同样整表回传，只改备注不该触发一次 Stop+Start；备注也不参与生成。
func outboundConfigChanged(old models.ServerOutbound, updates map[string]any) bool {
	for key, v := range updates {
		switch key {
		case "tag":
			if s, ok := v.(string); !ok || s != old.Tag {
				return true
			}
		case "protocol":
			if s, ok := v.(string); !ok || s != old.Protocol {
				return true
			}
		case "settings_json":
			s, ok := v.(string)
			if !ok || !jsonEqual(old.SettingsJSON, s) {
				return true
			}
		case "stream_settings_json":
			s, ok := v.(string)
			if !ok || !jsonEqual(old.StreamSettingsJSON, s) {
				return true
			}
		case "outbound_json":
			s, ok := v.(string)
			if !ok || !jsonEqual(old.OutboundJSON, s) {
				return true
			}
		case "send_through":
			if s, ok := v.(string); !ok || s != old.SendThrough {
				return true
			}
		case "enabled":
			if b, ok := v.(bool); !ok || b != old.Enabled {
				return true
			}
		case "priority":
			if n, ok := v.(int); !ok || n != old.Priority {
				return true
			}
		case "send_proxy_protocol":
			if b, ok := v.(bool); !ok || b != old.SendProxyProtocol {
				return true
			}
		case "inbound_ref":
			if !sameOptionalID(old.InboundRef, v) {
				return true
			}
		case "internal_uuid":
			if s, ok := v.(string); !ok || s != old.InternalUUID {
				return true
			}
		}
	}
	return false
}

// routingRuleConfigChanged 判断路由规则更新是否真的改动了影响生成的字段。
// 备注不参与生成，只改备注不该重推。
func routingRuleConfigChanged(old models.ServerRoutingRule, updates map[string]any) bool {
	for key, v := range updates {
		switch key {
		case "rule_json":
			s, ok := v.(string)
			if !ok || !jsonEqual(old.RuleJSON, s) {
				return true
			}
		case "enabled":
			if b, ok := v.(bool); !ok || b != old.Enabled {
				return true
			}
		case "priority":
			if n, ok := v.(int); !ok || n != old.Priority {
				return true
			}
		case "outbound_tag":
			if s, ok := v.(string); !ok || s != old.OutboundTag {
				return true
			}
		case "domain":
			if s, ok := v.(string); !ok || s != old.Domain {
				return true
			}
		case "ip":
			if s, ok := v.(string); !ok || s != old.IP {
				return true
			}
		case "port":
			if s, ok := v.(string); !ok || s != old.Port {
				return true
			}
		case "network":
			if s, ok := v.(string); !ok || s != old.Network {
				return true
			}
		case "protocol":
			if s, ok := v.(string); !ok || s != old.Protocol {
				return true
			}
		case "inbound_tag":
			if s, ok := v.(string); !ok || s != old.InboundTag {
				return true
			}
		}
	}
	return false
}

// jsonEqual 语义比较两段 JSON：键序与空白差异不算变化。
// 编辑器用 JSON.stringify 重新序列化表单值，与库内原始文本常常只是排版不同；
// 文本比较会让每次保存都被判成改过。任一侧无法解析时按「有变化」处理（保守）。
func jsonEqual(a, b string) bool {
	if a == b {
		return true
	}
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
