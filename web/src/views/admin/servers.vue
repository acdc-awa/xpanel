<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Check, Close, CopyDocument, Loading, Refresh } from '@element-plus/icons-vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsButtonDropdown,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsCheckbox,
  CsContainer,
  CsEmpty,
  CsInput,
  CsPage,
  CsSegmentedControl,
  CsSpinner,
  CsStatusIndicator,
  CsToolbar,
  CsTooltip,
  ElMessage,
  ElMessageBox,
  type ButtonDropdownItem,
  type SegmentedOption,
} from '@/components/cloudscape'
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
  getAgentVersions,
  getXrayVersions,
  upgradeXray,
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

const onlineCount = computed(() => list.value.filter((s) => s.status === 1).length)
const offlineCount = computed(() => list.value.filter((s) => s.status !== 1).length)

type StatusFilter = 'all' | 'online' | 'offline' | 'expiring'
type BadgeColor =
  | 'grey'
  | 'green'
  | 'blue'
  | 'red'
  | 'severity-critical'
  | 'severity-high'
  | 'severity-medium'
  | 'severity-low'
  | 'severity-neutral'

const statusFilter = ref<StatusFilter>('all')

const filtered = computed(() => {
  let res = list.value
  if (statusFilter.value === 'online') {
    res = res.filter((s) => s.status === 1)
  } else if (statusFilter.value === 'offline') {
    res = res.filter((s) => s.status !== 1)
  } else if (statusFilter.value === 'expiring' || filterExpiring.value) {
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

const statusFilterOptions = computed<SegmentedOption[]>(() => {
  const options: SegmentedOption[] = [
    { id: 'all', text: `全部 ${list.value.length}` },
    { id: 'online', text: `在线 ${onlineCount.value}` },
    { id: 'offline', text: `离线 ${offlineCount.value}` },
  ]
  if (expiringServers.value.length > 0) {
    options.push({ id: 'expiring', text: `待续费 ${expiringServers.value.length}` })
  }
  return options
})

function onStatusFilterChange(value: string | number) {
  statusFilter.value = value as StatusFilter
}

/** 页面原有的 x-chip 配色语义映射到 Cloudscape Badge。 */
function badgeColorOf(cls: string): BadgeColor {
  switch (cls) {
    case 'red':
      return 'red'
    case 'green':
    case 'emerald':
      return 'green'
    case 'blue':
      return 'blue'
    case 'orange':
    case 'deep-orange':
    case 'yellow':
    case 'amber':
      return 'severity-medium'
    default:
      return 'grey'
  }
}

function expiryBadgeColor(row: ServerItem): BadgeColor {
  const st = getExpiryStatus(row.expire_at)
  return st ? badgeColorOf(st.cls) : 'grey'
}

function cardMenuItems(row: ServerItem): ButtonDropdownItem[] {
  const items: ButtonDropdownItem[] = [
    { id: 'status', text: '运行状态', iconName: 'view-horizontal' },
    { id: 'logs', text: '服务器日志', iconName: 'file' },
  ]
  if (row.idc_address && isUrl(row.idc_address)) {
    items.push({ id: 'open_idc', text: '打开 IDC 控制台', iconName: 'external' })
  }
  items.push(
    { id: 'agent_version', text: 'Agent 版本…', iconName: 'upload' },
    { id: 'xray', text: 'Xray 内核版本…', iconName: 'settings' },
    { id: 'edit', text: '编辑服务器', iconName: 'edit', dividerBefore: true },
    { id: 'reset', text: '重置密钥', iconName: 'key' },
    { id: 'delete', text: '删除服务器', iconName: 'remove', dividerBefore: true, danger: true },
  )
  return items
}

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
    if (data.code === 0) {
      list.value = data.data.items
      // 待续费筛选项会随列表消失，避免筛选项没了列表还停在空结果上
      if (statusFilter.value === 'expiring' && expiringServers.value.length === 0) {
        statusFilter.value = 'all'
      }
    } else ElMessage.error(data.message)
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
  sort_order: 0,
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
      sort_order: createForm.sort_order,
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
      createForm.sort_order = 0
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
  createForm.sort_order = 0
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
// 升级对象：Agent 与 Xray 内核共用同一套进度弹窗，但版本字段与文案不同。
const upgradeKind = ref<'agent' | 'xray'>('agent')
const upgradeIsRollback = ref(false)
const rollbackTargetVersion = ref('')
// 版本选择器当前目标（Agent / Xray 共用）
const versionTarget = ref('')

const upgradeTitle = computed(() => {
  if (upgradeKind.value === 'xray') {
    return `Xray 内核${upgradeIsRollback.value ? '降级' : '切换'}监控 · ${upgradeTarget.value?.name ?? ''}`
  }
  return `Agent ${upgradeIsRollback.value ? '回滚' : '升级'}监控 · ${upgradeTarget.value?.name ?? ''}`
})

const upgradeFromLabel = computed(() =>
  upgradeKind.value === 'xray'
    ? upgradeTarget.value?.xray_version || '未知'
    : upgradeTarget.value?.agent_version || '未知',
)

const upgradeToLabel = computed(() => {
  // 版本选择器会显式写入目标版本（含升级到非最新版的情况），优先使用它
  if (rollbackTargetVersion.value) return rollbackTargetVersion.value
  return upgradeKind.value === 'xray'
    ? versionTarget.value || '目标版本'
    : latestAgentVersion.value || '目标版本'
})

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

// ---- 版本切换（Agent / Xray 共用一套选择器与进度弹窗） ----
type VersionSubject = 'agent' | 'xray'

const versionDialogOpen = ref(false)
const versionSubject = ref<VersionSubject>('agent')
const versionServer = ref<ServerItem | null>(null)
const versionOptions = ref<string[]>([])
const versionOptionsLoading = ref(false)
const versionHint = ref('')
const versionSwitching = ref(false)

function normVersionTag(v: string) {
  const s = (v || '').trim()
  if (!s) return ''
  return s.startsWith('v') ? s : `v${s}`
}

const versionSubjectLabel = computed(() => (versionSubject.value === 'agent' ? 'Agent' : 'Xray 内核'))
const versionCurrent = computed(() =>
  normVersionTag(
    versionSubject.value === 'agent'
      ? versionServer.value?.agent_version || ''
      : versionServer.value?.xray_version || '',
  ),
)
const versionCompare = computed(() => {
  if (!versionCurrent.value || !versionTarget.value) return null
  return compareVersion(versionCurrent.value, versionTarget.value)
})
const versionActionLabel = computed(() => {
  if (!versionTarget.value) return '切换'
  if (!versionCurrent.value) return '安装'
  if (versionCompare.value === 0) return '重新安装'
  return (versionCompare.value ?? 0) > 0 ? '升级' : '降级'
})

async function loadVersionOptions(refresh = false) {
  versionOptionsLoading.value = true
  try {
    const { data } =
      versionSubject.value === 'agent'
        ? await getAgentVersions(refresh)
        : await getXrayVersions(refresh)
    if (data.code === 0) {
      versionOptions.value = data.data.versions || []
      versionHint.value = data.data.checked_at
        ? `版本列表更新于 ${formatDateTime(data.data.checked_at)}（只列最近版本，更老的版本可直接输入）`
        : '可直接输入版本号'
    }
  } catch (e) {
    versionHint.value = `版本列表获取失败，可直接手动输入（${errMsg(e, '网络异常')}）`
  } finally {
    versionOptionsLoading.value = false
  }
}

async function openVersionDialog(row: any, subject: VersionSubject) {
  versionSubject.value = subject
  versionServer.value = row
  const current = normVersionTag(subject === 'agent' ? row.agent_version || '' : row.xray_version || '')
  // 默认预选：Agent 落后于最新版时选最新版（点开即升级），否则选当前版本（重装/切换）
  const latest = subject === 'agent' ? latestAgentVersion.value : ''
  versionTarget.value = latest && compareVersion(current || '0', latest) < 0 ? normVersionTag(latest) : current
  versionDialogOpen.value = true
  await loadVersionOptions(false)
}

async function applyVersionSwitch() {
  const row = versionServer.value
  const target = normVersionTag(versionTarget.value)
  const subject = versionSubject.value
  if (!row) return
  if (!target || !/^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?$/.test(target)) {
    ElMessage.error('版本号格式不正确（示例：v0.1.19 / v26.10.21）')
    return
  }
  const cmp = versionCompare.value
  const label = versionActionLabel.value
  const lines = [
    `服务器：${row.name}`,
    `当前 ${versionSubjectLabel.value}：${versionCurrent.value || '未知（旧 Agent 未上报）'}`,
    `目标 ${versionSubjectLabel.value}：${target}`,
    '',
  ]
  if (subject === 'agent') {
    if (cmp !== null && cmp < 0) {
      lines.push('⚠️ 降级风险：更老的 Agent 可能不认识主控新下发的协议消息（未知消息会被静默丢弃），请确认主控与之兼容。')
    }
    lines.push('将从 XPanel-Node Releases 下载 Agent 二进制并做 sha256 校验，完成后服务自动重启（短暂离线）。')
  } else if (cmp !== null && cmp < 0) {
    lines.push('⚠️ 降级风险：Xray ≥26.9.8 起 REALITY 强制要求 MLKEM keyshare，更老的内核也可能无法加载当前配置。')
    lines.push('安装失败会自动回滚到原内核；期间 xray 会短暂中断。')
  } else {
    lines.push('节点将从 XTLS/Xray-core Releases 下载并做 SHA-256 校验，完成后重启 xray（短暂中断）。')
    lines.push('geoip.dat / geosite.dat 会一并同步为该发布包内的数据。')
  }
  try {
    await ElMessageBox.confirm(lines.join('\n'), `确认${label}${versionSubjectLabel.value}`, {
      type: 'warning',
      confirmButtonText: `立即${label}`,
      cancelButtonText: '取消',
    })
  } catch {
    return
  }

  versionDialogOpen.value = false
  upgradeKind.value = subject
  upgradeIsRollback.value = cmp !== null && cmp < 0
  rollbackTargetVersion.value = target
  upgradeTarget.value = row
  upgradeStatus.value = {
    phase: 'starting',
    target,
    message: `正在向服务器下发${versionSubjectLabel.value}${label}指令...`,
    ts: Math.floor(Date.now() / 1000),
    kind: subject,
  }
  upgradeModalOpen.value = true
  upgradingId.value = row.id
  versionSwitching.value = true

  stopUpgradePolling()
  upgradeTimer = setInterval(() => {
    pollUpgradeStatus(row.id)
  }, 1500)

  try {
    // 目标不高于当前时必须 force（协议语义：force 才允许降级/同版本重装）
    const force = cmp === null || cmp <= 0
    const { data } =
      subject === 'agent'
        ? await upgradeAgent(row.id, { target, force })
        : await upgradeXray(row.id, { target, force })
    if (data.code === 0 && data.data.ok) {
      upgradeStatus.value = {
        phase: 'success',
        target,
        message: (data.data.data as string) || `${versionSubjectLabel.value}已切换完成`,
        ts: Math.floor(Date.now() / 1000),
        kind: subject,
      }
      stopUpgradePolling()
      load()
      setTimeout(load, 15000)
    } else {
      upgradeStatus.value = {
        phase: 'failed',
        target,
        message: `${versionSubjectLabel.value}切换失败`,
        error: data.data?.error || data.message || `${versionSubjectLabel.value}切换失败`,
        ts: Math.floor(Date.now() / 1000),
        kind: subject,
      }
      stopUpgradePolling()
    }
  } catch (e: any) {
    upgradeStatus.value = {
      phase: 'failed',
      target,
      message: `${versionSubjectLabel.value}切换请求超时或网络异常`,
      error: errMsg(e, '切换失败（服务器可能仍在后台下载，可稍后刷新查看版本）'),
      ts: Math.floor(Date.now() / 1000),
      kind: subject,
    }
    stopUpgradePolling()
  } finally {
    versionSwitching.value = false
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
  sort_order: 0,
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
    sort_order: row.sort_order ?? 0,
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
      sort_order: editForm.sort_order,
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
  else if (cmd === 'agent_version') openVersionDialog(row, 'agent')
  else if (cmd === 'xray') openVersionDialog(row, 'xray')
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
  <CsPage>
    <CsToolbar>
      <CsInput
        v-model="keyword"
        placeholder="搜索名称 / 地址 / 地区"
        prefix-icon="search"
        clearable
        style="width: 240px"
        aria-label="搜索服务器"
      />
      <CsButton icon-name="refresh" @click="load">刷新</CsButton>
      <CsCheckbox
        :model-value="allSelected"
        :indeterminate="selectedIds.length > 0 && !allSelected"
        :disabled="filtered.length === 0"
        @change="(v: boolean) => toggleSelectAll(!!v)"
      >全选</CsCheckbox>
      <template v-if="selectedIds.length">
        <CsBadge color="blue"><span class="cs-text-mono">已选 {{ selectedIds.length }} 台</span></CsBadge>
        <CsButton icon-name="upload" @click="batchUpgradeSelected">批量升级</CsButton>
        <CsButton variant="link" @click="selectedIds = []">取消选择</CsButton>
      </template>
      <CsTooltip v-if="latestAgentVersion" content="官方最新 Agent 版本，点击刷新最新版本检测" placement="top">
        <CsButton
          variant="inline-link"
          icon-name="refresh"
          :loading="fetchingLatestAgent"
          @click="loadLatestAgentVersion(true)"
        >
          <span class="cs-text-mono">官方最新: {{ latestAgentVersion }}</span>
        </CsButton>
      </CsTooltip>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" @click="createOpen = true">新增服务器</CsButton>
      </template>
    </CsToolbar>

    <!-- 续费提醒横幅（仅在未专门过滤待续费时提示） -->
    <CsAlert v-if="expiringServers.length > 0 && statusFilter !== 'expiring'" type="warning">
      续费提醒：共 <strong>{{ expiringServers.length }}</strong> 台服务器已到期或将在 7 天内到期，请注意及时前往 IDC 控制台续费。
      <template #action>
        <CsButton @click="statusFilter = 'expiring'">仅看待续费服务器</CsButton>
      </template>
    </CsAlert>

    <CsContainer header="服务器列表">
      <template #actions>
        <CsSegmentedControl
          :model-value="statusFilter"
          :options="statusFilterOptions"
          label="按状态筛选服务器"
          @update:model-value="onStatusFilterChange"
        />
      </template>

      <div v-if="loading" class="server-loading"><CsSpinner size="large" /></div>

      <CsEmpty
        v-else-if="filtered.length === 0"
        icon-name="grid-view"
        :description="keyword ? '未找到匹配服务器' : '尚未添加服务器，点击右上角「新增服务器」'"
      />

      <CsCardGrid v-else>
        <CsCard v-for="row in filtered" :key="row.id">
          <template #header>
            <div class="server-card-head">
              <CsCheckbox
                :model-value="selectedIds.includes(row.id)"
                :aria-label="`选择 ${row.name}`"
                @change="(v: boolean) => toggleSelect(row.id, !!v)"
                @click.stop
              />
              <CsBadge color="grey"><span class="cs-text-mono">#{{ row.sort_order ?? 0 }}</span></CsBadge>
              <CsStatusIndicator
                :type="row.status === 1 ? 'success' : 'stopped'"
                :text="row.status === 1 ? '在线' : '离线'"
              />
              <CsButton variant="link" class="server-name" @click="openDrawer(row)">{{ row.name }}</CsButton>
              <CsBadge v-if="row.location" color="blue">{{ row.location }}</CsBadge>
              <CsTooltip
                v-if="getExpiryStatus(row.expire_at)"
                :content="getExpiryStatus(row.expire_at)!.tip"
                placement="top"
              >
                <CsBadge :color="expiryBadgeColor(row)" class="server-chip-help">
                  {{ getExpiryStatus(row.expire_at)!.text }}
                </CsBadge>
              </CsTooltip>
              <CsTooltip v-if="xrayAlertChip(row).show" :content="xrayAlertChip(row).tip" placement="top">
                <CsBadge :color="badgeColorOf(xrayAlertChip(row).cls)" class="server-chip-help">
                  {{ xrayAlertChip(row).text }}
                </CsBadge>
              </CsTooltip>
            </div>
          </template>

          <template #actions>
            <CsTooltip v-if="pushChip(row).tip" :content="pushChip(row).tip" placement="top">
              <CsBadge :color="badgeColorOf(pushChip(row).cls)">{{ pushChip(row).text }}</CsBadge>
            </CsTooltip>
            <CsBadge v-else :color="badgeColorOf(pushChip(row).cls)">{{ pushChip(row).text }}</CsBadge>
          </template>

          <CsCardFacts :columns="2">
            <CsCardFact label="服务器地址 / 域名" full-width>
              <CsButton
                variant="inline-link"
                icon-name="copy"
                class="server-host"
                @click="copyText(row.host, '服务器地址')"
              >
                <span class="cs-text-mono">{{ row.host }}</span>
              </CsButton>
            </CsCardFact>

            <CsCardFact label="Agent">
              <span class="server-inline">
                <span class="cs-text-mono">{{ row.agent_version || 'v—' }}</span>
                <CsBadge v-if="getAgentVersionStatus(row.agent_version).type === 'latest'" color="green">最新</CsBadge>
                <CsBadge
                  v-else-if="getAgentVersionStatus(row.agent_version).type === 'outdated'"
                  color="severity-medium"
                  :title="`官方最新版本 ${latestAgentVersion}`"
                >有新版</CsBadge>
                <CsButton
                  v-if="row.status === 1"
                  variant="inline-link"
                  :disabled="upgradingId === row.id"
                  @click="openVersionDialog(row, 'agent')"
                >{{ getAgentVersionStatus(row.agent_version).type === 'outdated' ? '升级' : '版本' }}</CsButton>
              </span>
            </CsCardFact>

            <CsCardFact label="Xray">
              <span class="server-inline">
                <span class="cs-text-mono">{{ row.xray_version || '—' }}</span>
                <CsButton
                  v-if="row.status === 1"
                  variant="inline-link"
                  :disabled="upgradingId === row.id"
                  @click="openVersionDialog(row, 'xray')"
                >版本</CsButton>
              </span>
            </CsCardFact>

            <CsCardFact label="接入点">
              <CsButton variant="inline-link" @click="goInbounds(row)">
                <span class="cs-text-mono">{{ inboundCount(row.id) }}</span> 个入站
              </CsButton>
            </CsCardFact>

            <CsCardFact label="心跳" mono :value="row.last_seen_at ? fmtTime(row.last_seen_at).slice(5, 16) : '—'" />

            <CsCardFact
              v-if="row.expire_at || row.billing_cycle || row.price || row.idc_address"
              label="VPS 资产"
              full-width
            >
              <div class="server-asset">
                <span
                  v-if="row.expire_at"
                  class="cs-text-mono"
                  :style="getExpiryStatus(row.expire_at)?.isUrgent ? { color: getExpiryStatus(row.expire_at)!.color, fontWeight: 600 } : {}"
                  :title="getExpiryStatus(row.expire_at)?.tip || `到期日：${formatExpireDate(row.expire_at)}`"
                >{{ formatExpireDate(row.expire_at) }}</span>
                <CsBadge v-if="row.billing_cycle" color="grey">{{ row.billing_cycle }}</CsBadge>
                <span v-if="row.price" class="cs-text-mono server-price">{{ row.price }}</span>
                <CsButton
                  v-if="row.idc_address && isUrl(row.idc_address)"
                  variant="inline-link"
                  icon-name="external"
                  :href="normalizeUrl(row.idc_address)"
                  target="_blank"
                >IDC 控制台</CsButton>
                <CsButton
                  v-else-if="row.idc_address"
                  variant="inline-link"
                  icon-name="copy"
                  @click="copyText(row.idc_address, 'IDC 地址')"
                >{{ row.idc_address }}</CsButton>
              </div>
            </CsCardFact>
          </CsCardFacts>

          <template #footer>
            <CsButton icon-name="settings" @click="openDrawer(row)">详情</CsButton>
            <CsButton icon-name="view-horizontal" @click="openMetrics(row)">监控</CsButton>
            <CsButton icon-name="refresh" @click="restartXray(row)">重启</CsButton>
            <CsButtonDropdown
              icon-only
              icon-name="ellipsis"
              aria-label="更多操作"
              :items="cardMenuItems(row)"
              @select="(cmd: string | number) => onMore(String(cmd), row)"
            />
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <!-- 新增服务器 -->
    <el-dialog v-model="createOpen" title="新增服务器" width="680px" @close="closeCreate">
      <template v-if="!createdResult">
        <el-form label-position="top">
          <div class="dialog-cards-stack">
            <!-- 基础设施 -->
            <div class="x-micro-card">
              <div class="card-caption">
                <span class="caption-dot emerald"></span>
                <span>基础设施与连接定位</span>
              </div>
              <div class="form-grid-2">
                <el-form-item label="服务器名称" required>
                  <el-input v-model="createForm.name" placeholder="如 Tokyo-01 / 广州移动 BGP" />
                </el-form-item>
                <el-form-item label="主机地址 / 域名" required>
                  <el-input v-model="createForm.host" placeholder="如 tokyo01.example.com / 120.232.x.x" />
                </el-form-item>
              </div>
              <div class="form-grid-3">
                <el-form-item label="排序序号">
                  <el-input-number v-model="createForm.sort_order" :min="0" :step="1" placeholder="越小越靠前" style="width: 100%" />
                </el-form-item>
                <el-form-item label="地区位置">
                  <el-input v-model="createForm.location" placeholder="如 日本 / 广州（选填）" />
                </el-form-item>
                <el-form-item label="管理备注">
                  <el-input v-model="createForm.remark" placeholder="选填" />
                </el-form-item>
              </div>
            </div>

            <!-- VPS 续费与资产信息 -->
            <div class="x-micro-card">
              <div class="card-caption">
                <span class="caption-dot blue"></span>
                <span>VPS 采购与续费资产（选填）</span>
              </div>
              <div class="form-grid-2">
                <el-form-item label="到期日期">
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
              </div>
              <div class="form-grid-2">
                <el-form-item label="续费价格">
                  <el-input v-model="createForm.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
                </el-form-item>
                <el-form-item label="IDC 控制台地址">
                  <el-input v-model="createForm.idc_address" placeholder="控制台网址，如 https://..." clearable />
                </el-form-item>
              </div>
            </div>
          </div>
        </el-form>
      </template>
      <template v-else>
        <el-alert type="success" :closable="false" show-icon title="服务器已创建" description="在服务器上以 root 执行以下一键安装命令：" style="margin-bottom: 12px" />
        <div class="secret-box">
          <div class="secret-row install-row">
            <span class="k">一键安装</span>
            <code class="install-cmd font-mono-num">{{ createdResult.install_cmd }}</code>
            <el-button size="small" text @click="copyText(createdResult.install_cmd, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
          </div>
          <div class="secret-row">
            <span class="k">node_id</span>
            <code class="font-mono-num">{{ createdResult.node_id }}</code>
            <el-button size="small" text @click="copyText(createdResult.node_id, 'node_id')"><el-icon><CopyDocument /></el-icon></el-button>
          </div>
          <div class="secret-row">
            <span class="k">secret</span>
            <code class="font-mono-num">{{ createdResult.secret }}</code>
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

    <!-- 版本切换：Agent / Xray 共用（升级/降级/重装） -->
    <el-dialog v-model="versionDialogOpen" :title="`${versionSubjectLabel} 版本`" width="560px">
      <div class="xray-version-dialog">
        <div class="xray-current-row">
          <span class="meta-label">当前{{ versionSubjectLabel }}</span>
          <code class="cell-mono">{{ versionCurrent || '未知（旧 Agent 未上报）' }}</code>
        </div>
        <el-form label-position="top" style="margin-top: 16px">
          <el-form-item label="目标版本">
            <el-select
              v-model="versionTarget"
              filterable
              allow-create
              default-first-option
              :loading="versionOptionsLoading"
              :placeholder="versionSubject === 'agent' ? '选择版本，或直接输入（如 v0.1.19）' : '选择版本，或直接输入（如 v26.10.21）'"
              style="width: 100%"
            >
              <el-option v-for="v in versionOptions" :key="v" :label="v" :value="v">
                <span class="cell-mono">{{ v }}</span>
                <span v-if="v === versionOptions[0]" class="x-chip emerald mini-chip" style="margin-left: 8px">最新</span>
              </el-option>
            </el-select>
            <div class="field-hint">{{ versionHint }}</div>
          </el-form-item>
        </el-form>
        <el-alert
          v-if="versionCompare !== null && versionCompare < 0"
          type="warning"
          :closable="false"
          show-icon
          :title="`降级${versionSubjectLabel}有风险`"
          :description="versionSubject === 'agent'
            ? '更老的 Agent 可能不认识主控新下发的协议消息（未知消息会被静默丢弃），请确认主控与之兼容。服务重启期间节点短暂离线。'
            : 'Xray ≥26.9.8 起 REALITY 强制要求 MLKEM keyshare，更老的内核也可能无法加载当前配置。安装失败会自动回滚，期间 xray 短暂中断。'"
        />
        <el-alert
          v-else
          type="info"
          :closable="false"
          show-icon
          :title="`将${versionActionLabel}${versionSubjectLabel}：${versionCurrent || '未知'} → ${versionTarget || '未选择'}`"
          :description="versionSubject === 'agent'
            ? '节点从 XPanel-Node Releases 下载并做 sha256 校验，完成后服务自动重启。'
            : '节点从官方 Releases 下载并做 SHA-256 校验；内核与规则库（geoip/geosite）一起切换。'"
        />
        <div class="xray-dialog-foot">
          <el-button size="small" :loading="versionOptionsLoading" @click="loadVersionOptions(true)">刷新版本列表</el-button>
          <span class="muted font-11">
            {{ versionSubject === 'agent' ? 'Agent 与内核版本相互独立，切换 Agent 不会改动 Xray。' : 'Agent 与内核版本相互独立，切换内核不会改动 Agent。' }}
          </span>
        </div>
      </div>
      <template #footer>
        <el-button @click="versionDialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="versionSwitching" :disabled="!versionTarget" @click="applyVersionSwitch">
          {{ versionActionLabel }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Agent 升级/回滚 / Xray 内核切换 监控弹窗 -->
    <el-dialog
      v-model="upgradeModalOpen"
      :title="upgradeTitle"
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
            <el-tag size="small" type="info">{{ upgradeFromLabel }}</el-tag>
            <span class="ver-arrow">→</span>
            <el-tag size="small" :type="upgradeIsRollback ? 'warning' : 'success'">
              {{ upgradeToLabel }}
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
          <el-step
            title="版本解析"
            :description="upgradeKind === 'xray' ? '确认目标内核' : (upgradeIsRollback ? '检查指定版本' : '检查远端版本')"
          />
          <el-step title="下载资源" :description="upgradeKind === 'xray' ? 'XTLS Releases' : 'GitHub Releases'" />
          <el-step title="完整性校验" description="SHA256 校验" />
          <el-step title="重启生效" :description="upgradeKind === 'xray' ? '重启 xray' : '服务重载就绪'" />
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
              服务器正在执行后台{{ upgradeKind === 'xray' ? '内核切换' : (upgradeIsRollback ? '回滚' : '升级') }}操作，若网络连通较慢请耐心等待（通常耗时 10-60 秒）
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
    <el-dialog v-model="editOpen" title="编辑服务器" width="680px">
      <el-form label-position="top">
        <div class="dialog-cards-stack">
          <!-- 基础设施 -->
          <div class="x-micro-card">
            <div class="card-caption">
              <span class="caption-dot emerald"></span>
              <span>基础设施与连接定位</span>
            </div>
            <div class="form-grid-2">
              <el-form-item label="服务器名称" required>
                <el-input v-model="editForm.name" />
              </el-form-item>
              <el-form-item label="主机地址 / 域名" required>
                <el-input v-model="editForm.host" placeholder="如 tokyo01.example.com" />
              </el-form-item>
            </div>
            <div class="form-grid-3">
              <el-form-item label="排序序号">
                <el-input-number v-model="editForm.sort_order" :min="0" :step="1" placeholder="越小越靠前" style="width: 100%" />
              </el-form-item>
              <el-form-item label="地区位置">
                <el-input v-model="editForm.location" placeholder="选填" />
              </el-form-item>
              <el-form-item label="管理备注">
                <el-input v-model="editForm.remark" placeholder="选填" />
              </el-form-item>
            </div>
          </div>

          <!-- VPS 续费与资产信息 -->
          <div class="x-micro-card">
            <div class="card-caption">
              <span class="caption-dot blue"></span>
              <span>VPS 采购与续费资产（选填）</span>
            </div>
            <div class="form-grid-2">
              <el-form-item label="到期日期">
                <el-date-picker
                  v-model="editForm.expire_at"
                  type="date"
                  placeholder="选择到期日期"
                  format="YYYY-MM-DD"
                  value-format="YYYY-MM-DD"
                  style="width: 100%"
                  clearable
                />
              </el-form-item>
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
            </div>
            <div class="form-grid-2">
              <el-form-item label="续费价格">
                <el-input v-model="editForm.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
              </el-form-item>
              <el-form-item label="IDC 控制台地址">
                <el-input v-model="editForm.idc_address" placeholder="控制台网址，如 https://..." clearable />
              </el-form-item>
            </div>
          </div>
        </div>
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
  </CsPage>
</template>

<style scoped lang="scss">
.cell-mono { font-family: var(--x-font-mono, ui-monospace, Menlo, Consolas, monospace); font-size: 12.5px; color: var(--x-text-2); }

/* 页面壳：Cloudscape 卡片内只保留必需的布局类 */
.server-loading {
  padding: 48px 0;
  text-align: center;
}

.server-card-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-inline-size: 0;
}

.server-name {
  font-size: 15px;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.server-host {
  max-inline-size: 100%;
  overflow-wrap: anywhere;
}

.server-inline {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.server-asset {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.server-price {
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk, #0972d3);
}

.server-chip-help {
  cursor: help;
}

/* 弹窗微卡片与工控表单栈 */
.dialog-cards-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.x-micro-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: var(--x-radius-sm, 8px);
  padding: 12px 14px;
}

.card-caption {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--x-text-1, #1e293b);
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 1px dashed var(--x-border, #e2e8f0);

  .caption-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    &.emerald { background: #10b981; }
    &.blue { background: #3b82f6; }
    &.amber { background: #f59e0b; }
  }
}

.form-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  @media (max-width: 600px) {
    grid-template-columns: 1fr;
  }
}

.form-grid-3 {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  @media (max-width: 600px) {
    grid-template-columns: 1fr;
  }
}
.muted { color: var(--x-text-3); }
.secret-box { display: grid; gap: 10px; }
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

/* ================= Xray 内核版本弹窗 ================= */
.xray-version-dialog {
  .xray-current-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    background: var(--x-fill-2, #f8fafc);
    border: 1px solid var(--x-border-light, #f1f5f9);
    border-radius: 8px;
  }
  .xray-dialog-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-top: 12px;
  }
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

</style>
