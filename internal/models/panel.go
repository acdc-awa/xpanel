package models

import "time"

// ServerTypeXray 托管 Xray-core 计算节点（默认服务器类型；l4_relay 已于 2026-08-24 退役）。
const ServerTypeXray = "xray"

// Server 节点服务器（对应 §5 servers）。
type Server struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	ServerType string `gorm:"size:32;default:xray" json:"server_type"` // xray（托管节点；l4_relay 已于 2026-08-24 退役）
	Name       string `gorm:"size:64;not null" json:"name"`
	Host       string `gorm:"size:255;not null" json:"host"`
	NodeID     string `gorm:"size:32;uniqueIndex;not null" json:"node_id"`
	Secret     string `gorm:"size:64;not null" json:"-"`
	Location   string `gorm:"size:64" json:"location"`
	Remark     string `gorm:"size:255" json:"remark"`
	// VPS 到期日（纯日历日 YYYY-MM-DD，空=未设置）。续费日是"哪一天"而不是某一瞬，
	// 故全程不存时刻、不做时区换算——一旦存成时刻，同一日期在不同展示时区会漂成前后一天。
	ExpireAt              string `gorm:"size:10" json:"expire_at"`
	BillingCycle          string `gorm:"size:32" json:"billing_cycle"`                        // 计费周期（月付/季付/年付/一次性 等或自定义）
	Price                 string `gorm:"size:64" json:"price"`                                // 价格 / 续费金额（如 $5.99 / ¥35 / 120/年）
	IDCAddress            string `gorm:"column:idc_address;size:255" json:"idc_address"`      // IDC 地址 / 服务商控制台链接
	Status                int    `gorm:"default:0;index" json:"status"`                       // 0 离线 1 在线
	DefaultOutboundTag    string `gorm:"size:64;default:direct" json:"default_outbound_tag"`  // 默认出口（路由未命中时的出站标签）
	RoutingDomainStrategy string `gorm:"size:32;default:AsIs" json:"routing_domain_strategy"` // 路由域名策略 AsIs/IPIfNonMatch/IPOnDemand
	// 出站域名解析策略（freedom settings.domainStrategy）不在此建模：它是出站级属性，唯一入口是
	// 出站编辑器（ServerOutbound.SettingsJSON.domainStrategy）。历史上的服务器级
	// default_outbound_domain_strategy 列已移除，存量值由 migrateDefaultOutboundDSIntoOutbounds 并入出站。
	AgentVersion string `gorm:"size:32" json:"agent_version"`      // 节点心跳上报的 agent 版本（旧 agent 为空）
	XrayRunning  bool   `gorm:"default:false" json:"xray_running"` // 节点心跳上报的 xray 进程运行状态（旧 agent 不上报，保持上次值）
	// xray 启动失败可观测性（2026-09-21）：节点把"为什么没起来"带回主控，面板直接显示，
	// 不必再 SSH 翻 journalctl。旧 agent 不发这些字段（空值保持，不覆盖）。
	XrayState     string     `gorm:"size:16" json:"xray_state"`       // running / restarting / failed / stopped
	XrayLastError string     `gorm:"size:512" json:"xray_last_error"` // 最近一次启动失败原因（含退出码与 xray 原始报错）
	XrayErrorAt   *time.Time `json:"xray_error_at"`                   // 该原因的观测时刻
	XrayFailures  int        `gorm:"default:0" json:"xray_failures"`  // 连续启动失败次数（成功后归零）
	// 配置对账（2026-09-21）：节点上报的两个内容哈希，主控据此发现"节点跑的不是我以为的配置"
	// 或"磁盘被改过"（xray 只在启动时读一次配置，热更落盘只改磁盘、不改运行中内容）。
	// 旧 agent 不上报（空值保持，不覆盖）。
	XrayDiskHash    string `gorm:"size:64" json:"xray_disk_hash"`
	XrayRunningHash string `gorm:"size:64" json:"xray_running_hash"`
	// 当前在线用户 IP 快照：agent 心跳每次覆写的 JSON（[]{email,ips}，源自 xray GetUsersStats，
	// refcount 语义=当前活跃连接的去重源 IP）。不直接 JSON 透出，经 GET /admin/servers/:id/online-ips
	// 解析归类后返回。
	OnlineIPs  string     `gorm:"type:text" json:"-"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Inbound 入站（接入点），每节点可配多个。
// settings_json 存协议层 JSON（clients 动态注入 / fallbacks / decryption）；
// stream_settings 存传输层 JSON（network / security / realitySettings / wsSettings / tlsSettings 等）；
// sniffing 存流量嗅探 JSON。
type Inbound struct {
	ID             uint64  `gorm:"primaryKey" json:"id"`
	ServerID       uint64  `gorm:"index;not null" json:"server_id"`
	Tag            string  `gorm:"size:64;not null" json:"tag"`
	Protocol       string  `gorm:"size:16;not null" json:"protocol"` // vless / vmess / trojan / shadowsocks
	Port           int     `gorm:"not null" json:"port"`
	Listen         string  `gorm:"size:64" json:"listen"`            // 监听地址，空 = 0.0.0.0
	SettingsJSON   string  `gorm:"type:text" json:"settings_json"`   // 协议 settings（透传，clients 由后端注入）
	StreamSettings string  `gorm:"type:text" json:"stream_settings"` // 传输 streamSettings（透传）
	Sniffing       string  `gorm:"type:text" json:"sniffing"`        // 嗅探配置（透传）
	Ratio          float64 `gorm:"default:1" json:"ratio"`
	// 流量统计（冗余计数器，避免每次 SUM traffic_logs）
	Up            int64      `gorm:"default:0" json:"up"`
	Down          int64      `gorm:"default:0" json:"down"`
	Total         int64      `gorm:"default:0" json:"total"`                     // 入站总流量上限（0=不限）
	TrafficReset  string     `gorm:"size:16;default:never" json:"traffic_reset"` // never / daily / weekly / monthly
	LastResetDate string     `gorm:"size:10" json:"-"`                           // ISSUE-05：上次清零周期键（YYYY-MM-DD / YYYY-MM），防止同周期重复清零
	ExpiryTime    *time.Time `json:"expiry_time,omitempty"`                      // 入站自身到期时间
	// 分享地址（订阅专用，与节点监听解耦：四层转发场景监听为内网，订阅给用户的是转发端点）
	ShareAddrStrategy string `gorm:"size:16;default:node" json:"share_addr_strategy"` // node（服务器 Host）/ custom（自定义地址）
	ShareAddr         string `gorm:"size:255" json:"share_addr"`                      // 自定义分享地址（域名/IP，不带端口）
	SharePort         int    `gorm:"default:0" json:"share_port"`                     // 自定义分享端口（0 = 使用入站端口）
	// 外部反代与订阅覆写字段（与本地物理监听解耦，支持 Caddy/CDN TLS 卸载模式）
	ShareSecurity      string  `gorm:"size:16;default:auto" json:"share_security"` // auto（跟随stream_settings）/ tls / none
	ShareSNI           string  `gorm:"size:255" json:"share_sni"`                  // 订阅 SNI 覆写（如反代域名）
	ShareHost          string  `gorm:"size:255" json:"share_host"`                 // 订阅 HTTP/WS Host 覆写
	SharePath          string  `gorm:"size:255" json:"share_path"`                 // 订阅 WS/XHTTP Path 覆写
	ShareAllowInsecure bool    `gorm:"default:false" json:"share_allow_insecure"`  // 订阅是否跳过证书检查
	LayerID            *uint64 `gorm:"index" json:"layer_id,omitempty"`            // 所属对外接入层（空/0 = 直连自持端点；见 AccessLayer）
	// 入站物理类型形态：user / relay / tunnel
	Type            string  `gorm:"size:16;default:user" json:"type"`                 // user（用户入站）/ relay（内部链式转发入站）/ tunnel（四层直通管道）
	PreviousType    string  `gorm:"size:16" json:"-"`                                 // 被自动标 relay 前的类型（解绑引用后回退；空 = 保持不动）
	InternalUUID    string  `gorm:"size:36" json:"internal_uuid,omitempty"`           // relay 入站 UUID（节点生成上报，主控只读）
	TargetInboundID *uint64 `gorm:"index" json:"target_inbound_id,omitempty"`       // 四层直通目标落地入站 ID（空=待拓扑连线）
	TargetAddress   string  `gorm:"size:255" json:"target_address,omitempty"`        // 手动指定的外部目标地址
	TargetPort      int     `gorm:"default:0" json:"target_port,omitempty"`          // 手动指定的外部目标端口
	CertID          *uint64 `gorm:"index" json:"cert_id,omitempty"`                   // TLS 入站选择证书（certs 表）
	Enabled         bool    `gorm:"default:true" json:"enabled"`
	// 入站级流控（写进生成的 clients，VLESS settings 无顶层 flow，不入 settings_json）：
	// 空 = 自动（TCP+REALITY 自动注入 xtls-rprx-vision）；
	// xtls-rprx-vision = 为该入站用户全部开启；
	// none = 禁用自动注入。（UserInbound per-user 覆盖已随 2026-08-14 批2 冻结删除）
	Flow      string    `gorm:"size:32" json:"flow"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Plan 套餐。
