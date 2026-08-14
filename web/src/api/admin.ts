import { http } from './http'
import type {
  DashboardData,
  ServerMetricsData,
  AdminUser,
  AdminUserPage,
  ApiResp,
  AuditLog,
  CertItem,
  CreateInvitationResult,
  InboundItem,
  InboundSettings,
  Invitation,
  Order,
  PermissionGroup,
  Plan,
  ServerOutbound,
  ServerRoutingRule,
} from './types'

export type {
  DashboardData,
  ServerMetricsData,
  AdminUser,
  AdminUserPage,
  AuditLog,
  CertItem,
  InboundItem,
  InboundSettings,
  Invitation,
  Order,
  PermissionGroup,
  Plan,
  ServerOutbound,
  ServerRoutingRule,
} from './types'

export function getDashboard() {
  return http.get<ApiResp<DashboardData>>('/admin/dashboard')
}

export function getServerMetrics(id: number, range = '1h') {
  return http.get<ApiResp<ServerMetricsData>>(`/admin/servers/${id}/metrics`, { params: { range } })
}

// ===== 站点设置（设置页） =====

export interface SiteSettings {
  web_base: string
}

export function getSettings() {
  return http.get<ApiResp<SiteSettings>>('/admin/settings')
}

export function updateSettings(payload: Partial<SiteSettings>) {
  return http.put<ApiResp<SiteSettings>>('/admin/settings', payload)
}

export function getUsers(page = 1, size = 20) {
  return http.get<ApiResp<AdminUserPage>>('/admin/users', { params: { page, size } })
}

export function updateUser(
  id: number,
  payload: {
    plan_id?: number
    permission_group_id?: number
    device_limit?: number
    expire_at?: string | null
    status?: number
    password?: string
  },
) {
  return http.put<ApiResp<{ user: AdminUser }>>(`/admin/users/${id}`, payload)
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
  config_status: string // pushed / pending / ''（无待推送配置）
  default_outbound_tag: string // 路由默认出口
  routing_domain_strategy: string // 路由域名策略（路由匹配阶段）
  default_outbound_domain_strategy: string // 默认出口出站解析策略（freedom: AsIs/UseIP/UseIPv4/UseIPv6）
  last_seen_at: string | null
  created_at: string
}

