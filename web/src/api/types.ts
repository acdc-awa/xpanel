/** 后端统一响应：{ code, message, data }，code=0 成功。 */
export interface ApiResp<T = unknown> {
  code: number
  message: string
  data: T
}

export type Role = 'admin' | 'user'

export interface UserInfo {
  id: number
  username: string
  email: string
  role: Role
  status: number
  plan_id: number
  plan_name?: string
  device_limit?: number
  effective_device_limit?: number
  is_custom_device_limit?: boolean
  permission_group_id?: number
  effective_group_id?: number
  expire_at: string | null
  balance_cents?: number
  subscribe_token?: string
  subscribe_url?: string
  subscribe_path?: string
  created_at: string
  up_bytes: number
  down_bytes: number
  used_bytes: number
  total_bytes: number
  must_change_pwd?: boolean
  totp_enabled?: boolean
  auto_renew_expire?: boolean
  auto_renew_exhaust?: boolean
}

export interface LoginResult {
  user?: UserInfo
  twofa_required?: boolean
  access_token?: string
  refresh_token?: string
}

export interface DashboardSummary {
  today_revenue_cents: number
  today_used_cards_count: number
  month_revenue_cents: number
  total_revenue_cents: number
  today_traffic_up: number
  today_traffic_down: number
  today_traffic_total: number
  month_traffic_total: number
  online_servers: number
  total_servers: number
  active_users: number
  total_users: number
  total_orders: number
  today_orders: number
  realtime_rx_rate: number
  realtime_tx_rate: number
}

export interface TrafficTrendPoint {
  date: string
  up_bytes: number
  down_bytes: number
  total_bytes: number
}

export interface ServerTrafficItem {
  server_id: number
  name: string
  location: string
  up_bytes: number
  down_bytes: number
  total_bytes: number
  percent: number
}

export interface UserTrafficRankItem {
  user_id: number
  username: string
  email: string
  plan_name: string
  up_bytes: number
  down_bytes: number
  total_bytes: number
}

export interface ServerMatrixItem {
  id: number
  name: string
  node_id: string
  host: string
  location: string
  status: number
  last_seen_at: string | null
  cpu: number
  mem: number
  mem_total: number
  disk: number
  disk_total: number
  rx_rate: number
  tx_rate: number
  online_users: number
  is_active_flow: boolean
}

export interface RecentGiftCardItem {
  id: number
  code_masked: string
  name: string
  face_value_cents: number
  used_by_username: string
  used_at: string
}

export interface RecentOrderItem {
  id: number
  order_no: string
  username: string
  plan_name: string
  amount_cents: number
  payment_method: string
  status: string
  created_at: string
  paid_at: string | null
}

export interface DashboardData {
  summary: DashboardSummary
  traffic_trend: TrafficTrendPoint[]
  server_breakdown: ServerTrafficItem[]
  user_rank: UserTrafficRankItem[]
  server_matrix: ServerMatrixItem[]
  recent_gift_cards: RecentGiftCardItem[]
  recent_orders: RecentOrderItem[]
}

export interface ServerMetricsData {
  server_id: number
  server_name: string
  host: string
  location: string
  range: string
  timestamps: string[]
  cpu: number[]
  mem_percent: number[]
  mem_used: number[]
  mem_total: number
  disk_percent: number[]
  disk_total: number
  rx_mbps: number[]
  tx_mbps: number[]
  online_users: number[]
}

export interface AdminUser {
  id: number
  username: string
  email: string
  uuid: string
  remark?: string
  role: Role
  status: number
  plan_id: number
  permission_group_id?: number
  effective_group_id?: number
  device_limit: number
  effective_device_limit?: number
  is_custom_device_limit?: boolean
  expire_at: string | null
  balance_cents?: number
  created_at: string
  up_bytes: number
  down_bytes: number
  used_bytes: number
  total_bytes: number
  auto_renew_expire?: boolean
  auto_renew_exhaust?: boolean
}

export interface AdminUserPage {
  total: number
  page: number
  size: number
  items: AdminUser[]
}

export interface Invitation {
  id: number
  code: string
  created_by: number
  used_by: number
  used_at: string | null
  expires_at: string | null
  status: number
  created_at: string
}

export interface CreateInvitationResult {
  count: number
  codes: string[]
}

// ===== 礼品卡与账务系统 =====
export interface GiftCard {
  id: number
  code: string
  name: string
  face_value_cents: number
  status: 'unused' | 'used' | 'disabled'
  used_by: number
  used_at: string | null
  expires_at: string | null
  created_by: number
  created_at: string
}

export interface BalanceLog {
  id: number
  user_id: number
  amount_cents: number
  balance_after: number
  type: 'recharge_gift_card' | 'order_payment' | 'admin_adjust' | 'refund'
  related_id: number
  remark: string
  created_at: string
}

// ===== P5 套餐 / 订单 / 审计 =====

export interface Plan {
  id: number
  name: string
  description?: string
  price_cents: number
  traffic_gb: number
  duration_days: number
  device_limit: number
  purchasable: boolean // 可新购（商店展示并允许非持有者购买）
  renewable: boolean // 可续费（持有者余额直付顺延）
  permission_group_id?: number // Phase T：绑定权限组（套餐自动授权）
  sort_order?: number // 商城展示排序（越小越靠前）
  is_featured?: boolean // 商城「热门推荐」标记
  created_at: string
}

export type OrderStatus = 'paid'

export interface Order {
  id: number
  order_no: string
  user_id: number
  username: string
  plan_id: number
  plan_name: string
  amount_cents: number
  payment_method?: 'balance'
  status: OrderStatus
  created_at: string
  paid_at: string | null
}