type Plan struct {
	ID                uint64 `gorm:"primaryKey" json:"id"`
	Name              string `gorm:"size:64;not null" json:"name"`
	Description       string `gorm:"type:text" json:"description"` // 自定义文案/套餐特性说明
	PriceCents        int64  `gorm:"not null" json:"price_cents"`  // 价格（分）
	TrafficGB         int64  `gorm:"not null" json:"traffic_gb"`
	DurationDays      int    `gorm:"not null" json:"duration_days"`
	DeviceLimit       int    `gorm:"default:0" json:"device_limit"`              // 最大在线设备数（0=不限）
	PermissionGroupID uint64 `gorm:"index;default:0" json:"permission_group_id"` // 绑定权限组（0=不绑定）
	SortOrder         int    `gorm:"default:0" json:"sort_order"`                // 商城展示排序（越小越靠前；同值按价格升序）
	IsFeatured        bool   `gorm:"default:false" json:"is_featured"`           // 商城「热门推荐」标记（多选时仅排最前的生效）
	// 销售两属性（2026-09-03 替代 enabled，Xboard show/sell/renew 收敛式）：
	// purchasable=商店展示并允许新购（非持有者）；renewable=持有者续费顺延。
	// 商店身份感知过滤：非持有者只见 purchasable，持有者额外见自己可续费的套餐；
	// 支付接口按同一矩阵兜底（持有查 renewable，非持有查 purchasable）。
	Purchasable bool      `gorm:"default:false" json:"purchasable"`
	Renewable   bool      `gorm:"default:false" json:"renewable"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Order 订单（余额直付即时生效：paid）。人工确认收款已去除（2026-08-14 方向④）。
type Order struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	OrderNo       string     `gorm:"size:32;uniqueIndex;not null" json:"order_no"`
	UserID        uint64     `gorm:"index;not null" json:"user_id"`
	PlanID        uint64     `gorm:"index;not null" json:"plan_id"`
	AmountCents   int64      `gorm:"not null" json:"amount_cents"`
	PaymentMethod string     `gorm:"size:32;default:balance;index" json:"payment_method"` // balance（唯一）
	Status        string     `gorm:"size:16;default:paid;index" json:"status"`            // paid（唯一）
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
}

// TrafficLog 节点上报的流量明细（按 用户×入站×周期）。
// (user_id, inbound_id, period_start) 三列唯一：同一上报周期重复投递时覆盖合并（补报幂等）。
// 2026-08-14 U1 修复：原仅 period_start 单列唯一索引 → 多用户共周期上报时互相冲突丢数据。
// up/down 恒为原始字节（展示口径：dashboard/管理端/入站计数）；
// billed_* 为计费口径（套餐配额判定/自动续费触发/订阅用量展示），落库时按
// (用户生效组, 服务器) 生效入站倍率折算；历史行由一次性迁移按 1:1 回填。
type TrafficLog struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	UserID      uint64    `gorm:"uniqueIndex:idx_traffic_uid_inb_period_cycle,priority:1;not null" json:"user_id"`
	InboundID   uint64    `gorm:"uniqueIndex:idx_traffic_uid_inb_period_cycle,priority:2;index" json:"inbound_id"`
	UpBytes     int64     `gorm:"not null" json:"up_bytes"`
	DownBytes   int64     `gorm:"not null" json:"down_bytes"`
	BilledUp    int64     `gorm:"default:0" json:"billed_up"`
	BilledDown  int64     `gorm:"default:0" json:"billed_down"`
	PeriodStart time.Time `gorm:"uniqueIndex:idx_traffic_uid_inb_period_cycle,priority:3;index:idx_traffic_period_start" json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	// CycleID 账期 ID（审计 F3）：节点在采集时刻打标的本条增量所属计费周期。计费/配额按
	// 「cycle_id = 用户当前 traffic_cycle_id」归属，不再依赖上报时刻落进哪个小时桶。
	// 0 = 未知（旧 agent 未打标 / 历史行）：回退按 period_start >= traffic_cycle_start 归属。
	// 必须进唯一索引：同一小时桶内跨账期的两行不得合并，否则计费口径不可区分。
	CycleID   uint64    `gorm:"uniqueIndex:idx_traffic_uid_inb_period_cycle,priority:4;default:0" json:"cycle_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TrafficBatch 流量上报批次去重记录（审计 F2）。
//
// 为什么必须有：traffic_logs 的小时桶 upsert 是**加法**（同一小时多次上报要累加，不能改覆盖），
// 因此「同一批数据被重复投递」与「同一小时内两批合法增量」在库内不可区分——去重键必须来自
// 节点（BatchID 由 agent 生成、重发复用同一 ID）。去重记录与流量写入在同一事务，提交后才回 ACK。
//
// 保留期 trafficBatchRetentionDays（见 services）必须 ≥ agent outbox 最大保留时长，
// 否则「节点长期离线后补报」会在去重记录被清理后重复计量。
type TrafficBatch struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ServerID  uint64    `gorm:"uniqueIndex:idx_traffic_batch,priority:1;not null" json:"server_id"`
	BatchID   string    `gorm:"size:64;uniqueIndex:idx_traffic_batch,priority:2;not null" json:"batch_id"`
	Entries   int       `json:"entries"`
	UpBytes   int64     `json:"up_bytes"`
	DownBytes int64     `json:"down_bytes"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TrafficDaily 每日汇总（仪表盘用）。
type TrafficDaily struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"index;not null" json:"user_id"`
	Date      string    `gorm:"size:10;index" json:"date"` // YYYY-MM-DD
	UpBytes   int64     `gorm:"not null" json:"up_bytes"`
	DownBytes int64     `gorm:"not null" json:"down_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// NodeReport 节点心跳/状态上报。
type NodeReport struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	ServerID    uint64    `gorm:"index:idx_server_reported,priority:1;not null" json:"server_id"`
	CPU         float64   `json:"cpu"`
	Mem         float64   `json:"mem"`
	MemTotal    uint64    `json:"mem_total"`
	Disk        float64   `json:"disk"`
	DiskTotal   uint64    `json:"disk_total"`
	OnlineUsers int       `json:"online_users"`
	RxRate      float64   `json:"rx_rate"`
	TxRate      float64   `json:"tx_rate"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
	ReportedAt  time.Time `gorm:"index:idx_server_reported,priority:2;index" json:"reported_at"`
}

// AuditLog 审计日志（管理员操作、登录、订单确认等）。
type AuditLog struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	OperatorType string    `gorm:"size:16;index" json:"operator_type"` // admin / user / system
	OperatorID   uint64    `gorm:"index" json:"operator_id"`
	Action       string    `gorm:"size:64;index" json:"action"`
	Detail       string    `gorm:"type:text" json:"detail"`
	IP           string    `gorm:"size:64" json:"ip"`
	CreatedAt    time.Time `json:"created_at"`
}

// PendingConfig 服务器待推送的 Xray 配置（每服务器一条最新；节点离线时保留，上线后自动补推）。
type PendingConfig struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	ServerID   uint64 `gorm:"uniqueIndex;not null" json:"server_id"`
	ConfigJSON string `gorm:"type:text" json:"-"`
	Status     string `gorm:"size:16;default:pending" json:"status"` // pending / pushed
	// AppliedJSON 主控记录的「节点磁盘上应有的内容」：冷推成功时 = 刚推送的整份配置；
	// 热更落盘成功后 = 节点刚写下的那份（含最新用户集）。AppliedHash 是它的内容哈希，
	// 与节点上报的 disk_hash 对账即可发现磁盘偏离（第三方改动 / 落盘静默失败）。空 = 无记录。
	AppliedJSON string `gorm:"type:text" json:"-"`
	AppliedHash string `gorm:"size:64" json:"-"`
	// 推送可观测性（2026-08-31）：pending 期间最近一次失败原因/累计失败次数/最后尝试时间，
	// 面板直接展示失败原因；成功推送（MarkPushedIfSame）或重新生成（SavePending）后清零。
	LastError     string     `gorm:"size:500" json:"last_error"`
	Attempts      int        `json:"attempts"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	PushedAt      *time.Time `json:"pushed_at"`
}

