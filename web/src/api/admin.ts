import { http } from './http'
import type {
  AdminDashboard,
  AdminUserPage,
  ApiResp,
  CreateInvitationResult,
  Invitation,
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