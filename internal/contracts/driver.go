package contracts

import (
	"context"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

// RefTarget 出站 InboundRef 引用的目标入站（可跨服务器）。
type RefTarget struct {
	Inbound    models.Inbound
	ServerHost string // 目标服务器对外 Host（vnext address 来源）
	// CertPin 目标入站绑定证书的 leaf SHA-256（hex，取自已入库的 Cert.PinSHA256）；
	// 非空时 TLS 中转出站注入 pinnedPeerCertSha256（自签证书防 MITM；空 = 走系统 CA 验证）。
	CertPin string
}

// TopologyContext 拓扑化上下文（Phase T）：跨服务器引用与证书映射，由 provision 领域从 DB 组装。
// 传 nil 时生成行为退化为无 InboundRef / CertID 注入。
type TopologyContext struct {
	RefTargets  map[uint64]RefTarget // 出站 InboundRef → 目标入站
	CertDomains map[uint64]string    // CertID → 证书域名（路径注入 /etc/xray/certs/<domain>/）
}

// GenerateInput 核心生成输入（中立结构）：由 provision 领域取数装配，
// driver 只消费输入、不感知 DB。字段与 xray.Generate 参数一一对应。
type GenerateInput struct {
	Inbounds               []models.Inbound
	Outbounds              []models.ServerOutbound
	RoutingRules           []models.ServerRoutingRule
	UsersByTag             map[string][]protocol.User // 入站 tag → 已过滤用户（GetValidUsers 同源）
	Topology               *TopologyContext           // nil = 无跨服务器引用/证书注入
	DefaultOutboundTag     string                     // 空 = outbounds 第一个
	RoutingDomainStrategy  string                     // 空 = 模板默认
	DefaultOutboundDS      string                     // 默认出口出站解析策略（空/AsIs = 不注入）
}

// CoreDriver 代理核心配置驱动接口（xray / 未来 sing-box）。
type CoreDriver interface {
	Name() string // "xray", "sing-box"
	// Generate 由中立输入生成核心配置原文。
	Generate(ctx context.Context, in *GenerateInput) ([]byte, error)
	// ValidateConfig 校验配置可被核心加载（如 xray -test）。
	// 实现可在缺少校验手段（无二进制）时返回 nil 跳过。
	ValidateConfig(ctx context.Context, rawConfig []byte) error
}

// DriverRegistry 核心驱动注册表：按 Name 注册/查找，首个注册为默认。
type DriverRegistry struct {
	drivers []CoreDriver
}

// NewDriverRegistry 创建空注册表。
func NewDriverRegistry() *DriverRegistry {
	return &DriverRegistry{}
}

// Register 注册驱动；后注册的同名驱动覆盖之前的同名项。
func (r *DriverRegistry) Register(d CoreDriver) {
	if d == nil {
		return
	}
	for i, old := range r.drivers {
		if old.Name() == d.Name() {
			r.drivers[i] = d
			return
		}
	}
	r.drivers = append(r.drivers, d)
}

// Find 按 Name 查找驱动；未找到返回 nil。
func (r *DriverRegistry) Find(name string) CoreDriver {
	for _, d := range r.drivers {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

// Default 返回首个注册的驱动；空注册表返回 nil。
func (r *DriverRegistry) Default() CoreDriver {
	if len(r.drivers) == 0 {
		return nil
	}
	return r.drivers[0]
}