// PendingCert 证书待推记录（U7：节点离线时上传的证书，上线后补推）。
type PendingCert struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ServerID  uint64    `gorm:"uniqueIndex;not null" json:"server_id"`
	CertID    uint64    `gorm:"not null" json:"cert_id"`
	Status    string    `gorm:"size:16;default:pending" json:"status"` // pending / pushed
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Setting 站点配置（公告等键值对）。
type Setting struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServerOutbound 服务器独立出站规则（§多节点 3x-ui 架构）。
type ServerOutbound struct {
	ID                 uint64 `gorm:"primaryKey" json:"id"`
	ServerID           uint64 `gorm:"index;not null" json:"server_id"`
	Tag                string `gorm:"size:64;not null" json:"tag"`
	Protocol           string `gorm:"size:32;not null" json:"protocol"` // freedom / blackhole / socks / vmess / etc.
	SettingsJSON       string `gorm:"type:text" json:"settings_json"`
	StreamSettingsJSON string `gorm:"type:text" json:"stream_settings_json,omitempty"`
	SendThrough        string `gorm:"size:64" json:"send_through,omitempty"`
	// Phase T 拓扑化：引用目标入站（落地），vnext 由生成器自动构造；空 = 沿用透传 settings_json
	InboundRef *uint64   `gorm:"index" json:"inbound_ref,omitempty"`
	Enabled    bool      `gorm:"default:true" json:"enabled"`
	Priority   int       `gorm:"default:0" json:"priority"`
	Remark     string    `gorm:"size:255" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DefaultFreedomDirectSettingsJSON freedom 出站的面板默认 settings（canonical 形状，唯一真源）。
// 三处必须一致：嵌入式模板 config.template.json 的 direct 段、EnsureDefaultServerOutbounds 的种子行、
// 以及存量迁移的回填值；xray 包有测试断言模板与本体一致，勿单独改其中一处。
//
// 语义（对照 xray freedom 的 finalRules）：
//   - {block, ip:geoip:private, blockDelay:"0"}：私网目标在「最终 IP 解析后、拨号前」被拦，
//     目标不会被拨号；blockDelay 显式设 0 → 立即关闭连接，而非官方默认的 30-90s 黑洞挂起。
//   - {allow}：无条件的兜底放行规则。freedom 内建安全策略（对来自 VLESS/VMess/Trojan/SS 入站的
//     流量默认阻断私网与保留网段）只在「无显式规则命中」时生效，这条 allow 会取代它——即除私网外
//     一律放行，同时也不再拦保留网段（官方文档提示需自行评估安全影响）。
const DefaultFreedomDirectSettingsJSON = `{"domainStrategy":"AsIs","finalRules":[{"action":"block","ip":["geoip:private"],"blockDelay":"0"},{"action":"allow"}]}`

// ServerRoutingRule 服务器独立路由规则（§多节点 3x-ui 架构）。
type ServerRoutingRule struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	ServerID    uint64    `gorm:"index;not null" json:"server_id"`
	OutboundTag string    `gorm:"size:64;not null" json:"outbound_tag"`
	RuleJSON    string    `gorm:"type:text" json:"rule_json,omitempty"` // 自定义完整 Rule JSON
	Domain      string    `gorm:"type:text" json:"domain,omitempty"`    // 逗号/换行分隔或 JSON 数组
	IP          string    `gorm:"type:text" json:"ip,omitempty"`        // 逗号/换行分隔或 JSON 数组
	Port        string    `gorm:"size:64" json:"port,omitempty"`
	Network     string    `gorm:"size:32" json:"network,omitempty"`
	Protocol    string    `gorm:"size:64" json:"protocol,omitempty"`      // bittorrent / http / tls / quic，逗号分隔多选
	InboundTag  string    `gorm:"type:text" json:"inbound_tag,omitempty"` // 逗号/换行分隔或 JSON 数组
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	Priority    int       `gorm:"default:0" json:"priority"`
	Remark      string    `gorm:"size:255" json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
