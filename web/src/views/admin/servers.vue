<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsButton,
  CsCardGrid,
  CsCheckbox,
  CsColumnLayout,
  CsContainer,
  CsDatePicker,
  CsDrawer,
  CsEmpty,
  CsForm,
  CsFormItem,
  CsHeader,
  CsIconButton,
  CsInput,
  CsModal,
  CsPage,
  CsSegmentedControl,
  CsSelect,
  CsSpaceBetween,
  CsSpinner,
  CsToolbar,
  ElMessage,
  ElMessageBox,
  type SegmentedOption,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import ServerCard from './servers/ServerCard.vue'
import BatchActionModal, {
  type BatchActionItem,
  type BatchActionKind,
  type BatchActionRequest,
} from './servers/BatchActionModal.vue'
import XrayUpgradeDialog from './servers/XrayUpgradeDialog.vue'
import ServerSettingsModal from './servers/ServerSettingsModal.vue'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'
import {
  createServer,
  getAgentLatestVersion,
  getInbounds,
  getServers,
  getServerUpgradeStatuses,
  getXrayVersions,
  reorderServers,
  runServerCommand,
  type CommandDispatchResult,
  type ServerItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { compareVersion } from '@/utils/version'
import { getExpiryStatus } from '@/utils/vps'

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

// ---- 官方版本感知（Agent / Xray） ----
const latestAgentVersion = ref('')
const latestXrayVersion = ref('')
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

async function loadLatestXrayVersion() {
  try {
    const { data } = await getXrayVersions()
    if (data.code === 0 && data.data?.latest_version) {
      latestXrayVersion.value = data.data.latest_version
    }
  } catch {
    // 静默降级：没有参考版本时不显示「有新版」角标
  }
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
  loadLatestXrayVersion()
}
onMounted(load)

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
      // 下次打开路由画布时定位到这张新卡片（不强制跳页，避免打断安装流程）
      try {
        sessionStorage.setItem('topology-focus-id', `server-${data.data.server.id}`)
      } catch {
        /* 隐私模式等场景忽略 */
      }
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

// ---- 单选 / 多选 ----
const selectedIds = ref<number[]>([])
const selectMode = ref(false)
const selectedServers = computed(() => list.value.filter((s) => selectedIds.value.includes(s.id)))
const selectedOfflineCount = computed(() => selectedServers.value.filter((s) => s.status !== 1).length)
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

function toggleSelectMode() {
  selectMode.value = !selectMode.value
  selectedIds.value = []
  actionDrawerOpen.value = false
}

// ---- 移动端：批量操作收进底部操作栏 + 抽屉 ----
// 桌面把三个批量按钮放在列表标题行；768 以下标题行放不下，改为常驻底部操作栏，
// 具体操作在抽屉里统一呈现（升级 Agent / 升级 Xray / 重启 Xray 都是「对所选服务器下发」同一语义）。
const isMobile = ref(false)
const actionDrawerOpen = ref(false)
const drawerTopOffset = ref(0)
let mobileMq: MediaQueryList | null = null

const onMobileMq = () => {
  isMobile.value = window.matchMedia('(max-width: 768px)').matches || window.innerWidth <= 768
  if (!isMobile.value) actionDrawerOpen.value = false
}

onMounted(() => {
  mobileMq = window.matchMedia('(max-width: 768px)')
  onMobileMq()
  mobileMq.addEventListener('change', onMobileMq)
  // matchMedia 为主；再挂 resize 兜住不派发 change 的嵌入环境（IAB/WebView）
  window.addEventListener('resize', onMobileMq)
})
onUnmounted(() => {
  mobileMq?.removeEventListener('change', onMobileMq)
  window.removeEventListener('resize', onMobileMq)
})

/** 抽屉停在应用栏下沿：与 AdminLayout 的移动端导航抽屉同一口径（读 AppLayout 下发的 header 高度）。 */
function openActionDrawer() {
  const page = document.querySelector('.cs-page')
  const raw = page ? getComputedStyle(page).getPropertyValue('--awsui-header-height-ka4djm') : ''
  drawerTopOffset.value = parseInt(raw, 10) || 0
  actionDrawerOpen.value = true
}

/** 抽屉里点操作：先关抽屉再开对应流程，避免两层浮层叠加。 */
function runBatchFromDrawer(fn: () => void) {
  actionDrawerOpen.value = false
  fn()
}

const MAX_BATCH = 100

/** 后端 binding 上限 100；超了直接拦住，不让请求白跑一趟。 */
function tooMany(): boolean {
  if (selectedIds.value.length > MAX_BATCH) {
    ElMessage.warning(`一次最多操作 ${MAX_BATCH} 台服务器`)
    return true
  }
  return false
}

// ---- 排序（拖拽）----
// 唯一写入点：拖完一次性提交全量顺序，dashboard 的服务器监测按同一个 sort_order 取数。
// 筛选/搜索/多选时禁用——那几种状态下「插到哪」没有确定语义。
const canReorder = computed(
  () =>
    !selectMode.value &&
    !keyword.value.trim() &&
    statusFilter.value === 'all' &&
    !filterExpiring.value &&
    list.value.length > 1,
)

let draggingId: number | null = null
let orderSnapshot: ServerItem[] = []
let savingOrder = false
let scrollTimer: number | null = null
let scrollStep = 0

function cardElOf(target: EventTarget | null): HTMLElement | null {
  return (target as HTMLElement | null)?.closest('[data-server-id]') as HTMLElement | null
}

function onDragStart(e: DragEvent) {
  if (!canReorder.value || !(e.target as HTMLElement | null)?.closest('[data-drag-handle]')) {
    e.preventDefault()
    return
  }
  const card = cardElOf(e.target)
  if (!card) return
  draggingId = Number(card.getAttribute('data-server-id'))
  orderSnapshot = [...list.value]
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(draggingId))
    // 整卡跟随鼠标，而不是只拖一个小抓手
    e.dataTransfer.setDragImage(card, 24, 24)
  }
}

