<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Plus,
  Refresh,
  Delete,
  Edit,
  CopyDocument,
  Connection,
  Search,
  Key,
  MagicStick,
  User,
  Timer,
  Coin,
  View,
  Hide,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import BaseCard from '@/components/base/BaseCard.vue'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  getForwardUsers,
  getForwardUsersSummary,
  createForwardUser,
  updateForwardUser,
  deleteForwardUser,
  toggleForwardUser,
  resetForwardUserTraffic,
  getPermissionGroups,
  getInbounds,
  getServers,
  getAccessPoints,
  syncForwardUsersConfigs,
  type ForwardUserItem,
  type ForwardUserPayload,
  type ForwardUsersSummary,
  type PermissionGroup,
  type InboundItem,
  type ServerItem,
  type UserAccessPoint,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'
import { formatDateTime } from '@/utils/timezone'

const loading = ref(false)
const users = ref<ForwardUserItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

// 概览指标
const summary = ref<ForwardUsersSummary>({
  total_users: 0,
  active_users: 0,
  total_limit_gb: 0,
  total_used_bytes: 0,
})

// 筛选状态
const filterGroupId = ref<number | ''>('')
const filterStatus = ref<string>('')
const filterKeyword = ref<string>('')

// 辅助数据与操作状态
const permissionGroups = ref<PermissionGroup[]>([])
const servers = ref<ServerItem[]>([])
const inbounds = ref<InboundItem[]>([])
const accessPoints = ref<UserAccessPoint[]>([])
const syncingConfigs = ref(false)

// 密码显示状态映射 (user.id -> boolean)
const showPasswordMap = ref<Record<number, boolean>>({})

function togglePasswordVisibility(id: number) {
  showPasswordMap.value[id] = !showPasswordMap.value[id]
}

// 格式化辅助
function resetCycleLabel(cycle: string) {
  switch (cycle) {
    case 'daily':
      return '每日重置'
    case 'weekly':
      return '每周重置'
    case 'monthly':
      return '每月重置'
    case 'never':
    default:
      return '不重置'
  }
}

function calcStatus(u: ForwardUserItem) {
  if (!u.enabled) return { label: '已停用', type: 'info' as const }
  if (u.expires_at) {
    const exp = new Date(u.expires_at).getTime()
    if (!isNaN(exp) && exp < Date.now()) {
      return { label: '已到期', type: 'warning' as const }
    }
  }
  if (u.traffic_limit_gb > 0) {
    const limitBytes = u.traffic_limit_gb * 1024 * 1024 * 1024
    if (u.traffic_used_bytes >= limitBytes) {
      return { label: '超额熔断', type: 'danger' as const }
    }
  }
  return { label: '正常运行', type: 'success' as const }
}

function progressPercent(u: ForwardUserItem) {
  if (!u.traffic_limit_gb || u.traffic_limit_gb <= 0) return 0
  const limitBytes = u.traffic_limit_gb * 1024 * 1024 * 1024
  const pct = Math.round((u.traffic_used_bytes / limitBytes) * 100)
  return Math.min(100, Math.max(0, pct))
}

function progressColor(pct: number) {
  if (pct >= 90) return '#f56c6c'
  if (pct >= 70) return '#e6a23c'
  return '#409eff'
}

// 加载数据
async function loadSummary() {
  try {
    const res = await getForwardUsersSummary()
    if (res.data.code === 0 && res.data.data) {
      summary.value = res.data.data
    }
  } catch {
    // 概览指标加载失败不阻塞列表
  }
}

async function loadAuxData() {
  try {
    const [grpRes, srvRes, inbRes, apRes] = await Promise.all([
      getPermissionGroups(),
      getServers(),
      getInbounds(),
      getAccessPoints(),
    ])
    if (grpRes.data.code === 0) permissionGroups.value = grpRes.data.data.items
    if (srvRes.data.code === 0) servers.value = srvRes.data.data.items
    if (inbRes.data.code === 0) inbounds.value = inbRes.data.data.items
    if (apRes.data.code === 0) accessPoints.value = apRes.data.data.items
  } catch {
    // 忽略非阻塞错误
  }
}

async function loadData() {
  loading.value = true
  try {
    const res = await getForwardUsers({
      page: page.value,
      page_size: pageSize.value,
      permission_group_id: filterGroupId.value !== '' ? Number(filterGroupId.value) : undefined,
      enabled: filterStatus.value === 'enabled' ? true : filterStatus.value === 'disabled' ? false : undefined,
      keyword: filterKeyword.value.trim() || undefined,
    })
    if (res.data.code === 0 && res.data.data) {
      users.value = res.data.data.items || []
      total.value = res.data.data.total || 0
    } else {
      ElMessage.error(res.data.message || '加载转发用户失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载转发用户失败'))
  } finally {
    loading.value = false
  }
}