export interface AuditLog {
  id: number
  operator_type: string
  operator_id: number
  action: string
  detail: string
  ip: string
  created_at: string
}


export interface InboundItem {
  id: number
  server_id: number
  server_name: string
  tag: string
  protocol: string
  port: number
  listen: string
  settings_json: string
  stream_settings: string
  sniffing: string
  ratio: number
  total_gb?: number // 入站总流量上限（GB，0=不限）
  expiry_time?: string | null // 入站到期时间（null/缺失=永久）
  enabled: boolean
  type?: string // user / relay（Phase T）
  internal_uuid?: string // relay 只读（节点生成上报）
  cert_id?: number // 绑定的证书
  flow?: string // 入站级流控：空=自动 / xtls-rprx-vision / none
  share_addr_strategy?: string // node / custom（订阅专用，listen 已退役）
  share_addr?: string // 自定义分享地址
  share_port?: number // 自定义分享端口（0 = 用入站端口）
  share_security?: string // auto / tls / none（订阅安全层覆写）
  share_sni?: string // 订阅 SNI 覆写
  share_host?: string // 订阅 HTTP/WS Host 覆写
  share_path?: string // 订阅 WS/XHTTP Path 覆写
  share_allow_insecure?: boolean // 订阅跳过证书检查
  layer_id?: number // 所属对外接入层（挂层后订阅端点由层决议）
  created_at: string
}

// 对外接入层（订阅端点语义的显式分组：对外 host/port/security 自定义，内部实现不可见）
export interface AccessLayer {
  id: number
  server_id: number
  name: string
  host: string
  port: number
  security: string // tls / none
  remark?: string
  inbound_count?: number
  created_at?: string
  updated_at?: string
}

// 用户接入点（消费者模型：命名 + 权限组白名单 + 可选对外端点覆写，数据沿管道自适应继承）
export interface UserAccessPoint {
  id: number
  name: string
  custom_host?: string
  custom_port?: number
  target_type: 'inbound' | ''
  target_inbound_id?: number
  resolved_host?: string
  resolved_port?: number
  resolved_protocol?: string
  resolved_target_desc?: string
  target_server_name?: string
  target_inbound_tag?: string
  enabled: boolean
  remark?: string
  permission_group_ids: number[]
  created_at?: string
  updated_at?: string
}


export interface RealitySettings {
  server_name?: string
  server_names?: string[]
  public_key?: string
  short_id?: string
  short_ids?: string[]
  private_key?: string
  dest?: string
  spider_x?: string
}

export interface XHTTPSettings {
  mode?: string
  path?: string
  host?: string
}

export interface SniffingSettings {
  enabled: boolean
  destOverride?: string[]
  metadataOnly?: boolean
  routeOnly?: boolean
  dest_override?: string[]
  metadata_only?: boolean
  route_only?: boolean
}

export interface TLSSettings {
  server_name?: string
  cert_file?: string
  key_file?: string
  alpn?: string[]
  min_version?: string
  max_version?: string
  cipher_suites?: string[]
  certificates?: { certificate_file: string; key_file: string }[]
}

export interface FallbackItem {
  name?: string
  alpn?: string
  path?: string
  dest: string
  xver?: number
}

export interface VlessClientItem {
  id: string
  flow?: string
  email?: string
}

export interface InboundSettings {
  network?: string
  tls_type?: string
  flow?: string
  uuid?: string
  clients?: VlessClientItem[]
  reality?: RealitySettings
  xhttp?: XHTTPSettings
  tls?: TLSSettings
  sniffing?: SniffingSettings
  fallbacks?: FallbackItem[]
  // 顶层透传字段
  ratio?: number
  share_addr_strategy?: string
  share_addr?: string
  share_port?: number
  permission_group_ids?: number[]
}

export interface ServerOutbound {
  id: number
  server_id: number
  tag: string
  protocol: string
  settings_json: string
  stream_settings_json?: string
  send_through?: string
  enabled: boolean
  priority: number
  remark: string
  inbound_ref?: number // Phase T：引用落地入站
  created_at: string
  updated_at: string
}

// Phase T：证书（PEM 不回传）
export interface CertRef {
  server_id: number
  server_name: string
  inbound_id: number
  inbound_tag: string
}

export interface CertItem {
  id: number
  domain: string
  not_after: string
  pin_sha256: string // leaf DER SHA-256（hex），链式代理 TLS 中转出站自动 pin
  self_signed: boolean // 面板一键生成的自签证书
  remark: string
  refs: CertRef[] // 引用该证书的入站位置（服务器/入站标签）
  created_at: string
}

// Phase T：权限组
export interface PermissionGroup {
  id: number
  name: string
  remark: string
  clash_template?: string // 自定义 Clash/Mihomo 模板
  access_point_count?: number
  access_point_ids?: number[] // 组内优先级序（订阅输出同序）
  access_point_names?: string[]
  created_at: string
  updated_at: string
}

export interface ServerRoutingRule {
  id: number
  server_id: number
  outbound_tag: string
  rule_json?: string
  domain?: string
  ip?: string
  port?: string
  network?: string
  protocol?: string
  inbound_tag?: string
  enabled: boolean
  priority: number
  remark: string
  created_at: string
  updated_at: string
}

export interface NoticeItem {
  id: number
  title: string
  content: string
  is_pinned: boolean
  is_popup: boolean
  status: number
  sort_order: number
  created_at: string
  updated_at: string
}

export interface NoticePayload {
  title: string
  content: string
  is_pinned?: boolean
  is_popup?: boolean
  status?: number
  sort_order?: number
}
