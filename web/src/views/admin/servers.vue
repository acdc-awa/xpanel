<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsCheckbox,
  CsColumnLayout,
  CsContainer,
  CsDatePicker,
  CsEmpty,
  CsForm,
  CsFormItem,
  CsIconButton,
  CsInput,
  CsInputNumber,
  CsModal,
  CsPage,
  CsProgressBar,
  CsSegmentedControl,
  CsSelect,
  CsSpaceBetween,
  CsSpinner,
  CsStatusIndicator,
  CsTable,
  CsToolbar,
  CsTooltip,
  ElMessage,
  ElMessageBox,
  type SegmentedOption,
  type TableColumn,
  type BadgeColor,
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
import { pushStateChip, xrayAlertChip } from '@/utils/server-status'

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

// 列表里的 Xray / 配置同步徽标统一走 utils/server-status（与设置弹窗同一份文案与配色），
// 页面内不再各写一套 chip 逻辑。


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
      // 下次打开路由画布时定位到这张新卡片（不强制跳页，避免打断安装流程）
      try {
        sessionStorage.setItem('topology-focus-id', `server-${data.data.server.id}`)
      } catch {
        /* 隐私模式等场景忽略 */
      }
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

const batchColumns: TableColumn[] = [
  { id: 'name', header: '服务器', key: 'name', minWidth: 140 },
  { id: 'version', header: '当前版本', width: 110 },
  { id: 'status', header: '状态', width: 120 },
  { id: 'progress', header: '进度', minWidth: 200 },
]

const billingCycleOptions = [
  { label: '月付', value: '月付' },
  { label: '季付', value: '季付' },
  { label: '半年付', value: '半年付' },
  { label: '年付', value: '年付' },
  { label: '两年付', value: '两年付' },
  { label: '三年付', value: '三年付' },
  { label: '一次性 / 永久', value: '一次性' },
]
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

