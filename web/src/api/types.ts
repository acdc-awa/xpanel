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
  device_limit?: number
  effective_device_limit?: number
  expire_at: string | null
  balance_cents?: number
  subscribe_token?: string
  created_at: string
  up_bytes: number
  down_bytes: number
  used_bytes: number
  total_bytes: number
}

export interface LoginResult {
  user: UserInfo
  access_token: string
  refresh_token: string
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
  pending_orders: number
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
  price_cents: number
  traffic_gb: number
  duration_days: number
  speed_limit_kbps: number
  device_limit: number
  enabled: boolean
  permission_group_id?: number // Phase T：绑定权限组（套餐自动授权）
  created_at: string
}

export type OrderStatus = 'pending' | 'paid' | 'cancelled'

export interface Order {
  id: number
  order_no: string
  user_id: number
  username: string
  plan_id: number
  plan_name: string
  amount_cents: number
  payment_method?: 'balance' | 'manual'
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
  enabled: boolean
  type?: string // user / relay / idle（Phase T）
  internal_uuid?: string // relay 只读（节点生成上报）
  cert_id?: number // 绑定的证书
  flow?: string // 入站级流控：空=自动 / xtls-rprx-vision / none
  share_addr_strategy?: string // node / listen / custom（订阅专用）
  share_addr?: string // 自定义分享地址
  share_port?: number // 自定义分享端口（0 = 用入站端口）
  permission_group_ids?: number[] // 开放权限组 ID 列表（权威来源：节点定义权限组）
  created_at: string
}

// Phase T：入站三态
export const INBOUND_TYPE_USER = 'user'
export const INBOUND_TYPE_RELAY = 'relay'
export const INBOUND_TYPE_IDLE = 'idle'


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

export interface WSSettings {
  path?: string
  host?: string
  headers?: { Host?: string }
}

export interface XHTTPSettings {
  mode?: string
  path?: string
  host?: string
}

export interface GRPCSettings {
  service_name?: string
  authority?: string
  multi_mode?: boolean
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
  ws?: WSSettings
  xhttp?: XHTTPSettings
  grpc?: GRPCSettings
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
  inbound_count?: number
  inbound_tags?: string[]
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

export interface UserInboundGrant {
  inbounds: InboundItem[]
  granted_ids: number[]
}