function onDragOver(e: DragEvent) {
  if (draggingId === null) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  updateAutoScroll(e.clientY)
  const card = cardElOf(e.target)
  if (!card) return
  const targetId = Number(card.getAttribute('data-server-id'))
  if (!targetId || targetId === draggingId) return
  const from = list.value.findIndex((s) => s.id === draggingId)
  if (from < 0) return
  const rect = card.getBoundingClientRect()
  const after = e.clientX > rect.left + rect.width / 2
  const next = [...list.value]
  const [moved] = next.splice(from, 1)
  let insertAt = next.findIndex((s) => s.id === targetId)
  if (insertAt < 0) return
  if (after) insertAt += 1
  next.splice(insertAt, 0, moved)
  // 只在顺序真的变了才赋值，避免每次 dragover 都触发整树渲染
  if (next.some((s, i) => s.id !== list.value[i]?.id)) list.value = next
}

function onDrop(e: DragEvent) {
  if (draggingId === null) return
  e.preventDefault()
  void persistOrder()
}

function onDragEnd() {
  stopAutoScroll()
  draggingId = null
  void persistOrder()
}

async function persistOrder() {
  if (savingOrder) return
  const ids = list.value.map((s) => s.id)
  if (ids.join(',') === orderSnapshot.map((s) => s.id).join(',')) return
  savingOrder = true
  try {
    const { data } = await reorderServers(ids)
    if (data.code !== 0) throw new Error(data.message || '保存排序失败')
  } catch (e) {
    list.value = orderSnapshot
    ElMessage.error(errMsg(e, '保存排序失败，已恢复原顺序'))
  } finally {
    savingOrder = false
    orderSnapshot = [...list.value]
  }
}

function updateAutoScroll(clientY: number) {
  const edge = 24
  scrollStep = clientY < edge ? -14 : clientY > window.innerHeight - edge ? 14 : 0
  if (scrollStep && scrollTimer === null) {
    scrollTimer = window.setInterval(() => window.scrollBy(0, scrollStep), 16)
  } else if (!scrollStep) {
    stopAutoScroll()
  }
}

function stopAutoScroll() {
  if (scrollTimer !== null) {
    clearInterval(scrollTimer)
    scrollTimer = null
  }
  scrollStep = 0
}

onUnmounted(stopAutoScroll)

