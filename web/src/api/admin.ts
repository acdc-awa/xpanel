import { http } from './http'
import type {
  AdminDashboard,
  AdminUserPage,
  ApiResp,
  AuditLog,
  CreateInvitationResult,
  InboundItem,
  Invitation,
  Order,
  Plan,
  UserInboundGrant,
} from './types'

export type {
  AdminDashboard,
  AdminUserPage,
  AuditLog,
  InboundItem,
  Invitation,
  Order,
  Plan,
  UserInboundGrant,
} from './types'

export function getDashboard() {
  return http.get<ApiResp<AdminDashboard>>('/admin/dashboard')
}

export function getUsers(page = 1, size = 20) {
  return http.get<ApiResp<AdminUserPage>>('/admin/users', { params: { page, size } })
}

export function getInvitations() {
  return http.get<ApiResp<{ items: Invitation[] }>>('/admin/invitations')
}

export function createInvitations(count: number, expires?: string) {
  return http.post<ApiResp<CreateInvitationResult>>('/admin/invitations', {
    count,
    expires: expires ?? '',
  })
}

// ===== P1 节点通道：服务器管理 =====

export interface ServerItem {
  id: number
  name: string
  host: string
  node_id: string
  location: string
  remark: string
  status: number // 0 离线 1 在线
  last_seen_at: string | null
  created_at: string
}

export interface CreateServerResult {
  server: ServerItem
  node_id: string
  secret: string // 仅创建时返回一次
}

export interface CommandResult<T = unknown> {
  ok: boolean
  error: string
  data: T
}

export function getServers() {
  return http.get<ApiResp<{ items: ServerItem[] }>>('/admin/servers')
}

export function createServer(payload: {
  name: string
  host: string
  location?: string
  remark?: string
}) {
  return http.post<ApiResp<CreateServerResult>>('/admin/servers', payload)
}

export function deleteServer(id: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/servers/${id}`)
}

export function serverCommand(
  id: number,
  type: 'push_config' | 'restart_xray' | 'get_status' | 'get_logs',
  extra?: { config_json?: string; lines?: number },
) {
  return http.post<ApiResp<CommandResult>>(`/admin/servers/${id}/command`, {
    type,
    ...extra,
  })
}

// ===== P3 入站管理 + 配置生成 =====

export interface InboundPayload {
  server_id: number
  tag: string
  protocol: string
  port: number
  network: string
  tls_type?: string
  settings_json?: string
  ratio?: number
}

export function getInbounds(serverId?: number) {
  return http.get<ApiResp<{ items: InboundItem[] }>>('/admin/inbounds', {
    params: serverId ? { server_id: serverId } : {},
  })
}

export function createInbound(payload: InboundPayload) {
  return http.post<ApiResp<{ inbound: InboundItem }>>('/admin/inbounds', payload)
}

export function updateInbound(id: number, payload: Partial<InboundPayload>) {
  return http.put<ApiResp<{ inbound: InboundItem }>>(`/admin/inbounds/${id}`, payload)
}

export function deleteInbound(id: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/inbounds/${id}`)
}

export function toggleInbound(id: number) {
  return http.post<ApiResp<{ id: number; enabled: boolean }>>(`/admin/inbounds/${id}/toggle`)
}

export function generateAndPushConfig(serverId: number) {
  return http.post<ApiResp<{ ok: boolean; error: string; config: string }>>(
    `/admin/servers/${serverId}/generate-config`,
  )
}

// ===== P5 套餐 / 订单 / 审计 / 入站授权 =====

export function getPlans() {
  return http.get<ApiResp<{ items: Plan[] }>>('/admin/plans')
}

export function createPlan(payload: Partial<Plan>) {
  return http.post<ApiResp<{ plan: Plan }>>('/admin/plans', payload)
}

export function updatePlan(id: number, payload: Partial<Plan>) {
  return http.put<ApiResp<{ plan: Plan }>>(`/admin/plans/${id}`, payload)
}

export function deletePlan(id: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/plans/${id}`)
}

export function getOrders(page = 1, size = 20, status?: string) {
  return http.get<ApiResp<{ total: number; page: number; size: number; items: Order[] }>>(
    '/admin/orders',
    { params: { page, size, status } },
  )
}

export function confirmOrder(id: number) {
  return http.post<ApiResp<{ confirmed: number }>>(`/admin/orders/${id}/confirm`)
}

export function cancelOrder(id: number) {
  return http.post<ApiResp<{ cancelled: number }>>(`/admin/orders/${id}/cancel`)
}

export function getAuditLogs(page = 1, size = 20, action?: string) {
  return http.get<ApiResp<{ total: number; page: number; size: number; items: AuditLog[] }>>(
    '/admin/audit-logs',
    { params: { page, size, action } },
  )
}

export function getUserInbounds(userId: number) {
  return http.get<ApiResp<UserInboundGrant>>(`/admin/users/${userId}/inbounds`)
}

export function setUserInbounds(userId: number, inboundIds: number[]) {
  return http.post<ApiResp<{ user_id: number; granted: number }>>(`/admin/users/${userId}/inbounds`, {
    inbound_ids: inboundIds,
  })
}