function reloadAll() {
  loadSummary()
  loadData()
}

onMounted(() => {
  loadAuxData()
  loadSummary()
  loadData()
})

// 复制工具函数
async function copyText(text: string, label = '内容') {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败，请手动选择复制')
  }
}

// 快速启停
async function handleToggle(u: ForwardUserItem) {
  try {
    const res = await toggleForwardUser(u.id)
    if (res.data.code === 0) {
      ElMessage.success(u.enabled ? `已启用用户「${u.username}」` : `已停用用户「${u.username}」`)
      loadSummary()
    } else {
      u.enabled = !u.enabled
      ElMessage.error(res.data.message || '操作失败')
    }
  } catch (e) {
    u.enabled = !u.enabled
    ElMessage.error(errMsg(e, '切换状态失败'))
  }
}

// 重置流量
async function handleResetTraffic(u: ForwardUserItem) {
  try {
    await ElMessageBox.confirm(
      `确定清零转发用户「${u.username}」的本周期已用流量吗？清零后该用户账号若处于熔断状态将自动恢复。`,
      '重置流量',
      {
        confirmButtonText: '确定清零',
        cancelButtonText: '取消',
        type: 'warning',
      },
    )
  } catch {
    return
  }

  try {
    const res = await resetForwardUserTraffic(u.id)
    if (res.data.code === 0) {
      ElMessage.success(`已清零用户「${u.username}」的流量`)
      reloadAll()
    } else {
      ElMessage.error(res.data.message || '重置失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置流量失败'))
  }
}

// 删除用户
async function handleDelete(u: ForwardUserItem) {
  try {
    await ElMessageBox.confirm(
      `确定删除转发用户「${u.username}」吗？删除后该用户将立即从节点 SOCKS5 / HTTP 入站账号中移除，连接将被断开。`,
      '删除转发用户',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'error',
      },
    )
  } catch {
    return
  }

  try {
    const res = await deleteForwardUser(u.id)
    if (res.data.code === 0) {
      ElMessage.success(`转发用户「${u.username}」已删除`)
      reloadAll()
    } else {
      ElMessage.error(res.data.message || '删除失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除转发用户失败'))
  }
}

// ===== 新建 / 编辑弹窗 =====
const formVisible = ref(false)
const formSaving = ref(false)
const editingUser = ref<ForwardUserItem | null>(null)

const formData = reactive<{
  username: string
  password: string
  permission_group_id: number | ''
  traffic_limit_gb: number
  traffic_reset: 'never' | 'daily' | 'weekly' | 'monthly'
  expires_at: string | null
  enabled: boolean
  remark: string
}>({
  username: '',
  password: '',
  permission_group_id: '',
  traffic_limit_gb: 0,
  traffic_reset: 'never',
  expires_at: null,
  enabled: true,
  remark: '',
})