// ---- 批量动作 ----
const batchVisible = ref(false)
const batchKind = ref<BatchActionKind>('agent')
const batchItems = ref<BatchActionItem[]>([])
const batchRequest = ref<BatchActionRequest | null>(null)

function openBatch(kind: BatchActionKind, items: BatchActionItem[], request: BatchActionRequest) {
  batchKind.value = kind
  batchItems.value = items
  batchRequest.value = request
  batchVisible.value = true
}

function onBatchFinished() {
  load()
  // Agent 升级完成后要等重启与下一次心跳，版本号才会刷新
  setTimeout(load, 15000)
}

async function batchAgentUpgrade() {
  if (!selectedIds.value.length || tooMany()) return
  const selected = selectedServers.value
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
  openBatch(
    'agent',
    selected.map((s) => ({ id: s.id, name: s.name, version: s.agent_version || '未知' })),
    { ids: selected.map((s) => s.id), type: 'upgrade_agent', target: latest || undefined, force: false },
  )
}

// ---- 批量升级 Xray：先选版本，再走统一进度弹窗 ----
const xrayDialogOpen = ref(false)

function openXrayUpgrade() {
  if (!selectedIds.value.length || tooMany()) return
  xrayDialogOpen.value = true
}

function startXrayUpgrade(target: string) {
  xrayDialogOpen.value = false
  if (!selectedIds.value.length) return
  const selected = selectedServers.value
  openBatch(
    'xray',
    selected.map((s) => ({ id: s.id, name: s.name, version: s.xray_version || '未知' })),
    { ids: selected.map((s) => s.id), type: 'upgrade_xray', target },
  )
}

