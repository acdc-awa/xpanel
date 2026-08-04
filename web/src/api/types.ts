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
  expire_at: string | null
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

export interface AdminDashboard {
  total_users: number
  online_servers: number
  total_plans: number
  total_orders: number
  pending_orders: number
}

export interface AdminUser {
  id: number
  username: string
  email: string
  role: Role
  status: number
  plan_id: number
  expire_at: string | null
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

// ===== P5 套餐 / 订单 / 审计 =====

export interface Plan {
  id: number
  name: string
  price_cents: number
  traffic_gb: number
  duration_days: number
  speed_limit_kbps: number
  enabled: boolean
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
  network: string
  tls_type: string
  settings_json: string
  ratio: number
  enabled: boolean
  created_at: string
}


export interface RealitySettings {
  server_name: string
  public_key: string
  short_id: string
  private_key: string
  dest: string
}
export interface WSSettings {
  path: string
  host?: string
}
export interface XHTTPSettings {
  mode: string
  path: string
}
export interface TLSSettings {
  server_name?: string
  cert_file: string
  key_file: string
}
export interface InboundSettings {
  reality?: RealitySettings
  ws?: WSSettings
  xhttp?: XHTTPSettings
  tls?: TLSSettings
}

export interface UserInboundGrant {
  inbounds: InboundItem[]
  granted_ids: number[]
}