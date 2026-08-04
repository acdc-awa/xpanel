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