async function batchRestartXray() {
  if (!selectedIds.value.length || tooMany()) return
  const selected = selectedServers.value
  const offline = selected.filter((s) => s.status !== 1).length
  const lines = [
    `已选 ${selected.length} 台服务器，将并行下发重启指令。`,
    offline ? `其中 ${offline} 台离线，将自动跳过。` : '全部服务器当前在线。',
    '重启会短暂中断该节点上的 Xray（约 1-2 秒）。',
  ]
  try {
    await ElMessageBox.confirm(lines.join('\n'), '批量重启 Xray', {
      type: 'warning',
      confirmButtonText: '开始重启',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  openBatch(
    'restart',
    selected.map((s) => ({ id: s.id, name: s.name, version: s.xray_version || '—' })),
    { ids: selected.map((s) => s.id), type: 'restart_xray' },
  )
}

// ---- 单台重启：派发后短轮询，终态再提示 ----
async function restartXray(row: ServerItem) {
  try {
    await ElMessageBox.confirm(`确认重启服务器「${row.name}」的 Xray？约 1-2 秒断线。`, '重启 Xray', {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const { data } = await runServerCommand<CommandDispatchResult>({ ids: [row.id], type: 'restart_xray' })
    if (data.code !== 0) {
      ElMessage.error(data.message)
      return
    }
    const skip = data.data.skipped?.[0]
    if (skip) {
      ElMessage.error(`重启失败：${skip.reason}`)
      return
    }
    const result = await waitRestart(row.id)
    if (result.ok) ElMessage.success(result.message || '已重启')
    else ElMessage.error(result.message || '重启失败')
    load()
  } catch (e) {
    ElMessage.error(errMsg(e, '重启失败'))
  }
}

async function waitRestart(id: number): Promise<{ ok: boolean; message?: string }> {
  for (let i = 0; i < 30; i++) {
    await new Promise((resolve) => setTimeout(resolve, 500))
    try {
      const { data } = await getServerUpgradeStatuses([id])
      const st = data.data?.statuses?.[String(id)]
      if (st && st.kind === 'restart') {
        if (st.phase === 'success') return { ok: true, message: st.message }
        if (st.phase === 'failed') return { ok: false, message: st.error || st.message }
      }
    } catch {
      /* 轮询抖动忽略 */
    }
  }
  return { ok: true, message: '指令已下发，稍后刷新查看' }
}

const billingCycleOptions = [
  { label: '月付', value: '月付' },
  { label: '季付', value: '季付' },
  { label: '半年付', value: '半年付' },
  { label: '年付', value: '年付' },
  { label: '两年付', value: '两年付' },
  { label: '三年付', value: '三年付' },
  { label: '一次性 / 永久', value: '一次性' },
]

</script>

<template>
  <CsPage :class="{ 'servers-page--action-bar': selectMode && isMobile }">
    <CsToolbar>
      <CsInput
        v-model="keyword"
        class="server-search"
        placeholder="搜索名称 / 地址 / 地区"
        prefix-icon="search"
        clearable
        :style="isMobile ? undefined : 'width: 240px'"
        aria-label="搜索服务器"
      />
      <CsButton v-if="isMobile" class="server-refresh-inline" variant="icon" icon-name="refresh" aria-label="刷新服务器列表" @click="load" />
      <CsButton v-else icon-name="refresh" @click="load">刷新</CsButton>
      <!-- 「多选 / 取消多选」与全选、已选计数都在列表标题行（见 CsContainer #actions），
           工具栏只放搜索、刷新与新增。官方最新版本也只在卡片状态条里提示（Agent/Xray 有新版）。
           窄屏的新增按钮放在主行末尾：`#actions` 是独立 flex 项，留着会另起一行只剩一个按钮。 -->
      <CsButton
        v-if="!selectMode && isMobile"
        class="server-add-inline"
        variant="primary"
        icon-name="add-plus"
        aria-label="新增服务器"
        @click="createOpen = true"
      />
      <template #actions>
        <CsButton
          v-if="!selectMode && !isMobile"
          variant="primary"
          icon-name="add-plus"
          @click="createOpen = true"
        >新增服务器</CsButton>
      </template>
    </CsToolbar>

    <!-- 续费提醒横幅（仅在未专门过滤待续费时提示） -->
    <CsAlert v-if="expiringServers.length > 0 && statusFilter !== 'expiring'" type="warning">
      续费提醒：共 <strong>{{ expiringServers.length }}</strong> 台服务器已到期或将在 7 天内到期，请注意及时前往 IDC 控制台续费。
      <template #action>
        <CsButton @click="statusFilter = 'expiring'">仅看待续费服务器</CsButton>
      </template>
    </CsAlert>

    <CsContainer>
      <!-- 标题行：左侧标题 + 紧贴标题的「多选 / 取消多选」（多选态再加全选与蓝色计数），
           右侧常态放状态筛选、多选态放三个批量动作（窄屏改由底部操作栏 + 抽屉承载）。
           注意：CsContainer 的 #actions 只挂在它自带的默认标题上，一旦传入 #header 就整块不渲染，
           所以右侧控件必须写在这一行里，不能再用 #actions。 -->
      <template #header>
        <div class="server-list-head">
          <span class="server-list-title"><CsHeader variant="h2">服务器列表</CsHeader></span>
          <CsButton
            :variant="selectMode ? 'primary' : 'normal'"
            icon-name="check"
            @click="toggleSelectMode"
          >{{ selectMode ? '取消多选' : '多选' }}</CsButton>
          <template v-if="selectMode && !isMobile">
            <CsCheckbox
              :model-value="allSelected"
              :indeterminate="selectedIds.length > 0 && !allSelected"
              :disabled="filtered.length === 0"
              @change="(v: boolean) => toggleSelectAll(!!v)"
            >全选</CsCheckbox>
            <span class="server-selected-count">已选 {{ selectedIds.length }} 台服务器</span>
          </template>

          <div v-if="!selectMode || !isMobile" class="server-list-tools">
            <CsSegmentedControl
              v-if="!selectMode"
              :model-value="statusFilter"
              :options="statusFilterOptions"
              label="按状态筛选服务器"
              @update:model-value="onStatusFilterChange"
            />
            <div v-else-if="!isMobile" class="server-select-actions">
              <CsButton icon-name="upload" :disabled="!selectedIds.length" @click="batchAgentUpgrade">升级 Agent</CsButton>
              <CsButton icon-name="upload" :disabled="!selectedIds.length" @click="openXrayUpgrade">升级 Xray</CsButton>
              <CsButton icon-name="refresh" :disabled="!selectedIds.length" @click="batchRestartXray">重启 Xray</CsButton>
            </div>
          </div>
        </div>
      </template>

      <div v-if="loading" class="server-loading"><CsSpinner size="large" /></div>

      <CsEmpty
        v-else-if="filtered.length === 0"
        icon-name="grid-view"
        :description="keyword ? '未找到匹配服务器' : '尚未添加服务器，点击右上角「新增服务器」'"
      />

      <CsCardGrid
        v-else
        :min-width="360"
        @dragstart="onDragStart"
        @dragover="onDragOver"
        @drop="onDrop"
        @dragend="onDragEnd"
      >
        <ServerCard
          v-for="row in filtered"
          :key="row.id"
          :server="row"
          :selected="selectedIds.includes(row.id)"
          :select-mode="selectMode"
          :can-reorder="canReorder"
          :latest-agent-version="latestAgentVersion"
          :latest-xray-version="latestXrayVersion"
          :inbound-count="inboundCount(row.id)"
          @toggle-select="toggleSelect(row.id, !selectedIds.includes(row.id))"
          @open-settings="openSettings(row)"
          @restart="restartXray(row)"
          @metrics="openMetrics(row)"
          @go-inbounds="goInbounds(row)"
          @copy-text="copyText"
        />
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
              <CsColumnLayout :columns="2">
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

    <!-- 批量动作进度（Agent 升级 / Xray 升级 / Xray 重启共用） -->
    <BatchActionModal
      v-model:visible="batchVisible"
      :kind="batchKind"
      :items="batchItems"
      :request="batchRequest"
      @finished="onBatchFinished"
    />

    <!-- Xray 版本选择（批量升级） -->
    <XrayUpgradeDialog
      v-model:visible="xrayDialogOpen"
      :count="selectedIds.length"
      :offline-count="selectedOfflineCount"
      @start="startXrayUpgrade"
    />

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

    <!-- 移动端多选：常驻底部操作栏（蓝色计数 + 全选 + 批量操作入口） -->
    <div v-if="selectMode && isMobile" class="server-action-bar">
      <span class="server-selected-count">已选 {{ selectedIds.length }} 台服务器</span>
      <CsButton variant="inline-link" :disabled="filtered.length === 0" @click="toggleSelectAll(!allSelected)">
        {{ allSelected ? '取消全选' : '全选' }}
      </CsButton>
      <CsButton variant="primary" :disabled="selectedIds.length === 0" @click="openActionDrawer">批量操作</CsButton>
    </div>

    <!-- 移动端批量操作抽屉：三个动作语义相同（对所选服务器下发指令），统一成一份操作栏 -->
    <CsDrawer
      v-model:visible="actionDrawerOpen"
      header="批量操作"
      placement="end"
      :size="320"
      :top-offset="drawerTopOffset"
    >
      <div class="server-action-list">
        <p class="server-action-summary">
          已选 <strong>{{ selectedIds.length }}</strong> 台服务器<span v-if="selectedOfflineCount > 0">，其中离线 {{ selectedOfflineCount }} 台</span>。
          离线与无需变更的服务器会自动跳过。
        </p>
        <button
          type="button"
          class="server-action-item"
          :disabled="selectedIds.length === 0"
          @click="runBatchFromDrawer(batchAgentUpgrade)"
        >
          <span class="server-action-title">升级 Agent</span>
          <span class="server-action-desc">升级到官方最新 Agent 版本</span>
        </button>
        <button
          type="button"
          class="server-action-item"
          :disabled="selectedIds.length === 0"
          @click="runBatchFromDrawer(openXrayUpgrade)"
        >
          <span class="server-action-title">升级 Xray</span>
          <span class="server-action-desc">选择目标内核版本，可降级或重装</span>
        </button>
        <button
          type="button"
          class="server-action-item"
          :disabled="selectedIds.length === 0"
          @click="runBatchFromDrawer(batchRestartXray)"
        >
          <span class="server-action-title">重启 Xray</span>
          <span class="server-action-desc">只重启内核进程，不重新下发配置</span>
        </button>
      </div>
    </CsDrawer>
  </CsPage>
</template>

<style scoped lang="scss">

/* 页面壳：Cloudscape 卡片内只保留必需的布局类 */
.server-loading {
  padding: 48px 0;
  text-align: center;
}

/* 搜索框：桌面固定 240px；窄屏与其余控件共处一行，搜索让出宽度先收缩 */
.server-search {
  flex: 1 1 200px;
  min-inline-size: 0;
}

/* 注意：窄屏的刷新与新增两个图标按钮都要靠右，只能**一个**元素吃 auto 外边距。
   两处都写会把剩余空白摊成两段（搜索↔刷新、刷新↔新增），三个控件被拉散。 */
@media (max-width: 768px) {
  /* .cs-toolbar-main 默认按内容宽度收缩，搜索框的 flex-grow 无处可长，
     结果刷新与新增挤在搜索框右侧、整行右侧留空。窄屏让它占满一行。 */
  :deep(.cs-toolbar-main) {
    flex: 1 1 auto;
  }

  /* #actions 的槽在窄屏是空的（新增按钮改到主行内），但空壳仍占一个 gap，
     会让最右的「+」离视口边缘差 12px；空壳直接不参与布局。 */
  :deep(.cs-toolbar-actions:empty) {
    display: none;
  }

  .server-search {
    /* basis 只决定换行判定（换行按 basis 算，不按收缩后的实际宽度）：
       工具栏此时只剩 搜索 + 刷新 28 + 新增 32 两项按钮，320 宽（可用 265）也排得进一行。
       上限与桌面同宽：整行拉满会让搜索框过宽，留白由下面的 auto 外边距吃掉。 */
    flex: 1 1 120px;
    max-inline-size: 240px;
  }

  /* 搜索被限宽后，剩余空间给刷新按钮左侧，两个图标按钮保持贴右边 */
  .server-refresh-inline {
    margin-inline-start: auto;
  }

  /* 原生 input 的 min-width:auto 会把它顶回 ~129px（默认 20 字符），放开才能在 320 下继续收缩 */
  .server-search :deep(input) {
    min-inline-size: 0;
  }
}

/* 底部操作栏出现时给页面留出高度，最后一张卡片不被压住 */
.servers-page--action-bar {
  padding-block-end: 64px;
}

.server-action-bar {
  position: fixed;
  inset-inline: 0;
  inset-block-end: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  /* 计数与两个动作整体靠右：底栏左侧留给页面内容，拇指操作区在右下 */
  justify-content: flex-end;
  gap: 12px;
  padding: 10px 16px calc(10px + env(safe-area-inset-bottom, 0px));
  background: var(--color-background-container-content-mjfil9, #fff);
  border-block-start: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  box-shadow: 0 -2px 10px rgba(0, 0, 0, 0.08);
}

/* 已选计数：蓝色文字，不做徽标/状态卡 */
.server-selected-count {
  font-size: 14px;
  line-height: 20px;
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk, #0972d3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* 标题行：标题 + 紧贴标题的多选开关（官方标题不带外边距，这里只补横向间距） */
.server-list-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  min-inline-size: 0;

  /* 官方 Header 根节点自带 width:100%：外面套一层只占内容宽的壳，否则它会独占整行把按钮挤到第二行 */
  .server-list-title {
    display: inline-flex;
    flex: 0 0 auto;
    min-inline-size: 0;
  }
}

/* 标题行右侧：常态是状态筛选，多选态是三个批量动作。撑到行尾，窄到放不下时整组换行仍靠右。 */
.server-list-tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-inline-start: auto;
  min-inline-size: 0;
}

.server-action-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.server-action-summary {
  margin: 0 0 4px;
  font-size: 13px;
  line-height: 19px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-action-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  inline-size: 100%;
  padding: 12px 14px;
  border: 1px solid var(--color-border-item-card-hcapa2, #c6c6cd);
  border-radius: 8px;
  background: transparent;
  font-family: inherit;
  text-align: start;
  cursor: pointer;

  &:hover:not(:disabled) {
    border-color: var(--color-text-link-default-qabnuk, #0972d3);
    background: var(--color-background-button-normal-hover-1ajrtx, rgba(9, 114, 211, 0.06));
  }

  &:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }
}

.server-action-title {
  font-size: 15px;
  line-height: 21px;
  font-weight: 700;
  color: var(--color-text-link-default-qabnuk, #0972d3);
}

.server-action-desc {
  font-size: 13px;
  line-height: 19px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-select-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.server-select-hint {
  margin-inline-end: 4px;
  font-size: 13px;
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