export interface CreateServerResult {
  server: ServerItem
  node_id: string
  secret: string // 仅创建时返回一次
  install_cmd: string // 节点一键安装命令（含 secret，仅创建时有效）
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

export function updateServer(
  id: number,
  payload: { name?: string; host?: string; location?: string; remark?: string; default_outbound_tag?: string; routing_domain_strategy?: string; default_outbound_domain_strategy?: string },
) {
  return http.put<ApiResp<{ server: ServerItem }>>(`/admin/servers/${id}`, payload)
}

export function resetServerSecret(id: number) {
  return http.post<ApiResp<{ node_id: string; secret: string; install_cmd?: string }>>(`/admin/servers/${id}/reset-secret`)
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
  listen?: string
  settings_json?: string
  stream_settings?: string
  sniffing?: string
  ratio?: number
  type?: string // user / relay / idle（Phase T）
  cert_id?: number
  flow?: string // 入站级流控：空=自动 / xtls-rprx-vision / none
  share_addr_strategy?: string // node / listen / custom（订阅专用）
  share_addr?: string // 自定义分享地址
  share_port?: number // 自定义分享端口（0 = 用入站端口）
  permission_group_ids?: number[] // 开放权限组 ID 列表
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

// Phase T：relay 内部账户指令（UUID 由节点生成上报）
export function setupInternalInbound(id: number) {
  return http.post<ApiResp<{ inbound_id: number; internal_uuid: string }>>(`/admin/inbounds/${id}/setup-internal`)
}

export function rotateInternalInbound(id: number) {
  return http.post<ApiResp<{ inbound_id: number; internal_uuid: string }>>(`/admin/inbounds/${id}/rotate-internal`)
}

// Phase T：证书管理
export function getCerts() {
  return http.get<ApiResp<{ items: CertItem[] }>>('/admin/certs')
}

export interface CertPayload {
  domain: string
  cert_pem: string
  key_pem: string
  remark?: string
}

export function createCert(payload: CertPayload) {
  return http.post<ApiResp<{ cert: CertItem }>>('/admin/certs', payload)
}

export function updateCert(id: number, payload: Partial<CertPayload>) {
  return http.put<ApiResp<{ cert: CertItem }>>(`/admin/certs/${id}`, payload)
}

export function deleteCert(id: number) {
  return http.delete<ApiResp<{ ok: boolean }>>(`/admin/certs/${id}`)
}

// Phase T：权限组
export function getPermissionGroups() {
  return http.get<ApiResp<{ items: PermissionGroup[] }>>('/admin/permission-groups')
}

export interface PermissionGroupPayload {
  name: string
  remark?: string
  clash_template?: string
}

export function createPermissionGroup(payload: PermissionGroupPayload) {
  return http.post<ApiResp<{ group: PermissionGroup }>>('/admin/permission-groups', payload)
}

export function updatePermissionGroup(id: number, payload: Partial<PermissionGroupPayload>) {
  return http.put<ApiResp<{ group: PermissionGroup }>>(`/admin/permission-groups/${id}`, payload)
}

export interface TemplatePreviewResult {
  rendered: string
  proxy_count: number
  proxy_names: string[]
  is_sample_nodes: boolean
}

export function previewPermissionGroupTemplate(id: number, template: string) {
  return http.post<ApiResp<TemplatePreviewResult>>(`/admin/permission-groups/${id}/preview-template`, {
    template,
  })
}

export function deletePermissionGroup(id: number) {
  return http.delete<ApiResp<{ ok: boolean }>>(`/admin/permission-groups/${id}`)
}

export function getGroupInbounds(id: number) {
  return http.get<ApiResp<{ inbound_ids: number[] }>>(`/admin/permission-groups/${id}/inbounds`)
}

export function setGroupInbounds(id: number, inboundIds: number[]) {
  return http.post<ApiResp<{ group_id: number; count: number }>>(`/admin/permission-groups/${id}/inbounds`, {
    inbound_ids: inboundIds,
  })
}

export function previewConfig(serverId: number, form?: Partial<InboundPayload>) {
  return http.post<ApiResp<{ config: string }>>('/admin/xray/preview-config', {
    server_id: serverId,
    form,
  })
}

export function getXrayKeys() {
  return http.get<ApiResp<{ private_key: string; public_key: string }>>('/admin/xray/keys')
}

export function getServerConfigPreview(serverId: number) {
  return http.get<ApiResp<{ config: string }>>(`/admin/servers/${serverId}/config-preview`)
}

export function generateAndPushConfig(serverId: number) {
  return http.post<ApiResp<{ ok: boolean; pushed: boolean; message: string; config: string }>>(
    `/admin/servers/${serverId}/generate-config`,
  )
}

// ===== M2 节点出站（3x-ui outbound） =====

export interface OutboundPayload {
  tag: string
  protocol: string
  settings_json?: string
  stream_settings_json?: string
  send_through?: string
  enabled?: boolean
  priority?: number
  remark?: string
  inbound_ref?: number // Phase T：引用落地入站（0/null = 手动）
}

export function getServerOutbounds(serverId: number) {
  return http.get<ApiResp<{ items: ServerOutbound[] }>>(`/admin/servers/${serverId}/outbounds`)
}

export function createServerOutbound(serverId: number, payload: OutboundPayload) {
  return http.post<ApiResp<{ outbound: ServerOutbound }>>(`/admin/servers/${serverId}/outbounds`, payload)
}

export function updateServerOutbound(serverId: number, outboundId: number, payload: Partial<OutboundPayload>) {
  return http.put<ApiResp<{ outbound: ServerOutbound }>>(
    `/admin/servers/${serverId}/outbounds/${outboundId}`,
    payload,
  )
}

export function deleteServerOutbound(serverId: number, outboundId: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/servers/${serverId}/outbounds/${outboundId}`)
}

// ===== M2 节点路由规则（3x-ui routing） =====

export interface RoutingRulePayload {
  outbound_tag: string
  rule_json?: string
  domain?: string
  ip?: string
  port?: string
  network?: string
  protocol?: string
  inbound_tag?: string
  enabled?: boolean
  priority?: number
  remark?: string
}

export function getServerRoutingRules(serverId: number) {
  return http.get<ApiResp<{ items: ServerRoutingRule[] }>>(`/admin/servers/${serverId}/routing`)
}

export function createServerRoutingRule(serverId: number, payload: RoutingRulePayload) {
  return http.post<ApiResp<{ rule: ServerRoutingRule }>>(`/admin/servers/${serverId}/routing`, payload)
}

export function updateServerRoutingRule(serverId: number, ruleId: number, payload: Partial<RoutingRulePayload>) {
  return http.put<ApiResp<{ rule: ServerRoutingRule }>>(
    `/admin/servers/${serverId}/routing/${ruleId}`,
    payload,
  )
}

export function deleteServerRoutingRule(serverId: number, ruleId: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/servers/${serverId}/routing/${ruleId}`)
}

// ===== T8 拓扑画布：一次拉全量 =====

export interface TopologyOutbound {
  id: number
  server_id: number
  tag: string
  protocol: string
  inbound_ref?: number | null // Phase T：引用落地入站
  enabled: boolean
  priority: number
}

export interface TopologyRule {
  id: number
  server_id: number
  inbound_tag: string
  outbound_tag: string
  enabled: boolean
}

export interface TopologyData {
  servers: ServerItem[]
  inbounds: InboundItem[]
  outbounds: TopologyOutbound[]
  routing_rules: TopologyRule[]
}

export function getTopology() {
  return http.get<ApiResp<TopologyData>>('/admin/topology')
}

// 画布布局云端同步（盒子位置/宽度 + 内容哈希去重，跨浏览器/设备统一；settings 表存 JSON）
export interface TopologyLayout {
  hash?: string
  positions: Record<string, { x: number; y: number }>
  widths: Record<string, number>
}

export function getTopologyLayout() {
  return http.get<ApiResp<TopologyLayout>>('/admin/topology-layout')
}

export function saveTopologyLayout(payload: TopologyLayout) {
  return http.put<ApiResp<null>>('/admin/topology-layout', payload)
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

export function getAuditLogs(page = 1, size = 20, action?: string) {
  return http.get<ApiResp<{ total: number; page: number; size: number; items: AuditLog[] }>>(
    '/admin/audit-logs',
    { params: { page, size, action } },
  )
}

// ===== 手动用户管理 =====

export interface CreateUserResult {
  id: number
  username: string
  email: string
  uuid: string
  role: string
  status: number
}

export function createUser(payload: {
  email: string
  password: string
  plan_id?: number
  permission_group_id?: number
  device_limit?: number
  expire_at?: string | null
}) {
  return http.post<ApiResp<CreateUserResult>>('/admin/users', payload)
}

export function toggleUser(id: number) {
  return http.post<ApiResp<{ id: number; status: number }>>(`/admin/users/${id}/toggle`)
}

export function resetUserTraffic(id: number) {
  return http.post<ApiResp<{ ok: boolean }>>(`/admin/users/${id}/reset-traffic`)
}

export function deleteUser(id: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/users/${id}`)
}