function generateRandomPassword(len = 16) {
  const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789'
  let res = ''
  for (let i = 0; i < len; i++) {
    res += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  formData.password = res
}

function openCreate() {
  editingUser.value = null
  formData.username = ''
  formData.password = ''
  generateRandomPassword(16)
  formData.permission_group_id = permissionGroups.value.length > 0 ? permissionGroups.value[0].id : ''
  formData.traffic_limit_gb = 0
  formData.traffic_reset = 'never'
  formData.expires_at = null
  formData.enabled = true
  formData.remark = ''
  formVisible.value = true
}

function openEdit(u: ForwardUserItem) {
  editingUser.value = u
  formData.username = u.username
  formData.password = u.password
  formData.permission_group_id = u.permission_group_id
  formData.traffic_limit_gb = u.traffic_limit_gb
  formData.traffic_reset = (u.traffic_reset as any) || 'never'
  formData.expires_at = u.expires_at ? new Date(u.expires_at).toISOString() : null
  formData.enabled = u.enabled
  formData.remark = u.remark || ''
  formVisible.value = true
}

async function handleFormSubmit() {
  if (!editingUser.value && !formData.username.trim()) {
    ElMessage.warning('请输入用户名')
    return
  }
  if (!formData.password.trim()) {
    ElMessage.warning('请输入密码')
    return
  }
  if (!formData.permission_group_id) {
    ElMessage.warning('请选择所属权限组')
    return
  }

  formSaving.value = true
  try {
    if (editingUser.value) {
      const payload: Partial<ForwardUserPayload> = {
        password: formData.password.trim(),
        permission_group_id: Number(formData.permission_group_id),
        traffic_limit_gb: Number(formData.traffic_limit_gb) || 0,
        traffic_reset: formData.traffic_reset,
        expires_at: formData.expires_at ? new Date(formData.expires_at).toISOString() : undefined,
        enabled: formData.enabled,
        remark: formData.remark,
      }
      const res = await updateForwardUser(editingUser.value.id, payload)
      if (res.data.code === 0) {
        ElMessage.success('转发用户已更新')
        formVisible.value = false
        reloadAll()
      } else {
        ElMessage.error(res.data.message || '更新失败')
      }
    } else {
      const payload: ForwardUserPayload = {
        username: formData.username.trim(),
        password: formData.password.trim(),
        permission_group_id: Number(formData.permission_group_id),
        traffic_limit_gb: Number(formData.traffic_limit_gb) || 0,
        traffic_reset: formData.traffic_reset,
        expires_at: formData.expires_at ? new Date(formData.expires_at).toISOString() : undefined,
        enabled: formData.enabled,
        remark: formData.remark,
      }
      const res = await createForwardUser(payload)
      if (res.data.code === 0) {
        ElMessage.success('转发用户已创建')
        formVisible.value = false
        reloadAll()
      } else {
        ElMessage.error(res.data.message || '创建失败')
      }
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    formSaving.value = false
  }
}

// ===== 连接信息弹窗 =====
const connVisible = ref(false)
const connUser = ref<ForwardUserItem | null>(null)

interface MatchedInbound {
  id: number
  serverName: string
  apName?: string
  protocol: string
  port: number
  host: string
  isMixed?: boolean
  socksUrl?: string
  httpUrl?: string
  connectionUrl: string
}

const matchedInbounds = computed<MatchedInbound[]>(() => {
  if (!connUser.value) return []
  const gid = connUser.value.permission_group_id
  const user = connUser.value

  const srvMap = new Map<number, ServerItem>()
  for (const s of servers.value) srvMap.set(s.id, s)

  const inbMap = new Map<number, InboundItem>()
  for (const ib of inbounds.value) inbMap.set(ib.id, ib)

  const results: MatchedInbound[] = []
  for (const ap of accessPoints.value) {
    if (!ap.enabled) continue
    if (!ap.permission_group_ids?.includes(gid)) continue
    if (ap.target_type !== 'inbound' || !ap.target_inbound_id) continue

    const ib = inbMap.get(ap.target_inbound_id)
    if (!ib || !ib.enabled) continue
    if (ib.protocol !== 'mixed' && ib.protocol !== 'socks' && ib.protocol !== 'http') continue

    const srv = srvMap.get(ib.server_id)
    const host = ap.custom_host || ap.resolved_host || ib.share_addr || (srv ? srv.host : '127.0.0.1')
    const port = ap.custom_port || ap.resolved_port || ib.share_port || ib.port

    let isTls = false
    try {
      const ss = JSON.parse(ib.stream_settings || '{}')
      if (ss.security === 'tls') isTls = true
    } catch {}

    const socksScheme = isTls ? 'socks5s' : 'socks5'
    const httpScheme = isTls ? 'https' : 'http'
    const userPass = `${encodeURIComponent(user.username)}:${encodeURIComponent(user.password)}@${host}:${port}`
    const socksUrl = `${socksScheme}://${userPass}`
    const httpUrl = `${httpScheme}://${userPass}`

    if (ib.protocol === 'mixed') {
      results.push({
        id: ap.id,
        serverName: srv ? `${srv.name} (${ap.name})` : ap.name,
        apName: ap.name,
        protocol: 'MIXED',
        port,
        host,
        isMixed: true,
        socksUrl,
        httpUrl,
        connectionUrl: socksUrl,
      })
    } else {
      const proto = ib.protocol === 'socks' ? 'socks5' : 'http'
      const connUrl = ib.protocol === 'socks' ? socksUrl : httpUrl
      results.push({
        id: ap.id,
        serverName: srv ? `${srv.name} (${ap.name})` : ap.name,
        apName: ap.name,
        protocol: proto.toUpperCase(),
        port,
        host,
        isMixed: false,
        socksUrl,
        httpUrl,
        connectionUrl: connUrl,
      })
    }
  }

  return results
})

function openConnectionInfo(u: ForwardUserItem) {
  connUser.value = u
  connVisible.value = true
  Promise.all([getInbounds(), getAccessPoints()]).then(([iRes, aRes]) => {
    if (iRes.data.code === 0) inbounds.value = iRes.data.data.items
    if (aRes.data.code === 0) accessPoints.value = aRes.data.data.items
  })
}

// ===== 同步配置到服务器 =====
async function handleSyncConfigs() {
  try {
    await ElMessageBox.confirm(
      '将重新生成包含 SOCKS5 / HTTP 入站的服务器配置并推送到对应服务器（在线服务器将立即重启 Xray 使账号变更生效，离线服务器上线后自动补推）。确认立即应用？',
      '应用配置到服务器',
      {
        confirmButtonText: '立即应用',
        cancelButtonText: '取消',
        type: 'warning',
      },
    )
  } catch {
    return
  }

  syncingConfigs.value = true
  try {
    const res = await syncForwardUsersConfigs()
    if (res.data.code === 0) {
      ElMessage.success(`配置应用指令已下发（共 ${res.data.data.synced_servers} 台服务器）`)
    } else {
      ElMessage.error(res.data.message || '应用配置失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '应用配置失败'))
  } finally {
    syncingConfigs.value = false
  }
}
</script>

<template>
  <div class="channels-page x-page">
    <!-- 顶部概览指标舱 -->
    <div class="kpi-grid">
      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><User /></el-icon>
          <span>总转发用户</span>
        </div>
        <div class="kpi-val cell-mono">{{ summary.total_users }}</div>
        <div class="kpi-sub">系统内登记的全部转发账号</div>
      </BaseCard>

      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Connection /></el-icon>
          <span>活跃运行中</span>
        </div>
        <div class="kpi-val cell-mono is-active">{{ summary.active_users }}</div>
        <div class="kpi-sub">状态正常且处于开启状态</div>
      </BaseCard>

      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Coin /></el-icon>
          <span>本周期已用流量</span>
        </div>
        <div class="kpi-val cell-mono">{{ formatBytes(summary.total_used_bytes) }}</div>
        <div class="kpi-sub">各账号本周期实际产生流量总和</div>
      </BaseCard>

      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Timer /></el-icon>
          <span>独立总配额</span>
        </div>
        <div class="kpi-val cell-mono">
          {{ summary.total_limit_gb > 0 ? `${summary.total_limit_gb} GB` : '无限制' }}
        </div>
        <div class="kpi-sub">超额账号自动熔断剔除，不影响同节点其他用户</div>
      </BaseCard>
    </div>

    <!-- 筛选操作栏 -->
    <BaseCard class="filter-card">
      <div class="filter-bar">
        <div class="filter-left">
          <el-input
            v-model="filterKeyword"
            placeholder="搜索用户名或备注…"
            clearable
            style="width: 220px"
            :prefix-icon="Search"
            @keyup.enter="loadData"
            @clear="loadData"
          />

          <el-select
            v-model="filterGroupId"
            placeholder="所属权限组"
            clearable
            style="width: 170px"
            @change="loadData"
          >
            <el-option label="全部权限组" value="" />
            <el-option
              v-for="grp in permissionGroups"
              :key="grp.id"
              :label="grp.name"
              :value="grp.id"
            />
          </el-select>

          <el-select
            v-model="filterStatus"
            placeholder="账号状态"
            clearable
            style="width: 130px"
            @change="loadData"
          >
            <el-option label="全部状态" value="" />
            <el-option label="仅启用" value="enabled" />
            <el-option label="仅停用" value="disabled" />
          </el-select>

          <el-button :icon="Refresh" circle @click="reloadAll" title="刷新列表" />
        </div>

        <div class="filter-right" style="display: flex; gap: 8px">
          <el-button
            type="warning"
            plain
            :loading="syncingConfigs"
            @click="handleSyncConfigs"
          >
            <el-icon><Refresh /></el-icon>&nbsp;应用配置到服务器
          </el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">新建转发用户</el-button>
        </div>
      </div>
    </BaseCard>

    <!-- 用户卡片网格列表 -->
    <div v-loading="loading" class="user-list-wrapper">
      <div v-if="users.length === 0" class="empty-wrap">
        <el-empty description="暂无符合条件的转发用户">
          <el-button type="primary" :icon="Plus" @click="openCreate">新建转发用户</el-button>
        </el-empty>
      </div>

      <div v-else class="cards-grid">
        <BaseCard v-for="u in users" :key="u.id" class="user-card">
          <!-- 卡片顶栏 -->
          <div class="card-head">
            <div class="head-left">
              <span class="user-name cell-mono" :title="u.username">{{ u.username }}</span>
              <el-tag size="small" type="primary" effect="plain" class="group-tag">
                {{ u.group_name || `权限组 #${u.permission_group_id}` }}
              </el-tag>
            </div>
            <div class="head-right">
              <el-tag size="small" :type="calcStatus(u).type">
                {{ calcStatus(u).label }}
              </el-tag>
              <el-switch
                v-model="u.enabled"
                class="user-switch"
                inline-prompt
                @change="handleToggle(u)"
              />
            </div>
          </div>

          <!-- 卡片主要参数 -->
          <div class="card-body">
            <!-- 密码行 -->
            <div class="info-row">
              <span class="info-label">认证密码</span>
              <span class="info-val password-cell">
                <code class="cell-mono pass-text">
                  {{ showPasswordMap[u.id] ? u.password : '••••••••' }}
                </code>
                <el-button
                  type="primary"
                  link
                  size="small"
                  :icon="showPasswordMap[u.id] ? Hide : View"
                  title="查看/隐藏明文"
                  @click="togglePasswordVisibility(u.id)"
                />
                <el-button
                  type="primary"
                  link
                  size="small"
                  :icon="Key"
                  title="复制密码"
                  @click="copyText(u.password, '密码')"
                />
              </span>
            </div>

            <!-- 流量配额进度条 -->
            <div class="quota-box">
              <div class="quota-meta">
                <span class="quota-txt">
                  已用 <strong>{{ formatBytes(u.traffic_used_bytes) }}</strong>
                  <span class="quota-limit">
                    / {{ u.traffic_limit_gb > 0 ? `${u.traffic_limit_gb} GB` : '不限' }}
                  </span>
                </span>
                <span class="reset-badge">{{ resetCycleLabel(u.traffic_reset) }}</span>
              </div>
              <el-progress
                v-if="u.traffic_limit_gb > 0"
                :percentage="progressPercent(u)"
                :color="progressColor(progressPercent(u))"
                :stroke-width="6"
                :show-text="false"
              />
              <div v-else class="unlimited-bar" />
              <div class="quota-sub-info">
                <span>到期时间：{{ formatDateTime(u.expires_at, '永久有效') }}</span>
              </div>
            </div>

            <!-- 备注信息 -->
            <div v-if="u.remark" class="info-row remark-row">
              <span class="info-label">备注</span>
              <span class="info-val text-muted remark-text" :title="u.remark">{{ u.remark }}</span>
            </div>
          </div>

          <!-- 卡片底栏动作 -->
          <div class="card-footer">
            <el-button
              size="small"
              type="primary"
              plain
              :icon="Connection"
              @click="openConnectionInfo(u)"
            >
              连接信息
            </el-button>
            <el-button size="small" @click="handleResetTraffic(u)">清零流量</el-button>
            <div class="footer-right">
              <el-button size="small" :icon="Edit" circle @click="openEdit(u)" title="编辑用户" />
              <el-button
                size="small"
                type="danger"
                :icon="Delete"
                circle
                @click="handleDelete(u)"
                title="删除用户"
              />
            </div>
          </div>
        </BaseCard>
      </div>

      <!-- 分页栏 -->
      <div v-if="total > pageSize" class="pagination-bar">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="loadData"
          @size-change="loadData"
        />
      </div>
    </div>

    <!-- 新建 / 编辑弹窗 -->
    <el-dialog
      v-model="formVisible"
      :title="editingUser ? '编辑转发用户' : '新建转发用户'"
      width="540px"
      :append-to-body="true"
      destroy-on-close
    >
      <el-form label-width="100px" label-position="left">
        <el-form-item label="用户名" required>
          <el-input
            v-model="formData.username"
            :disabled="!!editingUser"
            placeholder="输入 SOCKS5 / HTTP 认证用户名"
            maxlength="64"
          />
          <div v-if="editingUser" class="form-tip">用户名为主键凭据，创建后不支持修改</div>
        </el-form-item>

        <el-form-item label="认证密码" required>
          <div class="pass-input-row">
            <el-input
              v-model="formData.password"
              placeholder="输入认证密码"
              maxlength="128"
            />
            <el-button :icon="MagicStick" @click="generateRandomPassword(16)" title="随机生成高强度密码">
              随机生成
            </el-button>
          </div>
        </el-form-item>

        <el-form-item label="所属权限组" required>
          <el-select
            v-model="formData.permission_group_id"
            placeholder="选择权限组"
            style="width: 100%"
          >
            <el-option
              v-for="grp in permissionGroups"
              :key="grp.id"
              :label="grp.name"
              :value="grp.id"
            />
          </el-select>
          <div class="form-tip">
            该用户账号将自动注入到系统中所有关联了此权限组的 SOCKS5 与 HTTP 入站中。
          </div>
        </el-form-item>

        <el-form-item label="流量配额">
          <div class="quota-input-row">
            <el-input-number
              v-model="formData.traffic_limit_gb"
              :min="0"
              :step="10"
              style="width: 160px"
            />
            <span class="unit-text">GB</span>
            <span class="form-tip" style="margin: 0 0 0 10px">设置为 0 表示不限制流量</span>
          </div>
        </el-form-item>

        <el-form-item label="重置周期">
          <el-radio-group v-model="formData.traffic_reset">
            <el-radio-button value="never">不重置</el-radio-button>
            <el-radio-button value="daily">每日</el-radio-button>
            <el-radio-button value="weekly">每周</el-radio-button>
            <el-radio-button value="monthly">每月</el-radio-button>
          </el-radio-group>
          <div class="form-tip">周期到达后流量清零；超额熔断账号将在流量清零后自动恢复可用。</div>
        </el-form-item>

        <el-form-item label="到期时间">
          <el-date-picker
            v-model="formData.expires_at"
            type="datetime"
            placeholder="选择到期时间（留空表示永久有效）"
            format="YYYY-MM-DD HH:mm:ss"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            clearable
            style="width: 100%"
          />
        </el-form-item>

        <el-form-item label="启用状态">
          <el-switch v-model="formData.enabled" />
          <span style="margin-left: 10px; font-size: 13px; color: var(--x-text-secondary)">
            {{ formData.enabled ? '启用（下发到节点入站）' : '禁用（从节点入站中剔除）' }}
          </span>
        </el-form-item>

        <el-form-item label="备注说明">
          <el-input
            v-model="formData.remark"
            type="textarea"
            :rows="2"
            placeholder="可选填写用户备注信息"
            maxlength="255"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <el-button @click="formVisible = false">取消</el-button>
          <el-button type="primary" :loading="formSaving" @click="handleFormSubmit">
            保存
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 连接信息弹窗 -->
    <el-dialog
      v-model="connVisible"
      :title="`转发连接信息 - ${connUser?.username || ''}`"
      width="680px"
      :append-to-body="true"
    >
      <div v-if="connUser" class="conn-modal-content">
        <!-- 用户凭据卡片 -->
        <div class="user-cred-bar">
          <div class="cred-item">
            <span class="cred-label">用户名：</span>
            <code class="cell-mono cred-val">{{ connUser.username }}</code>
            <el-button
              type="primary"
              link
              size="small"
              :icon="CopyDocument"
              @click="copyText(connUser.username, '用户名')"
            />
          </div>
          <div class="cred-item">
            <span class="cred-label">密码：</span>
            <code class="cell-mono cred-val">{{ connUser.password }}</code>
            <el-button
              type="primary"
              link
              size="small"
              :icon="CopyDocument"
              @click="copyText(connUser.password, '密码')"
            />
          </div>
          <div class="cred-item">
            <span class="cred-label">所属组：</span>
            <span class="cell-mono">{{ connUser.group_name || `权限组 #${connUser.permission_group_id}` }}</span>
          </div>
        </div>

        <div class="conn-section-title">
          <span>可用接入点（已向该权限组开放）</span>
          <TipIcon content="此账号已根据接入点权限组授权，自动注入至对应接入点指向的 Mixed 代理入站。客户端填写对应协议、服务器地址、端口及上述账密即可连接。" />
        </div>

        <div v-if="matchedInbounds.length === 0" class="conn-empty">
          <el-empty description="当前权限组暂无可用的 Mixed 代理接入点">
            <template #extra>
              <div style="font-size: 13px; color: var(--x-text-secondary); line-height: 1.6">
                请前往「接入点管理」或「拓扑画布」，为该组开放一个指向 Mixed 代理入站的接入点（授权该权限组并连接至对应入站）。
              </div>
            </template>
          </el-empty>
        </div>

        <div v-else class="conn-inbound-list">
          <div v-for="ib in matchedInbounds" :key="ib.id" class="conn-inbound-card">
            <div class="ib-card-head">
              <div class="ib-title">
                <el-tag size="small" :type="ib.protocol === 'MIXED' ? 'primary' : (ib.protocol === 'SOCKS5' ? 'success' : 'warning')" effect="dark">
                  {{ ib.protocol }}
                </el-tag>
                <span class="server-title">{{ ib.serverName }}</span>
              </div>
              <div class="ib-port cell-mono">端口: {{ ib.port }}</div>
            </div>

            <template v-if="ib.isMixed">
              <div class="ib-url-row">
                <span class="url-proto-tag">SOCKS5</span>
                <code class="cell-mono url-text">{{ ib.socksUrl }}</code>
                <el-button
                  type="primary"
                  size="small"
                  :icon="CopyDocument"
                  @click="copyText(ib.socksUrl!, 'SOCKS5 代理链接')"
                >
                  复制 SOCKS5
                </el-button>
              </div>
              <div class="ib-url-row" style="margin-top: 8px">
                <span class="url-proto-tag">HTTP</span>
                <code class="cell-mono url-text">{{ ib.httpUrl }}</code>
                <el-button
                  type="primary"
                  size="small"
                  :icon="CopyDocument"
                  @click="copyText(ib.httpUrl!, 'HTTP 代理链接')"
                >
                  复制 HTTP
                </el-button>
              </div>
            </template>
            <template v-else>
              <div class="ib-url-row">
                <code class="cell-mono url-text">{{ ib.connectionUrl }}</code>
                <el-button
                  type="primary"
                  size="small"
                  :icon="CopyDocument"
                  @click="copyText(ib.connectionUrl, `${ib.protocol} 代理链接`)"
                >
                  复制链接
                </el-button>
              </div>
            </template>

            <div class="ib-params-row">
              <div class="param-tag" @click="copyText(ib.host, '服务器地址')">
                <span class="p-k">地址:</span>
                <span class="p-v">{{ ib.host }}</span>
                <el-icon class="p-copy"><CopyDocument /></el-icon>
              </div>
              <div class="param-tag" @click="copyText(String(ib.port), '端口')">
                <span class="p-k">端口:</span>
                <span class="p-v">{{ ib.port }}</span>
                <el-icon class="p-copy"><CopyDocument /></el-icon>
              </div>
              <div class="param-tag" @click="copyText(connUser.username, '用户名')">
                <span class="p-k">用户:</span>
                <span class="p-v">{{ connUser.username }}</span>
                <el-icon class="p-copy"><CopyDocument /></el-icon>
              </div>
              <div class="param-tag" @click="copyText(connUser.password, '密码')">
                <span class="p-k">密码:</span>
                <span class="p-v">{{ connUser.password }}</span>
                <el-icon class="p-copy"><CopyDocument /></el-icon>
              </div>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <el-button @click="connVisible = false">关闭</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.channels-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 顶部概览 */
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px;
}

.kpi-card {
  padding: 16px 20px;
}

.kpi-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--x-text-secondary);
}

.kpi-val {
  font-size: 26px;
  font-weight: 700;
  margin: 8px 0 4px;
  color: var(--x-text-primary);
}

.kpi-val.is-active {
  color: #67c23a;
}

.kpi-sub {
  font-size: 12px;
  color: var(--x-text-placeholder);
}

/* 过滤栏 */
.filter-card {
  padding: 12px 18px;
}

.filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}

