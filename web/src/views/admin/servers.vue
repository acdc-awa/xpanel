<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CopyDocument } from '@element-plus/icons-vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
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
  type SegmentedOption,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import ServerSettingsModal from './servers/ServerSettingsModal.vue'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'
import {
  batchUpgradeServers,
  createServer,
  getAgentLatestVersion,
  getBatchUpgradeStatus,
  getInbounds,
  getServers,
  serverCommand,
  type AgentUpgradeStatus,
  type ServerItem,
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
      // 设置弹窗持有的是快照，列表刷新后按 id 回填，保存/换版本后立即反映
      if (settingsServer.value) {
        settingsServer.value = list.value.find((s) => s.id === settingsServer.value!.id) ?? settingsServer.value
      }
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

// ---- 服务器设置弹窗 ----
const settingsOpen = ref(false)
const settingsServer = ref<ServerItem | null>(null)

function openSettings(row: ServerItem) {
  settingsServer.value = row
  settingsOpen.value = true
}

function onSettingsRemoved() {
  settingsOpen.value = false
  settingsServer.value = null
  load()
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
              <CsButton variant="link" class="server-name" @click="openSettings(row)">{{ row.name }}</CsButton>
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
              </span>
            </CsCardFact>

            <CsCardFact label="Xray">
              <span class="cs-text-mono">{{ row.xray_version || '—' }}</span>
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
            <CsButton icon-name="settings" @click="openSettings(row)">设置</CsButton>
            <CsButton icon-name="refresh" @click="restartXray(row)">重启</CsButton>
            <CsButton icon-name="view-horizontal" @click="openMetrics(row)">监控</CsButton>
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

    <!-- 服务器设置弹窗 -->
    <ServerSettingsModal
      v-if="settingsOpen"
      v-model="settingsOpen"
      :server="settingsServer"
      @removed="onSettingsRemoved"
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

}

</style>