function progressStatus(phase?: string): 'success' | 'error' | 'in-progress' {
  if (phase === 'success') return 'success'
  if (phase === 'failed') return 'error'
  return 'in-progress'
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
                <CsBadge :color="xrayAlertChip(row).color" class="server-chip-help">
                  {{ xrayAlertChip(row).text }}
                </CsBadge>
              </CsTooltip>
            </div>
          </template>

          <template #actions>
            <CsTooltip v-if="pushStateChip(row).tip" :content="pushStateChip(row).tip" placement="top">
              <CsBadge :color="pushStateChip(row).color">{{ pushStateChip(row).text }}</CsBadge>
            </CsTooltip>
            <CsBadge v-else :color="pushStateChip(row).color">{{ pushStateChip(row).text }}</CsBadge>
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
    <CsModal v-model:visible="createOpen" header="新增服务器" size="large" @close="closeCreate">
      <CsForm v-if="!createdResult" label-position="top">
        <CsSpaceBetween size="m">
          <CsContainer header="基础设施与连接定位">
            <CsSpaceBetween size="m">
              <CsColumnLayout :columns="2">
                <CsFormItem label="服务器名称" required>
                  <CsInput v-model="createForm.name" placeholder="如 Tokyo-01 / 广州移动 BGP" />
                </CsFormItem>
                <CsFormItem label="主机地址 / 域名" required>
                  <CsInput v-model="createForm.host" placeholder="如 tokyo01.example.com / 120.232.x.x" />
                </CsFormItem>
              </CsColumnLayout>
              <CsColumnLayout :columns="3">
                <CsFormItem label="排序序号">
                  <CsInputNumber
                    v-model="createForm.sort_order"
                    :min="0"
                    :step="1"
                    placeholder="越小越靠前"
                  />
                </CsFormItem>
                <CsFormItem label="地区位置">
                  <CsInput v-model="createForm.location" placeholder="如 日本 / 广州（选填）" />
                </CsFormItem>
                <CsFormItem label="管理备注">
                  <CsInput v-model="createForm.remark" placeholder="选填" />
                </CsFormItem>
              </CsColumnLayout>
            </CsSpaceBetween>
          </CsContainer>

          <CsContainer header="VPS 采购与续费资产（选填）">
            <CsSpaceBetween size="m">
              <CsColumnLayout :columns="2">
                <CsFormItem label="到期日期">
                  <CsDatePicker
                    v-model="createForm.expire_at"
                    type="date"
                    value-format="YYYY-MM-DD"
                    placeholder="选择到期日期"
                    clearable
                  />
                </CsFormItem>
                <CsFormItem label="计费周期">
                  <CsSelect
                    v-model="createForm.billing_cycle"
                    :options="billingCycleOptions"
                    filterable
                    allow-create
                    clearable
                    placeholder="选择或输入计费周期"
                  />
                </CsFormItem>
              </CsColumnLayout>
              <CsColumnLayout :columns="2">
                <CsFormItem label="续费价格">
                  <CsInput v-model="createForm.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
                </CsFormItem>
                <CsFormItem label="IDC 控制台地址">
                  <CsInput v-model="createForm.idc_address" placeholder="控制台网址，如 https://..." clearable />
                </CsFormItem>
              </CsColumnLayout>
            </CsSpaceBetween>
          </CsContainer>
        </CsSpaceBetween>
      </CsForm>

      <CsSpaceBetween v-else size="s">
        <CsAlert type="success" header="服务器已创建">
          在服务器上以 root 执行以下一键安装命令。进入「路由管理 → 画布」时会自动定位这张新卡片。
        </CsAlert>
        <div class="secret-box">
          <div class="secret-row install-row">
            <span class="k">一键安装</span>
            <code class="install-cmd cs-text-mono">{{ createdResult.install_cmd }}</code>
            <CsIconButton
              icon-name="copy"
              label="复制安装命令"
              @click="copyText(createdResult.install_cmd, '安装命令')"
            />
          </div>
          <div class="secret-row">
            <span class="k">node_id</span>
            <code class="cs-text-mono">{{ createdResult.node_id }}</code>
            <CsIconButton
              icon-name="copy"
              label="复制 node_id"
              @click="copyText(createdResult.node_id, 'node_id')"
            />
          </div>
          <div class="secret-row">
            <span class="k">secret</span>
            <code class="cs-text-mono">{{ createdResult.secret }}</code>
            <CsIconButton
              icon-name="copy"
              label="复制 secret"
              @click="copyText(createdResult.secret, 'secret')"
            />
            <TipIcon
              type="warn"
              content="secret 仅显示这一次；安装完成后回到本页查看在线状态，在服务器详情中生成并下发配置。"
            />
          </div>
        </div>
      </CsSpaceBetween>

      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <template v-if="!createdResult">
            <CsButton @click="createOpen = false">取消</CsButton>
            <CsButton variant="primary" :loading="creating" @click="submitCreate">创建</CsButton>
          </template>
          <CsButton v-else variant="primary" @click="closeCreate">完成</CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>

    <!-- 批量升级总览弹窗 -->
    <CsModal
      v-model:visible="batchOpen"
      header="批量升级 Agent"
      size="large"
      :close-on-click-overlay="false"
      @close="stopBatchPolling"
    >
      <CsSpaceBetween size="s">
        <CsTable :items="batchRows" :columns="batchColumns" track-by="id" empty-text="暂无服务器">
          <template #cell-version="{ item }">
            <span class="cs-text-mono">{{ item.version }}</span>
          </template>
          <template #cell-status="{ item }">
            <CsBadge v-if="item.skipped" color="grey">已跳过</CsBadge>
            <CsBadge
              v-else-if="item.status"
              :color="
                item.status.phase === 'success'
                  ? 'green'
                  : item.status.phase === 'failed'
                    ? 'red'
                    : 'blue'
              "
            >
              {{ phaseMeta[item.status.phase]?.label || item.status.phase }}
            </CsBadge>
            <CsBadge v-else color="grey">等待</CsBadge>
          </template>
          <template #cell-progress="{ item }">
            <span v-if="item.skipped" class="cs-text-secondary">{{ item.reason }}</span>
            <template v-else-if="item.status">
              <CsProgressBar
                :value="phaseMeta[item.status.phase]?.percent ?? 0"
                :status="progressStatus(item.status.phase)"
                :bar-height="8"
              />
              <div v-if="item.status.error" class="batch-error cs-text-mono">{{ item.status.error }}</div>
              <div v-else-if="item.status.message" class="batch-message cs-text-secondary">
                {{ item.status.message }}
              </div>
            </template>
            <span v-else class="cs-text-secondary">—</span>
          </template>
        </CsTable>
        <p class="batch-hint cs-text-secondary">
          <template v-if="batchRunning">
            正在升级，进度实时更新…升级完成的服务器将在下一轮心跳后刷新版本号。
          </template>
          <template v-else>全部服务器已处理完成；升级完成的服务器版本号将在下一轮心跳后刷新。</template>
        </p>
      </CsSpaceBetween>
      <template #footer>
        <CsButton variant="primary" @click="batchOpen = false">关闭</CsButton>
      </template>
    </CsModal>

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

/* 弹窗内的安装凭据块 */
.secret-box { display: grid; gap: 10px; }
.install-cmd { font-size: 11.5px; }
.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  border-radius: 8px;
  padding: 10px 12px;

  .k {
    color: var(--color-text-body-secondary-66ygbb, #656871);
    font-size: 12.5px;
    flex: none;
    inline-size: 56px;
  }

  code {
    flex: 1;
    color: var(--color-text-link-default-qabnuk, #0972d3);
    word-break: break-all;
  }
}

.batch-error {
  margin-block-start: 2px;
  font-size: 11px;
  color: var(--color-text-status-error-ri38gt, #db0000);
  word-break: break-all;
}
.batch-message {
  margin-block-start: 2px;
  font-size: 11px;
}
.batch-hint {
  font-size: 12px;
  margin: 0;
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