.filter-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

/* 卡片网格 */
.user-list-wrapper {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.empty-wrap {
  padding: 40px 0;
  background: var(--x-card-bg, #ffffff);
  border-radius: 8px;
  border: 1px dashed var(--x-border-color, #e4e7ed);
}

.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 16px;
}

.user-card {
  display: flex;
  flex-direction: column;
  padding: 16px 18px;
  border-radius: 8px;
  transition: transform 0.2s, box-shadow 0.2s;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.08);
  }
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 12px;
  border-bottom: 1px dashed var(--x-border-light, #f0f2f5);

  .head-left {
    display: flex;
    align-items: center;
    gap: 8px;
    overflow: hidden;
  }

  .user-name {
    font-size: 15px;
    font-weight: 700;
    color: var(--x-text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 160px;
  }

  .group-tag {
    font-size: 11px;
    border-radius: 4px;
  }

  .head-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 0;
  flex: 1;

  .info-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 13px;

    .info-label {
      color: var(--x-text-secondary);
      flex-shrink: 0;
    }

    .info-val {
      display: flex;
      align-items: center;
      gap: 4px;
    }
  }

  .password-cell {
    .pass-text {
      background: var(--x-fill-light, #f5f7fa);
      padding: 2px 6px;
      border-radius: 4px;
      font-size: 12px;
    }
  }

  .remark-row {
    .remark-text {
      font-size: 12px;
      color: var(--x-text-placeholder);
      max-width: 220px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }
}

/* 流量配额框 */
.quota-box {
  background: var(--x-fill-lighter, #fafafa);
  border-radius: 6px;
  padding: 10px 12px;
  margin-top: 4px;

  .quota-meta {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 6px;

    .quota-txt {
      font-size: 13px;
      color: var(--x-text-regular);

      strong {
        color: var(--x-text-primary);
        font-weight: 600;
      }

      .quota-limit {
        color: var(--x-text-secondary);
        font-size: 12px;
      }
    }

    .reset-badge {
      font-size: 11px;
      padding: 1px 6px;
      border-radius: 3px;
      background: var(--x-border-light, #f0f2f5);
      color: var(--x-text-secondary);
    }
  }

  .unlimited-bar {
    height: 6px;
    border-radius: 3px;
    background: #e1f3d8;
  }

  .quota-sub-info {
    font-size: 11.5px;
    color: var(--x-text-placeholder);
    margin-top: 6px;
  }
}

/* 卡片底栏 */
.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 10px;
  border-top: 1px solid var(--x-border-light, #f0f2f5);

  .footer-right {
    display: flex;
    align-items: center;
    gap: 6px;
  }
}

/* 分页 */
.pagination-bar {
  display: flex;
  justify-content: flex-end;
  padding: 8px 0;
}

/* 弹窗样式 */
.pass-input-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.quota-input-row {
  display: flex;
  align-items: center;
  width: 100%;

  .unit-text {
    margin-left: 8px;
    font-size: 13px;
    color: var(--x-text-secondary);
  }
}

.form-tip {
  font-size: 12px;
  color: var(--x-text-placeholder);
  line-height: 1.5;
  margin-top: 4px;
}

/* 连接信息抽屉/弹窗 */
.conn-modal-content {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.user-cred-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  background: var(--x-fill-lighter, #fafafa);
  border: 1px solid var(--x-border-light, #ebeef5);
  border-radius: 6px;
  padding: 10px 14px;

  .cred-item {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;

    .cred-label {
      color: var(--x-text-secondary);
    }

    .cred-val {
      font-weight: 600;
      color: var(--x-primary, #409eff);
      background: var(--x-fill-light, #f5f7fa);
      padding: 1px 6px;
      border-radius: 4px;
    }
  }
}

.conn-section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text-primary);
  margin-top: 4px;
}

.conn-empty {
  padding: 20px 0;
}

.conn-inbound-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 420px;
  overflow-y: auto;
}

.conn-inbound-card {
  border: 1px solid var(--x-border-light, #ebeef5);
  border-radius: 6px;
  padding: 12px 14px;
  background: var(--x-card-bg, #ffffff);
  display: flex;
  flex-direction: column;
  gap: 8px;

  .ib-card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;

    .ib-title {
      display: flex;
      align-items: center;
      gap: 8px;

      .server-title {
        font-weight: 600;
        font-size: 14px;
      }
    }

    .ib-port {
      font-size: 12px;
      color: var(--x-text-secondary);
    }
  }

  .ib-url-row {
    display: flex;
    align-items: center;
    gap: 8px;

    .url-proto-tag {
      font-size: 11px;
      font-weight: 700;
      padding: 3px 6px;
      border-radius: 4px;
      background: var(--el-color-primary-light-9, #ecf5ff);
      color: var(--el-color-primary, #409eff);
      border: 1px solid var(--el-color-primary-light-7, #d9ecff);
      letter-spacing: 0.5px;
      flex-shrink: 0;
    }

    .url-text {
      flex: 1;
      background: var(--x-fill-light, #f5f7fa);
      border: 1px solid var(--x-border-light, #ebeef5);
      padding: 6px 10px;
      border-radius: 4px;
      font-size: 12px;
      color: var(--x-text-regular);
      overflow-x: auto;
      white-space: nowrap;
    }
  }

  .ib-params-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;

    .param-tag {
      display: flex;
      align-items: center;
      gap: 4px;
      background: var(--x-fill-lighter, #fafafa);
      border: 1px solid var(--x-border-light, #ebeef5);
      border-radius: 4px;
      padding: 3px 8px;
      font-size: 11.5px;
      cursor: pointer;
      transition: background 0.15s, border-color 0.15s;

      &:hover {
        background: var(--x-fill-light, #f0f2f5);
        border-color: var(--x-primary-light-7, #c6e2ff);
        color: var(--x-primary, #409eff);
      }

      .p-k {
        color: var(--x-text-secondary);
      }

      .p-v {
        font-family: monospace;
        font-weight: 600;
      }

      .p-copy {
        font-size: 12px;
        opacity: 0.7;
      }
    }
  }
}
</style>
