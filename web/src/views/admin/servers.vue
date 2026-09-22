<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Search, Refresh, View, Document, Delete, Key, CopyDocument, Edit, Setting, RefreshRight, TrendCharts, Upload, Download, MoreFilled, Loading, Check, Close, Platform, Link } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import TipIcon from '@/components/base/TipIcon.vue'
import ServerNodeDrawer from './servers/ServerNodeDrawer.vue'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'
import {
  createServer,
  deleteServer,
  getInbounds,
  getServers,
  resetServerSecret,
  serverCommand,
  upgradeAgent,
  getAgentUpgradeStatus,
  batchUpgradeServers,
  getBatchUpgradeStatus,
  type AgentUpgradeStatus,
  getAgentLatestVersion,
  type CommandResult,
  type ServerItem,
  updateServer,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { compareVersion } from '@/utils/version'
import { formatDateTime } from '@/utils/timezone'
import { getExpiryStatus, formatExpireDate, normalizeUrl, isUrl } from '@/utils/vps'

const router = useRouter()

const list = ref<ServerItem[]>([])
const loading = ref(false)
const keyword = ref('')
const filterExpiring = ref(false)

const metricsOpen = ref(false)
const metricsServer = ref<ServerItem | null>(null)

function openMetrics(row: any) {
  metricsServer.value = row
  metricsOpen.value = true
}

// 接入点计数：server_id → 入站数（服务器页摘要，跳转节点页按服务器过滤）
const inboundCountMap = ref<Record<number, number>>({})

async function loadInboundCounts() {
  try {
    const { data } = await getInbounds()
    if (data.code === 0) {
      const m: Record<number, number> = {}
      for (const ib of data.data.items) m[ib.server_id] = (m[ib.server_id] ?? 0) + 1
      inboundCountMap.value = m
    }
  } catch {
    /* 计数失败不阻塞列表 */
  }
}

function inboundCount(id: number) {
  return inboundCountMap.value[id] ?? 0
}

function goInbounds(row: any) {
  router.push({ path: '/admin/nodes', query: { server_id: row.id } })
}

const expiringServers = computed(() => {
  return list.value.filter((s) => {
    const st = getExpiryStatus(s.expire_at)
    return st && st.isUrgent
  })
})

const filtered = computed(() => {
  let res = list.value
  if (filterExpiring.value) {
    res = res.filter((s) => {
      const st = getExpiryStatus(s.expire_at)
      return st && st.isUrgent
    })
  }
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return res
  return res.filter(
    (s) =>
      s.name.toLowerCase().includes(kw) ||
      s.host.toLowerCase().includes(kw) ||
      (s.location ?? '').toLowerCase().includes(kw) ||
      (s.billing_cycle ?? '').toLowerCase().includes(kw) ||
      (s.price ?? '').toLowerCase().includes(kw) ||
      (s.idc_address ?? '').toLowerCase().includes(kw),
  )
})

// ---- 官方 Agent 最新版本感知与比对 ----
const latestAgentVersion = ref('')
const fetchingLatestAgent = ref(false)

async function loadLatestAgentVersion(refresh = false) {
  fetchingLatestAgent.value = true
  try {
    const { data } = await getAgentLatestVersion(refresh)
    if (data.code === 0 && data.data?.latest_version) {
      latestAgentVersion.value = data.data.latest_version
    }
  } catch {
    // 静默降级
  } finally {
    fetchingLatestAgent.value = false
  }
}

function getAgentVersionStatus(currentVer: string) {
  if (!currentVer) return { type: 'unknown', label: '未知' }
  if (!latestAgentVersion.value) return { type: 'normal', label: '' }
  const cmp = compareVersion(currentVer, latestAgentVersion.value)
  if (cmp < 0) {
    return { type: 'outdated', label: `有新版 ${latestAgentVersion.value}` }
  }
  return { type: 'latest', label: '最新' }
}

async function load() {
  loading.value = true
  try {
    const { data } = await getServers()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载服务器失败'))
  } finally {
    loading.value = false
  }
  loadInboundCounts()
  loadLatestAgentVersion()
}
onMounted(load)

function fmtTime(t: string | null) {
  if (!t) return '—'
  return formatDateTime(t, '—')
}

// xray 状态中文（节点心跳/状态查询回传的 xray_state；旧 agent 为空由调用处跳过该行）
function xrayStateText(state: string) {
  switch (state) {
    case 'running':
      return '运行中'
    case 'restarting':
      return '启动失败，自动重试中'
    case 'failed':
      return '启动失败，已停止自动拉起'
    case 'stopped':
      return '未启动'
    default:
      return state
  }
}

// 列表里的 xray 异常徽标：failed/restarting 才显示（正常态不加噪音）。
// 配色与节点抽屉一致（failed 红、restarting 橙）；节点离线时状态是"离线前最后一次上报"，
// 显式加「上次」前缀并在提示里说明，避免被当成实时状态。
function xrayAlertChip(row: any) {
  const s = row?.xray_state
  const stale = row?.status !== 1
  if (s !== 'failed' && s !== 'restarting') return { show: false, cls: '', text: '', tip: '' }
  const reason = row.xray_last_error || '原因未知'
  return {
    show: true,
    cls: s === 'failed' ? 'red' : 'orange',
    text: `${stale ? '上次 ' : ''}${s === 'failed' ? 'Xray 启动失败' : 'Xray 重试中'}`,
    tip: `xray ${s === 'failed' ? '连续启动失败，已停止自动拉起' : '启动失败，自动重试中'}：${reason}${
      stale ? '（服务器离线，此为离线前最后状态）' : ''
    }`,
  }
}

// pushChip 配置同步状态芯片（四态 + 磁盘偏离）。状态由后端 push_state 给出，前端只做呈现。
function pushChip(row: any) {
  const st = row?.push_state || (row?.config_status === 'pushed' ? 'synced' : row?.config_status === 'pending' ? 'pending' : 'none')
  switch (st) {
    case 'drift':
      return {
        cls: 'red',
        text: '磁盘偏离',
        tip: `节点磁盘上的配置与主控记录不一致（节点哈希 ${(row.xray_disk_hash || '').slice(0, 12) || '未上报'}…）。可能被手工改过、落盘失败或节点回退过配置，重新生成并推送一次即可对齐。`,
      }
    case 'rejected':
      return { cls: 'red', text: '推送被拒', tip: `节点拒绝该配置：${row.push_error || '原因未知'}（已尝试 ${row.push_attempts || 0} 次）` }
    case 'synced':
      return { cls: 'green', text: '已同步', tip: '' }
    case 'pending':
      return { cls: 'orange', text: '待推送', tip: row.push_error ? `尚未应用：${row.push_error}（已尝试 ${row.push_attempts || 0} 次）` : '' }
    default:
      return { cls: 'gray', text: '未投递', tip: '' }
  }
}

// ---- 新增服务器 ----
const createOpen = ref(false)
const createForm = reactive({
  server_type: 'xray' as 'xray',
  name: '',
  host: '',
  location: '',
  remark: '',
  expire_at: '' as string | null,
  billing_cycle: '',
  price: '',
  idc_address: '',
})
const creating = ref(false)
const createdResult = ref<{ node_id: string; secret: string; install_cmd: string } | null>(null)

async function submitCreate() {
  if (!createForm.name || !createForm.host) {
    ElMessage.warning('请填写名称与地址')
    return
  }
  creating.value = true
  try {
    const { data } = await createServer({
      server_type: createForm.server_type,
      name: createForm.name,
      host: createForm.host,
      location: createForm.location,
      remark: createForm.remark,
      expire_at: createForm.expire_at || null,
      billing_cycle: createForm.billing_cycle,
      price: createForm.price,
      idc_address: createForm.idc_address,
    })
    if (data.code === 0) {
      createdResult.value = { node_id: data.data.node_id, secret: data.data.secret, install_cmd: data.data.install_cmd }
      createForm.name = ''
      createForm.host = ''
      createForm.location = ''
      createForm.remark = ''
      createForm.expire_at = null
      createForm.billing_cycle = ''
      createForm.price = ''
      createForm.idc_address = ''
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建失败'))
  } finally {
    creating.value = false
  }
}

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

function closeCreate() {
  createOpen.value = false
  createdResult.value = null
  createForm.name = ''
  createForm.host = ''
  createForm.location = ''
  createForm.remark = ''
  createForm.expire_at = null
  createForm.billing_cycle = ''
  createForm.price = ''
  createForm.idc_address = ''
}

// ---- 节点管理抽屉 ----
const drawerOpen = ref(false)
const drawerServer = ref<ServerItem | null>(null)

function openDrawer(row: any) {
  drawerServer.value = row
  drawerOpen.value = true
}

// ---- 状态详情 ----
const statusOpen = ref(false)
const statusData = ref<CommandResult<any> | null>(null)
const statusLoading = ref(false)

async function openStatus(row: any) {
  statusOpen.value = true
  statusData.value = null
  statusLoading.value = true
  try {
    const { data } = await serverCommand(row.id, 'get_status')
    statusData.value = data.data
  } catch (e) {
    ElMessage.error(errMsg(e, '查询状态失败'))
  } finally {
    statusLoading.value = false
  }
}

// ---- 重启 ----
async function restartXray(row: any) {
  try {
    await ElMessageBox.confirm(`确认重启服务器「${row.name}」的 Xray？约 1-2 秒断线。`, '重启 Xray', {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const { data } = await serverCommand(row.id, 'restart_xray')
    if (data.code === 0 && data.data.ok) ElMessage.success('已重启')
    else ElMessage.error(data.data.error || '重启失败')
  } catch (e) {
    ElMessage.error(errMsg(e, '重启失败'))
  }
}

// ---- Agent 升级监控 ----
const upgradingId = ref(0)
const upgradeModalOpen = ref(false)
const upgradeTarget = ref<ServerItem | null>(null)
const upgradeStatus = ref<AgentUpgradeStatus | null>(null)

const upgradeActiveStep = computed(() => {
  if (!upgradeStatus.value) return 0
  switch (upgradeStatus.value.phase) {
    case 'starting':
    case 'checking':
      return 0
    case 'downloading':
      return 1
    case 'verifying':
      return 2
    case 'replacing':
    case 'restarting':
      return 3
    case 'success':
      return 4
    case 'failed':
      return 1
    default:
      return 0
  }
})

let upgradeTimer: any = null

function stopUpgradePolling() {
  if (upgradeTimer) {
    clearInterval(upgradeTimer)
    upgradeTimer = null
  }
}

onUnmounted(stopUpgradePolling)

async function pollUpgradeStatus(serverId: number) {
  try {
    const { data } = await getAgentUpgradeStatus(serverId)
    if (data?.data?.status) {
      upgradeStatus.value = data.data.status
      if (data.data.status.phase === 'success') {
        stopUpgradePolling()
        load()
      } else if (data.data.status.phase === 'failed') {
        stopUpgradePolling()
      }
    }
  } catch {
    // 忽略轮询抖动
  }
}

const upgradeIsRollback = ref(false)
const rollbackTargetVersion = ref('')

async function upgradeNodeAgent(row: any) {
  upgradeIsRollback.value = false
  rollbackTargetVersion.value = ''
  const currentVer = row.agent_version || ''
  const latest = latestAgentVersion.value
  let msg = ''
  let isAlreadyLatest = false

  if (latest && currentVer) {
    const cmp = compareVersion(currentVer, latest)
    if (cmp >= 0) {
      isAlreadyLatest = true
      msg = `服务器「${row.name}」当前 Agent 版本（${currentVer}）已是官方最新版本（${latest}）。\n\n是否仍要重新拉取并覆盖安装？`
    } else {
      msg = `检测到官方最新版本 Agent：\n- 当前服务器版本：${currentVer}\n- 官方最新版本：${latest}\n\n确认将服务器「${row.name}」升级至最新版？（sha256 校验，完成后服务器自动重启，期间短暂离线）`
    }
  } else {
    msg = `将从 GitHub Releases 下载最新版 Agent 并在服务器「${row.name}」上升级（sha256 校验），完成后服务器自动重启，期间短暂离线。当前版本：${currentVer || '未知'}`
  }

  try {
    await ElMessageBox.confirm(
      msg,
      isAlreadyLatest ? '重新安装 Agent' : '升级 Agent',
      {
        type: isAlreadyLatest ? 'info' : 'warning',
        confirmButtonText: isAlreadyLatest ? '重新安装' : '立即升级',
        cancelButtonText: '取消',
      },
    )
  } catch {
    return
  }

  upgradeTarget.value = row
  upgradeStatus.value = {
    phase: 'starting',
    target: latest || undefined,
    message: '正在向服务器下发自升级指令...',
    ts: Math.floor(Date.now() / 1000),
  }
  upgradeModalOpen.value = true
  upgradingId.value = row.id

  stopUpgradePolling()
  upgradeTimer = setInterval(() => {
    pollUpgradeStatus(row.id)
  }, 1500)

  try {
    const { data } = await upgradeAgent(row.id, {
      target: latest || undefined,
      force: isAlreadyLatest,
    })
    if (data.code === 0 && data.data.ok) {
      upgradeStatus.value = {
        phase: 'success',
        target: latest || undefined,
        message: (data.data.data as string) || '已升级完成，服务器已重新加载新版本',
        ts: Math.floor(Date.now() / 1000),
      }
      stopUpgradePolling()
      load()
      setTimeout(load, 15000)
    } else {
      upgradeStatus.value = {
        phase: 'failed',
        target: latest || undefined,
        message: '升级失败',
        error: data.data?.error || data.message || '升级失败',
        ts: Math.floor(Date.now() / 1000),
      }
      stopUpgradePolling()
    }
  } catch (e) {
    upgradeStatus.value = {
      phase: 'failed',
      target: latest || undefined,
      message: '升级请求超时或网络异常',
      error: errMsg(e, '升级失败（服务器可能仍在后台下载，可稍后刷新查看版本）'),
      ts: Math.floor(Date.now() / 1000),
    }
    stopUpgradePolling()
  } finally {
    upgradingId.value = 0
  }
}

async function rollbackNodeAgent(row: any) {
  const currentVer = row.agent_version || ''
  try {
    const { value } = await ElMessageBox.prompt(
      `当前服务器「${row.name}」Agent 版本为：${currentVer || '未知'}\n\n请输入要回滚的目标版本号（如 v0.1.15）：`,
      '回滚 Agent',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputPattern: /^v?\d+(\.\d+)+.*$/,
        inputErrorMessage: '版本号格式不正确（示例：v0.1.15）',
      },
    )
    let target = (value || '').trim()
    if (!target) return
    if (!target.startsWith('v')) target = 'v' + target
    if (currentVer && (currentVer === target || compareVersion(currentVer, target) === 0)) {
      ElMessage.info(`服务器当前已是版本 ${target}，无需回滚`)
      return
    }

    try {
      await ElMessageBox.confirm(
        `确认将服务器「${row.name}」的 Agent 版本从 ${currentVer || '当前版本'} 回滚至 ${target}？\n\n（将下载旧版本并执行 sha256 完整性校验，完成后服务器自动重启，期间短暂离线）`,
        '确认回滚 Agent',
        {
          type: 'warning',
          confirmButtonText: '立即回滚',
          cancelButtonText: '取消',
        },
      )
    } catch {
      return
    }

    upgradeIsRollback.value = true
    rollbackTargetVersion.value = target
    upgradeTarget.value = row
    upgradeStatus.value = {
      phase: 'starting',
      target,
      message: '正在向服务器下发回滚指令...',
      ts: Math.floor(Date.now() / 1000),
    }
    upgradeModalOpen.value = true
    upgradingId.value = row.id

    stopUpgradePolling()
    upgradeTimer = setInterval(() => {
      pollUpgradeStatus(row.id)
    }, 1500)

    const { data } = await upgradeAgent(row.id, {
      target,
      force: true,
    })
    if (data.code === 0 && data.data.ok) {
      upgradeStatus.value = {
        phase: 'success',
        target,
        message: (data.data.data as string) || '已回滚完成，服务器已重新加载旧版本',
        ts: Math.floor(Date.now() / 1000),
      }
      stopUpgradePolling()
      load()
      setTimeout(load, 15000)
    } else {
      upgradeStatus.value = {
        phase: 'failed',
        target,
        message: '回滚失败',
        error: data.data?.error || data.message || '回滚失败',
        ts: Math.floor(Date.now() / 1000),
      }
      stopUpgradePolling()
    }
  } catch (e: any) {
    if (e === 'cancel' || e?.action === 'cancel') return
    upgradeStatus.value = {
      phase: 'failed',
      target: rollbackTargetVersion.value || undefined,
      message: '回滚请求超时或网络异常',
      error: errMsg(e, '回滚失败（服务器可能仍在后台下载，可稍后刷新查看版本）'),
      ts: Math.floor(Date.now() / 1000),
    }
    stopUpgradePolling()
  } finally {
    upgradingId.value = 0
  }
}

// ---- 批量升级 Agent ----
const selectedIds = ref<number[]>([])
const allSelected = computed(
  () => filtered.value.length > 0 && filtered.value.every((s) => selectedIds.value.includes(s.id)),
)

function toggleSelect(id: number, val: boolean) {
  if (val) {
    if (!selectedIds.value.includes(id)) selectedIds.value = [...selectedIds.value, id]
  } else {
    selectedIds.value = selectedIds.value.filter((i) => i !== id)
  }
}

function toggleSelectAll(val: boolean) {
  selectedIds.value = val ? filtered.value.map((s) => s.id) : []
}

// phase → 展示文案 / 进度百分比（批量总览行内进度条）
const phaseMeta: Record<string, { label: string; percent: number }> = {
  starting: { label: '排队中', percent: 5 },
  checking: { label: '检查版本', percent: 15 },
  downloading: { label: '下载资源', percent: 50 },
  verifying: { label: '校验完整性', percent: 70 },
  replacing: { label: '替换二进制', percent: 85 },
  restarting: { label: '重启生效', percent: 95 },
  success: { label: '升级成功', percent: 100 },
  failed: { label: '升级失败', percent: 100 },
}

interface BatchRow {
  id: number
  name: string
  version: string
  skipped: boolean
  reason?: string
  status: AgentUpgradeStatus | null
}
const batchOpen = ref(false)
const batchRunning = ref(false)
const batchRows = ref<BatchRow[]>([])
let batchTimer: any = null

function stopBatchPolling() {
  if (batchTimer) {
    clearInterval(batchTimer)
    batchTimer = null
  }
}

onUnmounted(stopBatchPolling)

function progressStatus(phase?: string) {
  if (phase === 'success') return 'success' as const
  if (phase === 'failed') return 'exception' as const
  return undefined
}

async function pollBatch() {
  const activeIds = batchRows.value.filter((r) => !r.skipped).map((r) => r.id)
  if (!activeIds.length) {
    stopBatchPolling()
    batchRunning.value = false
    return
  }
  try {
    const { data } = await getBatchUpgradeStatus(activeIds)
    if (data.code === 0) {
      const statuses = data.data.statuses || {}
      for (const row of batchRows.value) {
        if (row.skipped) continue
        const st = statuses[String(row.id)]
        if (st) row.status = st
      }
      const allDone = batchRows.value
        .filter((r) => !r.skipped)
        .every((r) => r.status && (r.status.phase === 'success' || r.status.phase === 'failed'))
      if (allDone) {
        stopBatchPolling()
        batchRunning.value = false
        load()
        setTimeout(load, 15000)
      }
    }
  } catch {
    // 忽略轮询抖动
  }
}

async function batchUpgradeSelected() {
  const selected = list.value.filter((s) => selectedIds.value.includes(s.id))
  if (!selected.length) return
  const latest = latestAgentVersion.value
  const outdated = selected.filter((s) => latest && s.agent_version && compareVersion(s.agent_version, latest) < 0)
  const unknown = selected.filter((s) => !s.agent_version)
  const upToDate = selected.length - outdated.length - unknown.length
  const lines = [
    `已选 ${selected.length} 台服务器，目标版本：${latest || '官方最新（提交时在线获取）'}。`,
    `可升级 ${outdated.length + unknown.length} 台（${outdated.length} 台有新版${unknown.length ? `、${unknown.length} 台版本未知` : ''}）${upToDate ? `；已是最新 ${upToDate} 台将自动跳过` : ''}。`,
    '离线服务器将自动跳过；升级并行执行（sha256 校验，完成后自动重启，期间短暂离线）。',
  ]
  try {
    await ElMessageBox.confirm(lines.join('\n'), '批量升级 Agent', {
      type: 'warning',
      confirmButtonText: '开始升级',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }

  batchRows.value = selected.map((s) => ({
    id: s.id,
    name: s.name,
    version: s.agent_version || '未知',
    skipped: false,
    status: { phase: 'starting', message: '提交中…', ts: Math.floor(Date.now() / 1000) } as AgentUpgradeStatus,
  }))
  batchOpen.value = true
  batchRunning.value = true
  stopBatchPolling()
  batchTimer = setInterval(pollBatch, 1500)

  try {
    const { data } = await batchUpgradeServers({
      ids: selected.map((s) => s.id),
      target: latest || undefined,
      force: false,
    })
    if (data.code !== 0) {
      ElMessage.error(data.message)
      stopBatchPolling()
      batchRunning.value = false
      batchOpen.value = false
      return
    }
    const skippedMap = new Map(data.data.skipped.map((s) => [s.id, s.reason]))
    const dispatchedSet = new Set(data.data.dispatched.map((d) => d.id))
    for (const row of batchRows.value) {
      if (skippedMap.has(row.id)) {
        row.skipped = true
        row.reason = skippedMap.get(row.id)
        row.status = null
      } else if (!dispatchedSet.has(row.id)) {
        row.skipped = true
        row.reason = '未派发'
        row.status = null
      }
    }
    pollBatch()
  } catch (e) {
    stopBatchPolling()
    batchRunning.value = false
    batchOpen.value = false
    ElMessage.error(errMsg(e, '批量升级提交失败'))
  }
}

// ---- 日志 ----
const logOpen = ref(false)
const logContent = ref('')
const logLoading = ref(false)
const logTarget = ref<ServerItem | null>(null)
const logLines = ref(50)
const logReverse = ref(false)
const logPreRef = ref<HTMLPreElement | null>(null)

const displayedLogContent = computed(() => {
  if (!logContent.value) return '（空）'
  if (!logReverse.value) return logContent.value
  const lines = logContent.value.split('\n')
  return lines.slice().reverse().join('\n')
})

async function scrollLogToBottom() {
  await nextTick()
  if (logPreRef.value && !logReverse.value) {
    logPreRef.value.scrollTop = logPreRef.value.scrollHeight
  }
}

async function fetchLogs() {
  if (!logTarget.value) return
  logLoading.value = true
  try {
    const { data } = await serverCommand(logTarget.value.id, 'get_logs', { lines: logLines.value })
    logContent.value = (data.data.data as string) || ''
    await scrollLogToBottom()
  } catch (e) {
    logContent.value = `读取失败：${errMsg(e)}`
  } finally {
    logLoading.value = false
  }
}

async function openLogs(row: any) {
  logTarget.value = row
  logOpen.value = true
  logContent.value = ''
  await fetchLogs()
}

function copyLogs() {
  if (!logContent.value) return
  navigator.clipboard.writeText(displayedLogContent.value)
    .then(() => ElMessage.success('日志已复制到剪贴板'))
    .catch(() => ElMessage.error('复制失败'))
}

// ---- 编辑服务器 ----
const editOpen = ref(false)
const editForm = reactive({
  id: 0,
  server_type: 'xray' as 'xray',
  name: '',
  host: '',
  location: '',
  remark: '',
  expire_at: '' as string | null,
  billing_cycle: '',
  price: '',
  idc_address: '',
})
const editSaving = ref(false)

function openEdit(row: any) {
  Object.assign(editForm, {
    id: row.id,
    server_type: 'xray',
    name: row.name,
    host: row.host,
    location: row.location ?? '',
    remark: row.remark ?? '',
    expire_at: row.expire_at || null,
    billing_cycle: row.billing_cycle ?? '',
    price: row.price ?? '',
    idc_address: row.idc_address ?? '',
  })
  editOpen.value = true
}

async function submitEdit() {
  if (!editForm.name || !editForm.host) {
    ElMessage.warning('请填写名称与地址')
    return
  }
  editSaving.value = true
  try {
    const { data } = await updateServer(editForm.id, {
      server_type: editForm.server_type,
      name: editForm.name,
      host: editForm.host,
      location: editForm.location,
      remark: editForm.remark,
      expire_at: editForm.expire_at || null,
      billing_cycle: editForm.billing_cycle,
      price: editForm.price,
      idc_address: editForm.idc_address,
    })
    if (data.code === 0) {
      ElMessage.success('已保存')
      editOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    editSaving.value = false
  }
}

// ---- 重置密钥 ----
const secretOpen = ref(false)
const secretInfo = ref<{ node_id: string; secret: string; install_cmd?: string } | null>(null)

async function resetSecret(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认重置服务器「${row.name}」的密钥？旧密钥立即失效，需在服务器 Agent 配置（/etc/xray-agent/config.yml）中更新。`,
      '重置密钥',
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    const { data } = await resetServerSecret(row.id)
    if (data.code === 0) {
      secretInfo.value = { node_id: data.data.node_id, secret: data.data.secret, install_cmd: data.data.install_cmd }
      secretOpen.value = true
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  }
}

// ---- 更多操作（编辑/重置密钥/删除/状态/日志/升级/重启/打开 IDC） ----
function onMore(cmd: string, row: any) {
  if (cmd === 'edit') openEdit(row)
  else if (cmd === 'reset') resetSecret(row)
  else if (cmd === 'delete') removeServer(row)
  else if (cmd === 'status') openStatus(row)
  else if (cmd === 'restart') restartXray(row)
  else if (cmd === 'logs') openLogs(row)
  else if (cmd === 'upgrade') upgradeNodeAgent(row)
  else if (cmd === 'rollback') rollbackNodeAgent(row)
  else if (cmd === 'open_idc') {
    if (row.idc_address) window.open(normalizeUrl(row.idc_address), '_blank')
  }
}

// ---- 删除 ----
async function removeServer(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除服务器「${row.name}」？其关联的入站、待推送配置与上报数据将一并删除，该服务器的 Agent 将无法再连接。`, '删除服务器', {
      type: 'error',
    })
  } catch {
    return
  }
  try {
    const { data } = await deleteServer(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input v-model="keyword" placeholder="搜索名称 / 地址 / 地区" :prefix-icon="Search" clearable style="width: 240px" />
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
        <el-checkbox
          :model-value="allSelected"
          :disabled="filtered.length === 0"
          @change="(v: any) => toggleSelectAll(!!v)"
        >全选</el-checkbox>
        <template v-if="selectedIds.length">
          <span class="x-chip blue" style="font-size: 11px">已选 {{ selectedIds.length }} 台</span>
          <el-button type="warning" plain @click="batchUpgradeSelected">
            <el-icon><Upload /></el-icon>&nbsp;批量升级
          </el-button>
          <el-button link @click="selectedIds = []">取消选择</el-button>
        </template>
        <el-tag
          v-if="latestAgentVersion"
          size="default"
          type="info"
          class="cell-mono"
          style="cursor: pointer; height: 32px; display: inline-flex; align-items: center"
          title="官方最新 Agent 版本，点击刷新最新版本检测"
          @click="loadLatestAgentVersion(true)"
        >
          官方最新: {{ latestAgentVersion }}
        </el-tag>
      </div>
      <el-button type="primary" @click="createOpen = true"><el-icon><Plus /></el-icon>&nbsp;新增服务器</el-button>
    </div>

    <!-- 续费提醒横幅 -->
    <el-alert
      v-if="expiringServers.length > 0"
      type="warning"
      show-icon
      :closable="false"
      style="margin-bottom: 14px"
    >
      <template #title>
        <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px">
          <span>
            续费提醒：共 <strong>{{ expiringServers.length }}</strong> 台服务器已到期或将在 7 天内到期，请注意及时前往 IDC 控制台续费。
          </span>
          <el-button
            size="small"
            :type="filterExpiring ? 'warning' : 'default'"
            round
            @click="filterExpiring = !filterExpiring"
          >
            {{ filterExpiring ? '查看全部服务器' : '仅看待续费服务器' }}
          </el-button>
        </div>
      </template>
    </el-alert>

    <BaseCard title="服务器列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="filtered.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Platform /></el-icon>
        <p style="margin-top: 8px">{{ keyword ? '未找到匹配服务器' : '尚未添加服务器，点击右上角「新增服务器」' }}</p>
      </div>

      <!-- 全局统一服务器卡片网格流 (自适应 1~4 列) -->
      <div v-else class="server-card-grid">
        <div v-for="row in filtered" :key="row.id" class="server-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <el-checkbox
                :model-value="selectedIds.includes(row.id)"
                style="height: auto; margin-right: 2px"
                @change="(v: any) => toggleSelect(row.id, !!v)"
                @click.stop
              />
              <span class="x-status-dot" :class="row.status === 1 ? 'online' : 'offline'" />
              <span class="server-name" title="点击查看详情" @click="openDrawer(row)">{{ row.name }}</span>
              <span class="x-chip" :class="row.status === 1 ? 'green' : 'gray'" style="font-size: 10px; padding: 1px 5px">
                {{ row.status === 1 ? '在线' : '离线' }}
              </span>
              <span v-if="row.location" class="x-chip blue" style="font-size: 10px; padding: 1px 5px">{{ row.location }}</span>
              <!-- 到期预警徽标 -->
              <el-tooltip v-if="getExpiryStatus(row.expire_at)" :content="getExpiryStatus(row.expire_at)!.tip" placement="top">
                <span class="x-chip" :class="getExpiryStatus(row.expire_at)!.cls" style="cursor: help; font-size: 10px; padding: 1px 5px">
                  {{ getExpiryStatus(row.expire_at)!.text }}
                </span>
              </el-tooltip>
              <el-tooltip v-if="xrayAlertChip(row).show" :content="xrayAlertChip(row).tip" placement="top">
                <span class="x-chip" :class="xrayAlertChip(row).cls" style="cursor: help; font-size: 10px; padding: 1px 5px">
                  {{ xrayAlertChip(row).text }}
                </span>
              </el-tooltip>
            </div>
            <!-- 配置同步状态（push_state 由后端推导：none/pending/rejected/synced/drift） -->
            <el-tooltip v-if="pushChip(row).tip" :content="pushChip(row).tip" placement="top">
              <span class="x-chip" :class="pushChip(row).cls" style="cursor: help; font-size: 10.5px">{{ pushChip(row).text }}</span>
            </el-tooltip>
            <span v-else class="x-chip" :class="pushChip(row).cls" style="font-size: 10.5px">{{ pushChip(row).text }}</span>
          </div>

          <!-- 属性网格 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <span class="item-label">服务器地址</span>
              <div class="item-value">
                <code class="cell-mono font-12" style="cursor: pointer; color: var(--x-primary); font-weight: 600" title="点击复制" @click="copyText(row.host, '服务器地址')">
                  {{ row.host }}
                </code>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">接入点</span>
              <div class="item-value">
                <el-link type="primary" :underline="false" style="font-size: 12.5px; font-weight: 600" @click="goInbounds(row)">
                  {{ inboundCount(row.id) }} 个入站
                </el-link>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">Agent 版本</span>
              <div class="item-value" style="display: flex; align-items: center; gap: 4px; flex-wrap: wrap">
                <span class="cell-mono muted font-12">{{ row.agent_version || 'v—' }}</span>
                <span
                  v-if="getAgentVersionStatus(row.agent_version).type === 'latest'"
                  class="x-chip green"
                  style="font-size: 9.5px; padding: 0 4px"
                >最新</span>
                <span
                  v-else-if="getAgentVersionStatus(row.agent_version).type === 'outdated'"
                  class="x-chip orange"
                  style="font-size: 9.5px; padding: 0 4px"
                  :title="`官方最新版本 ${latestAgentVersion}`"
                >有新版</span>
                <el-link
                  v-if="row.status === 1"
                  :type="getAgentVersionStatus(row.agent_version).type === 'outdated' ? 'warning' : 'primary'"
                  :underline="false"
                  :disabled="upgradingId === row.id"
                  style="font-size: 11px; font-weight: 600"
                  @click="upgradeNodeAgent(row)"
                >
                  {{ getAgentVersionStatus(row.agent_version).type === 'outdated' ? '升级' : (getAgentVersionStatus(row.agent_version).type === 'latest' ? '重新安装' : '升级') }}
                </el-link>
              </div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">最后心跳</span>
              <div class="item-value cell-mono muted" style="font-size: 11.5px">
                {{ row.last_seen_at ? fmtTime(row.last_seen_at) : '未有心跳记录' }}
              </div>
            </div>
            <!-- VPS 续费与资产信息（若设置了其中任何一项） -->
            <div v-if="row.expire_at || row.billing_cycle || row.price || row.idc_address" class="grid-item full-width">
              <span class="item-label">VPS 续费与资产</span>
              <div class="item-value" style="display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap">
                <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px">
                  <span
                    v-if="row.expire_at"
                    class="cell-mono"
                    :style="getExpiryStatus(row.expire_at)?.isUrgent ? { color: getExpiryStatus(row.expire_at)!.color, fontWeight: 600 } : {}"
                    :title="getExpiryStatus(row.expire_at)?.tip || `到期日：${formatExpireDate(row.expire_at)}`"
                  >
                    {{ formatExpireDate(row.expire_at) }}
                  </span>
                  <span v-if="row.billing_cycle" class="x-chip gray" style="font-size: 9.5px; padding: 0 4px">{{ row.billing_cycle }}</span>
                  <span v-if="row.price" class="cell-mono" style="font-weight: 600; color: var(--x-primary)">{{ row.price }}</span>
                </div>
                <div v-if="row.idc_address" style="display: flex; align-items: center; gap: 4px">
                  <el-link
                    v-if="isUrl(row.idc_address)"
                    type="primary"
                    :underline="false"
                    style="font-size: 11px"
                    :href="normalizeUrl(row.idc_address)"
                    target="_blank"
                  >
                    <el-icon style="margin-right: 2px"><Link /></el-icon>IDC 控制台
                  </el-link>
                  <span
                    v-else
                    class="muted font-12"
                    style="cursor: pointer; display: inline-flex; align-items: center; gap: 2px"
                    title="点击复制 IDC 地址"
                    @click="copyText(row.idc_address, 'IDC 地址')"
                  >
                    {{ row.idc_address }}
                    <el-icon style="font-size: 11px"><CopyDocument /></el-icon>
                  </span>
                </div>
              </div>
            </div>
          </div>

          <!-- 操作按钮栏 -->
          <div class="card-foot-actions">
            <el-button size="small" type="primary" plain @click="openDrawer(row)">
              <el-icon><Setting /></el-icon>&nbsp;详情
            </el-button>
            <el-button size="small" type="success" plain @click="openMetrics(row)">
              <el-icon><TrendCharts /></el-icon>&nbsp;监控
            </el-button>
            <el-button size="small" @click="restartXray(row)">
              <el-icon><RefreshRight /></el-icon>&nbsp;重启
            </el-button>
            <el-dropdown trigger="click" @command="(cmd: string) => onMore(cmd, row)">
              <el-button size="small" style="flex: none; padding: 0 8px">
                <el-icon><MoreFilled /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="status"><el-icon><View /></el-icon>运行状态</el-dropdown-item>
                  <el-dropdown-item command="logs"><el-icon><Document /></el-icon>服务器日志</el-dropdown-item>
                  <el-dropdown-item v-if="row.idc_address && isUrl(row.idc_address)" command="open_idc">
                    <el-icon><Link /></el-icon>打开 IDC 控制台
                  </el-dropdown-item>
                  <el-dropdown-item command="upgrade">
                    <el-icon><Upload /></el-icon>
                    {{ getAgentVersionStatus(row.agent_version).type === 'outdated' ? '升级 Agent' : (getAgentVersionStatus(row.agent_version).type === 'latest' ? '重新安装 Agent' : '升级 Agent') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="rollback">
                    <el-icon><Download /></el-icon>回滚 Agent
                  </el-dropdown-item>
                  <el-dropdown-item divided command="edit"><el-icon><Edit /></el-icon>编辑服务器</el-dropdown-item>
                  <el-dropdown-item command="reset"><el-icon><Key /></el-icon>重置密钥</el-dropdown-item>
                  <el-dropdown-item command="delete" divided style="color: var(--el-color-danger)">
                    <el-icon><Delete /></el-icon>删除服务器
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>
    </BaseCard>

    <!-- 新增服务器 -->
    <el-dialog v-model="createOpen" title="新增服务器" width="680px" @close="closeCreate">
      <template v-if="!createdResult">
        <el-form label-position="top">
          <el-row :gutter="14">
            <el-col :span="12">
              <el-form-item label="名称" required><el-input v-model="createForm.name" placeholder="如 Tokyo-01 / 广州移动 BGP" /></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="地址" required><el-input v-model="createForm.host" placeholder="如 tokyo01.example.com / 120.232.x.x" /></el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="14">
            <el-col :span="12">
              <el-form-item label="地区"><el-input v-model="createForm.location" placeholder="如 日本 / 广州（选填）" /></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="备注"><el-input v-model="createForm.remark" placeholder="选填" /></el-form-item>
            </el-col>
          </el-row>
          <el-divider content-position="left" style="margin: 8px 0 14px">
            <span style="font-size: 12px; color: var(--x-text-3)">VPS 续费与资产信息（选填）</span>
          </el-divider>
          <el-row :gutter="14">
            <el-col :span="12">
              <el-form-item label="到期日">
                <el-date-picker
                  v-model="createForm.expire_at"
                  type="date"
                  placeholder="选择到期日期"
                  format="YYYY-MM-DD"
                  value-format="YYYY-MM-DD"
                  style="width: 100%"
                  clearable
                />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="计费周期">
                <el-select
                  v-model="createForm.billing_cycle"
                  filterable
                  allow-create
                  default-first-option
                  placeholder="选择或输入计费周期"
                  clearable
                  style="width: 100%"
                >
                  <el-option label="月付" value="月付" />
                  <el-option label="季付" value="季付" />
                  <el-option label="半年付" value="半年付" />
                  <el-option label="年付" value="年付" />
                  <el-option label="两年付" value="两年付" />
                  <el-option label="三年付" value="三年付" />
                  <el-option label="一次性 / 永久" value="一次性" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="14">
            <el-col :span="12">
              <el-form-item label="续费价格">
                <el-input v-model="createForm.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="IDC 地址">
                <el-input v-model="createForm.idc_address" placeholder="控制台网址，如 https://..." clearable />
              </el-form-item>
            </el-col>
          </el-row>
        </el-form>
      </template>
      <template v-else>
        <el-alert type="success" :closable="false" show-icon title="服务器已创建" description="在服务器上以 root 执行以下一键安装命令：" style="margin-bottom: 12px" />
          <div class="secret-box">
            <div class="secret-row install-row">
              <span class="k">安装</span>
              <code class="install-cmd">{{ createdResult.install_cmd }}</code>
              <el-button size="small" text @click="copyText(createdResult.install_cmd, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
            <div class="secret-row">
              <span class="k">node_id</span>
              <code>{{ createdResult.node_id }}</code>
              <el-button size="small" text @click="copyText(createdResult.node_id, 'node_id')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
            <div class="secret-row">
              <span class="k">secret</span>
              <code>{{ createdResult.secret }}</code>
              <el-button size="small" text @click="copyText(createdResult.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
              <TipIcon
                type="warn"
                content="secret 仅显示这一次；安装完成后回到本页查看在线状态，在服务器详情中生成并下发配置。"
              />
            </div>
          </div>
      </template>
      <template #footer>
        <template v-if="!createdResult">
          <el-button @click="createOpen = false">取消</el-button>
          <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
        </template>
        <template v-else>
          <el-button type="primary" @click="closeCreate">完成</el-button>
        </template>
      </template>
    </el-dialog>

    <!-- 状态详情 -->
    <el-dialog v-model="statusOpen" title="服务器状态" width="420px">
      <div v-loading="statusLoading" class="status-rows">
        <template v-if="statusData">
          <div class="row"><span class="k">Xray 运行</span><span class="v">{{ statusData.data?.xray_running ? '运行中' : '已停止' }}</span></div>
          <div v-if="statusData.data?.xray_state" class="row">
            <span class="k">Xray 状态</span>
            <span class="v" :style="statusData.data?.xray_state === 'failed' ? { color: 'var(--el-color-danger, #ef4444)' } : {}">
              {{ xrayStateText(statusData.data.xray_state) }}<template v-if="statusData.data?.xray_restart_failures">（连续失败 {{ statusData.data.xray_restart_failures }} 次）</template>
            </span>
          </div>
          <div v-if="statusData.data?.xray_last_error" class="row">
            <span class="k">启动失败原因</span>
            <span class="v" style="word-break: break-all; font-size: 12px">{{ statusData.data.xray_last_error }}</span>
          </div>
          <div class="row"><span class="k">进程 PID</span><span class="v">{{ statusData.data?.pid ?? '—' }}</span></div>
          <div class="row"><span class="k">启动时间</span><span class="v">{{ statusData.data?.started_at ? fmtTime(statusData.data.started_at) : '—' }}</span></div>
          <div class="row"><span class="k">运行时长</span><span class="v">{{ statusData.data?.uptime_sec ?? 0 }} 秒</span></div>
          <div class="row"><span class="k">配置路径</span><span class="v"><code class="cell-mono">{{ statusData.data?.config_path ?? '—' }}</code></span></div>
        </template>
        <el-empty v-else-if="!statusLoading" description="无数据" />
      </div>
    </el-dialog>

    <!-- 日志 -->
    <el-dialog v-model="logOpen" :title="`最近日志 · ${logTarget?.name ?? ''}`" width="760px">
      <div class="log-toolbar">
        <div class="toolbar-left">
          <span class="toolbar-label">行数:</span>
          <el-radio-group v-model="logLines" size="small" @change="fetchLogs">
            <el-radio-button :value="50">50</el-radio-button>
            <el-radio-button :value="100">100</el-radio-button>
            <el-radio-button :value="200">200</el-radio-button>
            <el-radio-button :value="500">500</el-radio-button>
          </el-radio-group>
          <el-switch v-model="logReverse" size="small" active-text="最新在最前" @change="scrollLogToBottom" />
        </div>
        <div class="toolbar-right">
          <el-button size="small" :loading="logLoading" @click="fetchLogs">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
          <el-button size="small" @click="copyLogs">
            <el-icon><CopyDocument /></el-icon>&nbsp;复制
          </el-button>
        </div>
      </div>
      <pre ref="logPreRef" v-loading="logLoading" class="log-view">{{ displayedLogContent }}</pre>
    </el-dialog>

    <!-- Agent 升级/回滚监控弹窗 -->
    <el-dialog
      v-model="upgradeModalOpen"
      :title="`Agent ${upgradeIsRollback ? '回滚' : '升级'}监控 · ${upgradeTarget?.name ?? ''}`"
      width="640px"
      :close-on-click-modal="false"
      @close="stopUpgradePolling"
    >
      <div class="upgrade-modal-content">
        <div class="upgrade-header">
          <div class="upgrade-node-info">
            <span class="node-name">{{ upgradeTarget?.name }}</span>
            <code class="cell-mono muted font-11">服务器 ID: {{ upgradeTarget?.node_id }}</code>
          </div>
          <div class="upgrade-ver-info">
            <span class="ver-label">版本流转:</span>
            <el-tag size="small" type="info">{{ upgradeTarget?.agent_version || '未知' }}</el-tag>
            <span class="ver-arrow">→</span>
            <el-tag size="small" :type="upgradeIsRollback ? 'warning' : 'success'">
              {{ (upgradeIsRollback ? rollbackTargetVersion : latestAgentVersion) || '目标版本' }}
            </el-tag>
          </div>
        </div>

        <!-- 升级/回滚步骤条 -->
        <el-steps
          :active="upgradeActiveStep"
          finish-status="success"
          :process-status="upgradeStatus?.phase === 'failed' ? 'error' : 'process'"
          align-center
          style="margin: 28px 0 20px"
        >
          <el-step title="版本解析" :description="upgradeIsRollback ? '检查指定版本' : '检查远端版本'" />
          <el-step title="下载资源" description="GitHub Releases" />
          <el-step title="完整性校验" description="SHA256 校验" />
          <el-step title="重启生效" description="服务重载就绪" />
        </el-steps>

        <!-- 当前状态卡片 -->
        <div
          class="upgrade-status-box"
          :class="{
            'is-failed': upgradeStatus?.phase === 'failed',
            'is-success': upgradeStatus?.phase === 'success',
            'is-running': upgradeStatus && upgradeStatus.phase !== 'failed' && upgradeStatus.phase !== 'success'
          }"
        >
          <div class="status-icon">
            <el-icon v-if="upgradeStatus?.phase === 'success'" class="icon-success"><Check /></el-icon>
            <el-icon v-else-if="upgradeStatus?.phase === 'failed'" class="icon-error"><Close /></el-icon>
            <el-icon v-else class="is-loading icon-process"><Loading /></el-icon>
          </div>
          <div class="status-texts">
            <div class="status-msg">{{ upgradeStatus?.message || '等待服务器响应...' }}</div>
            <div v-if="upgradeStatus?.error" class="status-err cell-mono">
              {{ upgradeStatus.error }}
            </div>
            <div v-else-if="upgradeStatus?.phase !== 'success' && upgradeStatus?.phase !== 'failed'" class="status-hint">
              服务器正在执行后台{{ upgradeIsRollback ? '回滚' : '升级' }}操作，若网络连通较慢请耐心等待（通常耗时 10-60 秒）
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 10px">
          <el-button
            v-if="upgradeStatus?.phase !== 'success' && upgradeStatus?.phase !== 'failed'"
            @click="upgradeModalOpen = false"
          >
            后台运行
          </el-button>
          <el-button
            v-else
            type="primary"
            @click="upgradeModalOpen = false"
          >
            完成并关闭
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 批量升级总览弹窗 -->
    <el-dialog
      v-model="batchOpen"
      title="批量升级 Agent"
      width="680px"
      :close-on-click-modal="false"
      @close="stopBatchPolling"
    >
      <el-table :data="batchRows" size="small" max-height="420">
        <el-table-column prop="name" label="服务器" min-width="140" show-overflow-tooltip />
        <el-table-column label="当前版本" width="110">
          <template #default="{ row }"><span class="cell-mono font-12">{{ row.version }}</span></template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.skipped" size="small" type="info">已跳过</el-tag>
            <el-tag
              v-else-if="row.status"
              size="small"
              :type="row.status.phase === 'success' ? 'success' : row.status.phase === 'failed' ? 'danger' : 'primary'"
            >
              {{ phaseMeta[row.status.phase]?.label || row.status.phase }}
            </el-tag>
            <el-tag v-else size="small" type="info">等待</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="进度" min-width="200">
          <template #default="{ row }">
            <span v-if="row.skipped" class="muted" style="font-size: 12px">{{ row.reason }}</span>
            <template v-else-if="row.status">
              <el-progress
                :percentage="phaseMeta[row.status.phase]?.percent ?? 0"
                :status="progressStatus(row.status.phase)"
                :stroke-width="8"
              />
              <div v-if="row.status.error" class="cell-mono" style="font-size: 11px; color: var(--el-color-danger); margin-top: 2px; word-break: break-all">
                {{ row.status.error }}
              </div>
              <div v-else-if="row.status.message" class="muted" style="font-size: 11px; margin-top: 2px">
                {{ row.status.message }}
              </div>
            </template>
            <span v-else class="muted" style="font-size: 12px">—</span>
          </template>
        </el-table-column>
      </el-table>
      <div style="margin-top: 10px; font-size: 12px; color: var(--x-text-3)">
        <template v-if="batchRunning">正在升级，进度实时更新…升级完成的服务器将在下一轮心跳后刷新版本号。</template>
        <template v-else>全部服务器已处理完成；升级完成的服务器版本号将在下一轮心跳后刷新。</template>
      </div>
      <template #footer>
        <el-button type="primary" @click="batchOpen = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 编辑服务器 -->
    <el-dialog v-model="editOpen" title="编辑服务器" width="640px">
      <el-form label-position="top">
        <el-row :gutter="14">
          <el-col :span="12">
            <el-form-item label="名称" required><el-input v-model="editForm.name" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="地址" required><el-input v-model="editForm.host" placeholder="如 tokyo01.example.com" /></el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="14">
          <el-col :span="12">
            <el-form-item label="地区"><el-input v-model="editForm.location" placeholder="选填" /></el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="备注"><el-input v-model="editForm.remark" placeholder="选填" /></el-form-item>
          </el-col>
        </el-row>
        <el-divider content-position="left" style="margin: 8px 0 14px">
          <span style="font-size: 12px; color: var(--x-text-3)">VPS 续费与资产信息（选填）</span>
        </el-divider>
        <el-row :gutter="14">
          <el-col :span="12">
            <el-form-item label="到期日">
              <el-date-picker
                v-model="editForm.expire_at"
                type="date"
                placeholder="选择到期日期"
                format="YYYY-MM-DD"
                value-format="YYYY-MM-DDTHH:mm:ss.SSSZ"
                style="width: 100%"
                clearable
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="计费周期">
              <el-select
                v-model="editForm.billing_cycle"
                filterable
                allow-create
                default-first-option
                placeholder="选择或输入计费周期"
                clearable
                style="width: 100%"
              >
                <el-option label="月付" value="月付" />
                <el-option label="季付" value="季付" />
                <el-option label="半年付" value="半年付" />
                <el-option label="年付" value="年付" />
                <el-option label="两年付" value="两年付" />
                <el-option label="三年付" value="三年付" />
                <el-option label="一次性 / 永久" value="一次性" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="14">
          <el-col :span="12">
            <el-form-item label="续费价格">
              <el-input v-model="editForm.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="IDC 地址">
              <el-input v-model="editForm.idc_address" placeholder="控制台网址，如 https://..." clearable />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="editOpen = false">取消</el-button>
        <el-button type="primary" :loading="editSaving" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密钥 -->
    <el-dialog v-model="secretOpen" title="重置密钥" width="600px">
      <el-alert type="warning" :closable="false" show-icon title="新密钥已生成（仅显示这一次）" description="请更新服务器 /etc/xray-agent/config.yml 中的 secret 后重启 xray-agent 服务；或重新执行下方安装命令" style="margin-bottom: 12px" />
      <div v-if="secretInfo" class="secret-box">
        <div class="secret-row">
          <span class="k">node_id</span>
          <code>{{ secretInfo.node_id }}</code>
        </div>
        <div class="secret-row">
          <span class="k">secret</span>
          <code>{{ secretInfo.secret }}</code>
          <el-button size="small" text @click="copyText(secretInfo.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
        </div>
        <div v-if="secretInfo.install_cmd" class="secret-row">
          <span class="k">安装</span>
          <code class="install-cmd">{{ secretInfo.install_cmd }}</code>
          <el-button size="small" text @click="copyText(secretInfo.install_cmd!, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
        </div>
      </div>
      <template #footer>
        <el-button type="primary" @click="secretOpen = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 节点管理抽屉 -->
    <ServerNodeDrawer
      v-model="drawerOpen"
      :server="drawerServer"
      @removed="load"
      @changed="load"
    />

    <!-- 时序性能监控抽屉 -->
    <ServerMetricsDrawer
      v-model="metricsOpen"
      :server-id="metricsServer?.id || 0"
      :server-name="metricsServer?.name || ''"
    />
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.agent-cell { display: flex; align-items: center; gap: 8px; }
.muted { color: var(--x-text-3); }
.secret-box { display: grid; gap: 10px; }
.tip { font-size: 12px; color: var(--x-text-3); }
.install-cmd { font-size: 11.5px; }
.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-primary-soft);
  border-radius: 8px;
  padding: 10px 12px;
  .k { color: var(--x-text-2); font-size: 12.5px; flex: none; width: 56px; }
  code { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-primary); word-break: break-all; flex: 1; }
}
.ok-text { color: var(--x-success); font-size: 13px; }
.err-text { color: var(--x-danger); font-size: 13px; }
.status-rows .row { display: flex; justify-content: space-between; padding: 11px 0; border-bottom: 1px solid var(--x-border); font-size: 13.5px; }
.status-rows .k { color: var(--x-text-2); }
.status-rows .v { font-weight: 500; }
.log-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  flex-wrap: wrap;
  gap: 10px;

  .toolbar-left, .toolbar-right {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .toolbar-label {
    font-size: 12px;
    color: var(--x-text-2);
  }
}
.log-view {
  background: #171b2e;
  color: #c7d2fe;
  border-radius: 8px;
  padding: 14px;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 460px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

/* ================= Agent 升级监控弹窗 ================= */
.upgrade-modal-content {
  padding: 4px 6px;
}
.upgrade-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--x-fill-2, #f8fafc);
  padding: 12px 16px;
  border-radius: 8px;
  border: 1px solid var(--x-border-light, #f1f5f9);

  .upgrade-node-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    .node-name {
      font-weight: 600;
      font-size: 14px;
      color: var(--x-text);
    }
  }
  .upgrade-ver-info {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    .ver-label {
      color: var(--x-text-3);
    }
    .ver-arrow {
      color: var(--x-text-3);
      font-weight: bold;
    }
  }
}
.upgrade-status-box {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 8px;
  background: var(--x-bg, #f9fafb);
  border: 1px solid var(--x-border, #e5e7eb);
  margin-top: 16px;

  // 与管理端「面板更新结果」同款：状态色走语义 token，深色模式下不再出现亮色块
  &.is-running {
    border-color: var(--x-info);
    background: var(--x-info-soft);
    .icon-process { font-size: 22px; color: var(--x-info); }
  }
  &.is-success {
    border-color: var(--x-success);
    background: var(--x-success-soft);
    .icon-success { font-size: 22px; color: var(--x-success); font-weight: bold; }
  }
  &.is-failed {
    border-color: var(--x-danger);
    background: var(--x-danger-soft);
    .icon-error { font-size: 22px; color: var(--x-danger); font-weight: bold; }
  }

  .status-texts {
    flex: 1;
    .status-msg {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text);
      line-height: 1.5;
    }
    .status-err {
      margin-top: 6px;
      color: var(--x-danger);
      font-size: 11.5px;
      line-height: 1.4;
      word-break: break-all;
      background: rgba(254, 226, 226, 0.7);
      padding: 6px 8px;
      border-radius: 4px;
    }
    .status-hint {
      margin-top: 4px;
      color: var(--x-text-3);
      font-size: 11.5px;
    }
  }
}

/* ================= 全局统一服务器卡片网格流 ================= */
.server-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.server-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e5e7eb);
  border-radius: var(--x-radius, 10px);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  display: flex;
  flex-direction: column;
  justify-content: space-between;

  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
    box-shadow: var(--x-shadow-md);
    transform: translateY(-1px);
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-title {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .server-name {
      font-weight: 600;
      font-size: 14px;
      color: var(--x-text, #111827);
      cursor: pointer;
      &:hover {
        color: var(--x-primary);
      }
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      &.full-width {
        grid-column: 1 / -1;
      }

      .item-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .item-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }
    }
  }

  .card-foot-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    padding-top: 10px;
    border-top: 1px solid var(--x-border-light, #f1f5f9);
    margin-top: 6px;

    .el-button {
      flex: 1;
      margin: 0;
      font-size: 12px;
      padding: 6px 8px;
      height: 30px;
    }
  }
}

@media (max-width: 768px) {
  .secret-row {
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;

    .k {
      width: auto;
      font-weight: 600;
    }
  }

  .status-rows .row {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
    padding: 8px 0;
  }
}

@media (max-width: 640px) {
  .server-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>