<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import {
  CsBadge,
  CsButton,
  CsContainer,
  CsForm,
  CsFormField,
  CsInput,
  CsModal,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsTable,
  CsTabs,
  CsToggle,
  CsToolbar,
  CsTooltip,
  toast,
  messageBox,
  type TabItem,
  type TableColumn,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from './servers/InboundConfigEditor.vue'
import AccessPointDialog from './servers/AccessPointDialog.vue'
import {
  createInbound,
  deleteInbound,
  getInbounds,
  getPermissionGroups,
  toggleInbound,
  updateInbound,
  updateAccessPoint,
  getAccessPoints,
  type InboundItem,
  type InboundPayload,
  type PermissionGroup,
  type UserAccessPoint,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'
import { formatDateOnly } from '@/utils/timezone'
import { useServerScope } from '@/composables/useServerScope'

const route = useRoute()
const router = useRouter()

// 入站页保留「全部服务器」语义：未选服务器时展示所有服务器的入站。
const { servers, serverFilter, serverLoading, loadServers, syncServerQuery } = useServerScope({
  autoSelectFirst: false,
})

const tabs: TabItem[] = [
  { id: 'inbounds', label: '入站 (Inbounds)' },
  { id: 'access_points', label: '接入点' },
]
const activeTab = ref('inbounds')

const serverFilterOptions = computed(() => servers.value.map((s) => ({ label: s.name, value: s.id })))
const serverFormOptions = computed(() =>
  servers.value.map((s) => ({ label: `${s.name} (${s.host})`, value: s.id })),
)
const typeFilterOptions = [
  { label: '用户入站', value: 'user' },
  { label: '四层直通', value: 'tunnel' },
]
const inboundStatusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
]

const list = ref<InboundItem[]>([])
const loading = ref(false)
const inboundKeyword = ref('')
const typeFilter = ref('')
const inboundStatusFilter = ref('')
const inboundPage = ref(1)
const inboundSize = ref(10)

const hasInboundFilters = computed(() =>
  Boolean(serverFilter.value || typeFilter.value || inboundStatusFilter.value || inboundKeyword.value.trim()),
)

function resetInboundFilters() {
  serverFilter.value = undefined
  typeFilter.value = ''
  inboundStatusFilter.value = ''
  inboundKeyword.value = ''
}

// 本地过滤（列表数据已全量在本地；历史行 type 为空按用户入站口径处理）
const filteredInbounds = computed(() => {
  const kw = inboundKeyword.value.trim().toLowerCase()
  return list.value.filter((r) => {
    if (typeFilter.value && (r.type || 'user') !== typeFilter.value) return false
    if (inboundStatusFilter.value === 'enabled' && !r.enabled) return false
    if (inboundStatusFilter.value === 'disabled' && r.enabled) return false
    if (kw) {
      const hitTag = (r.tag || '').toLowerCase().includes(kw)
      const hitRemark = (r.remark || '').toLowerCase().includes(kw)
      const hitPort = String(r.port ?? '').includes(kw)
      if (!hitTag && !hitRemark && !hitPort) return false
    }
    return true
  })
})

watch([typeFilter, inboundStatusFilter, inboundKeyword, serverFilter], () => {
  inboundPage.value = 1
})

watch([() => filteredInbounds.value.length, inboundSize], ([len, size]) => {
  const maxPage = Math.max(1, Math.ceil(len / Math.max(1, size)))
  if (inboundPage.value > maxPage) inboundPage.value = maxPage
})

const pagedInbounds = computed(() => {
  const start = (inboundPage.value - 1) * inboundSize.value
  return filteredInbounds.value.slice(start, start + inboundSize.value)
})

const inboundColumns: TableColumn[] = [
  { id: 'id', header: '编号' },
  { id: 'inbound', header: '入站' },
  { id: 'server', header: '所属服务器' },
  { id: 'protocol', header: '协议 / 端口' },
  { id: 'transport', header: '传输 / TLS' },
  { id: 'traffic', header: '用量' },
  { id: 'status', header: '状态' },
  { id: 'actions', header: '操作', width: '1%' },
]

type BadgeColor =
  | 'grey'
  | 'green'
  | 'blue'
  | 'red'
  | 'severity-medium'
  | 'severity-neutral'

function inboundChipText(row: InboundItem): string {
  if (row.type === 'tunnel' || row.type === 'forward' || row.type === 'channel' || row.protocol === 'tunnel') return '四层直通'
  if (row.protocol === 'hysteria') return 'Hysteria 2'
  return 'VLESS'
}

function inboundChipColor(row: InboundItem): BadgeColor {
  if (row.type === 'tunnel' || row.type === 'forward' || row.type === 'channel' || row.protocol === 'tunnel') return 'grey'
  if (row.protocol === 'hysteria') return 'green'
  return 'blue'
}

function serverName(id: number) {
  return servers.value.find((s) => s.id === id)?.name ?? `#${id}`
}

async function load() {
  loading.value = true
  try {
    // CsSelect 清空时写回 ''，这里统一归一化为「全部服务器」
    const sid = typeof serverFilter.value === 'number' ? serverFilter.value : undefined
    const { data } = await getInbounds(sid)
    if (data.code === 0) list.value = data.data.items
    else toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '加载入站失败'))
  } finally {
    loading.value = false
  }
}

function syncTabQuery() {
  const query: LocationQueryRaw = { ...route.query }
  if (activeTab.value === 'access_points') query.tab = 'access_points'
  else delete query.tab
  router.replace({ query })
}

// 用量轮询（20s）：只就地回填 up/down/enabled，不整表替换——整行换新会让卡片重建、
// 开关与滚动位置闪动。页面不可见、有弹窗打开或正在保存/加载时跳过。
const USAGE_REFRESH_MS = 20000
let usageTimer: number | undefined
// 写操作在途（启停/删除/保存）：轮询让路，避免把开关状态刷回旧值
const mutating = ref(false)

async function refreshUsage() {
  if (document.visibilityState !== 'visible') return
  if (activeTab.value !== 'inbounds' || loading.value || saving.value || editorOpen.value || mutating.value) return
  const sid = typeof serverFilter.value === 'number' ? serverFilter.value : undefined
  try {
    const { data } = await getInbounds(sid)
    if (data.code !== 0) return
    const byId = new Map(list.value.map((r) => [r.id, r]))
    for (const fresh of data.data.items) {
      const row = byId.get(fresh.id)
      if (!row) continue
      row.up = fresh.up
      row.down = fresh.down
      // 配额/到期触发的自动停用发生在后端 cron，就地跟上开关状态
      row.enabled = fresh.enabled
    }
  } catch {
    /* 轮询失败静默：下次进入可见/下次 tick 再试，不弹 toast 打扰 */
  }
}

let initialized = false

onMounted(async () => {
  // 从服务器页跳转进入：?server_id=X 预选过滤
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  await loadServers()
  await Promise.all([load(), loadAccessPoints(), loadPermissionGroups()])
  if (route.query.tab === 'access_points') activeTab.value = 'access_points'
  initialized = true
  syncServerQuery()
  usageTimer = window.setInterval(refreshUsage, USAGE_REFRESH_MS)
})

onUnmounted(() => {
  if (usageTimer !== undefined) window.clearInterval(usageTimer)
  usageTimer = undefined
})

watch(serverFilter, () => {
  if (!initialized) return
  syncServerQuery()
  load()
})

watch(activeTab, () => {
  if (!initialized) return
  syncTabQuery()
})

// ---- 用户接入点（Access Points）统一管理 ----
const apList = ref<UserAccessPoint[]>([])
const apLoading = ref(false)
const apGroups = ref<PermissionGroup[]>([])
const allInbounds = ref<InboundItem[]>([])
const apKeyword = ref('')
const apGroupFilter = ref('')
const apStatusFilter = ref('')
const apPage = ref(1)
const apSize = ref(10)

const apGroupFilterOptions = computed(() => [
  { label: '无权限组', value: 'none' },
  ...apGroups.value.map((g) => ({ label: g.name, value: String(g.id) })),
])

const apStatusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
]

const hasApFilters = computed(() =>
  Boolean(apKeyword.value.trim() || apGroupFilter.value || apStatusFilter.value),
)

function resetApFilters() {
  apKeyword.value = ''
  apGroupFilter.value = ''
  apStatusFilter.value = ''
}

const filteredAccessPoints = computed(() => {
  const kw = apKeyword.value.trim().toLowerCase()
  return apList.value.filter((row) => {
    if (apStatusFilter.value === 'enabled' && !row.enabled) return false
    if (apStatusFilter.value === 'disabled' && row.enabled) return false
    if (apGroupFilter.value === 'none') {
      if (row.permission_group_ids && row.permission_group_ids.length > 0) return false
    } else if (apGroupFilter.value) {
      const gid = Number(apGroupFilter.value)
      if (!row.permission_group_ids?.includes(gid)) return false
    }
    if (kw) {
      const hitName = (row.name || '').toLowerCase().includes(kw)
      const hitRemark = (row.remark || '').toLowerCase().includes(kw)
      const hitTarget = apTargetDesc(row).toLowerCase().includes(kw)
      const hitHost = (row.custom_host || '').toLowerCase().includes(kw)
      if (!hitName && !hitRemark && !hitTarget && !hitHost) return false
    }
    return true
  })
})

watch([apKeyword, apGroupFilter, apStatusFilter], () => {
  apPage.value = 1
})

watch([() => filteredAccessPoints.value.length, apSize], ([len, size]) => {
  const maxPage = Math.max(1, Math.ceil(len / Math.max(1, size)))
  if (apPage.value > maxPage) apPage.value = maxPage
})

const pagedAccessPoints = computed(() => {
  const start = (apPage.value - 1) * apSize.value
  return filteredAccessPoints.value.slice(start, start + apSize.value)
})

const apColumns: TableColumn[] = [
  { id: 'id', header: '编号' },
  { id: 'name', header: '接入点' },
  { id: 'groups', header: '开放权限组' },
  { id: 'target', header: '目标绑定' },
  { id: 'endpoint', header: '连接地址' },
  { id: 'status', header: '状态' },
  { id: 'actions', header: '操作', width: '1%' },
]

async function loadPermissionGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) apGroups.value = data.data.items
  } catch {
    /* 忽略 */
  }
}

function groupName(id: number) {
  return apGroups.value.find((g) => g.id === id)?.name ?? `#${id}`
}

function apGroupTitle(ap: UserAccessPoint): string {
  if (!ap.permission_group_ids || ap.permission_group_ids.length === 0) return '未分配权限组（全员不可见）'
  return ap.permission_group_ids.map((gid) => groupName(gid)).join('、')
}

function apTargetDesc(ap: any): string {
  if (ap.target_type === 'inbound' && ap.target_inbound_tag) {
    if (ap.is_tunnel) {
      const landing = ap.tunnel_landing_desc ? ` → ${ap.tunnel_landing_desc}` : ''
      return `直通 → ${ap.target_inbound_tag}${landing}`
    }
    return `直连 → ${ap.target_inbound_tag}`
  }
  return '无'
}

function apTargetColor(ap: any): BadgeColor {
  if (apTargetDesc(ap) === '无') return 'severity-medium'
  if (ap.is_tunnel) return 'grey'
  return 'blue'
}

async function loadAccessPoints() {
  apLoading.value = true
  try {
    const [res, inbRes] = await Promise.all([getAccessPoints(), getInbounds(undefined)])
    if (res.data.code === 0) apList.value = res.data.data.items
    else toast.error(res.data.message)
    if (inbRes.data.code === 0) allInbounds.value = inbRes.data.data.items
  } catch (e) {
    toast.error(errMsg(e, '加载接入点失败'))
  } finally {
    apLoading.value = false
  }
}

const apDialogOpen = ref(false)
const apEditingItem = ref<UserAccessPoint | null>(null)

function openCreateAccessPoint() {
  apEditingItem.value = null
  apDialogOpen.value = true
}

function openEditAccessPoint(ap: UserAccessPoint) {
  apEditingItem.value = ap
  apDialogOpen.value = true
}

async function toggleAccessPoint(ap: any) {
  try {
    const { data } = await updateAccessPoint(ap.id, {
      name: ap.name,
      enabled: !ap.enabled,
      custom_host: ap.custom_host,
      custom_port: ap.custom_port || 0,
      remark: ap.remark || '',
      target_type: ap.target_type,
      target_inbound_id: ap.target_inbound_id,
    })
    if (data.code === 0) {
      toast.success(ap.enabled ? '接入点已停用' : '接入点已启用')
      loadAccessPoints()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '操作失败'))
  }
}

// ---- 新增 / 编辑（复用 InboundConfigEditor） ----
const editorOpen = ref(false)
const editing = ref<InboundItem | null>(null)
const inboundChange = ref<InboundEditorChangePayload | null>(null)
const formServerId = ref(0)
const formType = ref('user')
const formCertId = ref(0)
const saving = ref(false)

function editorModelValue(): string {
  if (!editing.value) return '{}'
  let settingsObj: Record<string, any> = {}
  let streamObj: Record<string, any> = {}
  let sniffObj: Record<string, any> | undefined
  try {
    settingsObj = JSON.parse(editing.value.settings_json || '{}')
    streamObj = JSON.parse(editing.value.stream_settings || '{}')
    sniffObj = editing.value.sniffing ? JSON.parse(editing.value.sniffing) : undefined
  } catch {
    /* 保持空对象，交给编辑器按默认值处理 */
  }
  return JSON.stringify({
    tag: editing.value.tag,
    port: editing.value.port,
    listen: editing.value.listen,
    protocol: editing.value.protocol,
    settings: settingsObj,
    streamSettings: streamObj,
    ...(sniffObj ? { sniffing: sniffObj } : {}),
    _panel: {
      remark: editing.value.remark || '',
      traffic_reset: editing.value.traffic_reset || 'never',
      flow: editing.value.flow || '',
      ratio: editing.value.ratio,
      total_gb: editing.value.total_gb ?? 0,
      up: editing.value.up ?? 0,
      down: editing.value.down ?? 0,
      expiry_time: editing.value.expiry_time ?? null,
      type: editing.value.type || 'user',
      target_inbound_id: editing.value.target_inbound_id,
      target_address: editing.value.target_address,
      target_port: editing.value.target_port,
      cert_id: editing.value.cert_id || 0,
      share_addr_strategy: editing.value.share_addr_strategy,
      share_addr: editing.value.share_addr,
      share_port: editing.value.share_port,
      share_sni: editing.value.share_sni,
      share_allow_insecure: editing.value.share_allow_insecure,
      layer_id: editing.value.layer_id || 0,
    },
  })
}

function closeEditor() {
  editorOpen.value = false
  editing.value = null
  inboundChange.value = null
}

function openCreate() {
  editing.value = null
  inboundChange.value = null
  formServerId.value = serverFilter.value || servers.value[0]?.id || 0
  formType.value = 'user'
  formCertId.value = 0
  editorOpen.value = true
}

function openEdit(row: any) {
  editing.value = row as InboundItem
  inboundChange.value = null
  formServerId.value = row.server_id
  formType.value = row.type || 'user'
  formCertId.value = row.cert_id || 0
  editorOpen.value = true
}

function onInboundEditorChange(payload: InboundEditorChangePayload) {
  inboundChange.value = payload
}

async function save() {
  const c = inboundChange.value
  if (!c) {
    toast.warning('请先在表单中编辑入站配置')
    return
  }
  if (!formServerId.value) {
    toast.warning('请选择所属服务器')
    return
  }
  if (!c.tag.trim() || !c.port) {
    toast.warning('请填写标签与端口')
    return
  }
  if (c.validationError) {
    toast.warning(`请先修正配置：${c.validationError}`)
    return
  }

  saving.value = true
  try {
    const payload: InboundPayload = {
      server_id: formServerId.value,
      tag: c.tag,
      // 0 / 空串都是有效终态（解绑、关闭、清空），不能再用 `|| undefined` 吞掉：
      // 后端收到 nil 会保留旧值，界面显示保存成功但设置没变。
      remark: c.remark ?? '',
      traffic_reset: c.traffic_reset || 'never',
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing,
      ratio: c.ratio,
      total_gb: c.total_gb,
      expiry_time: (c.expiry_time && c.expiry_time.trim()) ? c.expiry_time : null,
      type: c.type || formType.value,
      target_inbound_id: c.targetInboundId ?? 0,
      target_address: c.targetAddress ?? '',
      target_port: c.targetPort ?? 0,
      cert_id: formCertId.value,
      flow: c.flow,
      share_addr_strategy: c.shareAddrStrategy || 'node',
      share_addr: c.shareAddr ?? '',
      share_port: c.sharePort ?? 0,
      share_sni: c.shareSni ?? '',
      share_allow_insecure: c.shareAllowInsecure,
      layer_id: c.layerId ?? 0,
    }

    if (editing.value) {
      const { data } = await updateInbound(editing.value.id, payload)
      if (data.code === 0) {
        toast.success('入站已保存')
        closeEditor()
        load()
      } else {
        toast.error(data.message)
      }
    } else {
      const { data } = await createInbound(payload)
      if (data.code === 0) {
        toast.success('入站已创建')
        closeEditor()
        load()
      } else {
        toast.error(data.message)
      }
    }
  } catch (e) {
    toast.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- 启用 / 删除 ----
async function toggle(row: any) {
  // 与用量轮询互斥：否则轮询可能在 toggle 请求在途时把开关刷回旧值
  mutating.value = true
  try {
    const { data } = await toggleInbound(row.id)
    if (data.code === 0) {
      toast.success(row.enabled ? '已停用' : '已启用')
      load()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '操作失败'))
  } finally {
    mutating.value = false
  }
}

async function remove(row: any) {
  try {
    await messageBox.confirm(`确认删除入站「${row.tag}」？`, '删除入站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteInbound(row.id)
    if (data.code === 0) {
      toast.success('已删除')
      load()
    } else {
      toast.error(errMsg(data.message, '删除失败'))
    }
  } catch (e) {
    toast.error(errMsg(e, '删除失败'))
  }
}

function removeFromEditor() {
  const target = editing.value
  if (!target) return
  closeEditor()
  remove(target)
}

function transportOf(row: any): string {
  try {
    const ss = JSON.parse(row.stream_settings || '{}')
    return `${ss.network || '—'}/${ss.security || '—'}`
  } catch {
    return '—'
  }
}

const BYTES_PER_GB = 1024 * 1024 * 1024
const RING_RADIUS = 7.5
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS
const RESET_TEXT: Record<string, string> = { daily: '每天重置', weekly: '每周重置', monthly: '每月重置' }

// 本周期已用字节：inbounds.up/down 由 agent 的 inbound>>> 计数器累计，
// 口径与限额判定完全一致（都按字节比配额），tunnel 入站同样计数。
function usedBytes(row: InboundItem): number {
  return (row.up || 0) + (row.down || 0)
}

function quotaBytes(row: InboundItem): number {
  return (row.total_gb || 0) * BYTES_PER_GB
}

function usagePercent(row: InboundItem): number {
  const quota = quotaBytes(row)
  if (quota <= 0) return 0
  return Math.min(100, Math.round((usedBytes(row) / quota) * 100))
}

function ringDashOffset(row: InboundItem): number {
  const quota = quotaBytes(row)
  const used = usedBytes(row)
  if (quota <= 0 || used <= 0) return RING_CIRCUMFERENCE
  const ratio = Math.min(1, used / quota)
  return RING_CIRCUMFERENCE * (1 - ratio)
}

// 停用原因：cron 自动停用不写标记，只能按同一口径反推（与后端 QuotaExhausted 对齐）。
// 没有这层提示时，超额/到期入站在列表上与「管理员手动停用」长得一模一样。
function stopReason(row: InboundItem): string {
  if (row.enabled) return ''
  if (row.total_gb && usedBytes(row) >= quotaBytes(row)) return '超额停用'
  if (row.expiry_time && new Date(row.expiry_time).getTime() <= Date.now()) return '到期停用'
  return ''
}

function resetTextOf(row: InboundItem): string {
  return (row.traffic_reset && RESET_TEXT[row.traffic_reset]) || '不自动重置'
}

function inboundTrafficTitle(row: InboundItem): string {
  const parts = [row.total_gb ? `已用 ${usagePercent(row)}%` : '不限流量', resetTextOf(row)]
  if (row.expiry_time) {
    parts.push(`到期：${formatDateOnly(row.expiry_time)}`)
  }
  return parts.join('｜')
}
</script>

<template>
  <CsPage>
    <CsTabs v-model="activeTab" :tabs="tabs" aria-label="入站与接入点">
      <!-- ==================== Tab 1: 入站 (Inbounds) ==================== -->
      <div v-if="activeTab === 'inbounds'" class="nodes-tab-body">
        <CsToolbar>
          <CsInput
            v-model="inboundKeyword"
            placeholder="搜索标签 / 备注 / 端口"
            clearable
            prefix-icon="search"
            class="filter-search"
          />
          <CsSelect
            v-model="serverFilter"
            :options="serverFilterOptions"
            :loading="serverLoading"
            clearable
            filterable
            placeholder="全部服务器"
            aria-label="按服务器筛选入站"
            class="filter-select filter-select--lg"
          />
          <CsSelect
            v-model="typeFilter"
            :options="typeFilterOptions"
            clearable
            placeholder="全部类型"
            aria-label="按类型筛选入站"
            class="filter-select filter-select--md"
          />
          <CsSelect
            v-model="inboundStatusFilter"
            :options="inboundStatusOptions"
            clearable
            placeholder="全部状态"
            aria-label="按状态筛选入站"
            class="filter-select filter-select--sm"
          />
          <CsButton v-if="hasInboundFilters" @click="resetInboundFilters">重置</CsButton>
          <CsButton icon-name="refresh" @click="load">刷新</CsButton>
          <TipIcon
            content="在此为 Xray 服务器配置物理入站：VLESS / Hysteria 2 代理入站支持多源复用（同时承载用户与链式中继），四层直通管道实现多级穿透。新增/编辑/停用后自动生成配置推送到服务器（离线保存，上线补推）。"
          />
          <template #actions>
            <CsButton variant="primary" icon-name="add-plus" @click="openCreate">新增入站</CsButton>
          </template>
        </CsToolbar>

        <CsContainer header="入站列表">
          <CsTable
            compact
            :wrap-lines="false"
            track-by="id"
            :items="pagedInbounds"
            :columns="inboundColumns"
            :loading="loading"
            loading-text="加载中…"
            :empty-text="
              hasInboundFilters
                ? '未找到匹配当前筛选的入站'
                : '尚未为任何服务器添加入站，点击右上角「新增入站」开始配置。'
            "
          >
            <template #cell-id="{ row }">
              <span class="row-id">#{{ row.id }}</span>
            </template>

            <template #cell-inbound="{ row }">
              <div class="name-cell">
                <div class="name-cell-main">
                  <button
                    type="button"
                    class="name-link-btn"
                    :class="{ 'is-muted': !row.enabled }"
                    title="点击编辑入站"
                    @click="openEdit(row)"
                  >
                    {{ row.tag }}
                  </button>
                </div>
                <div v-if="row.remark" class="row-remark" :title="row.remark">备注：{{ row.remark }}</div>
              </div>
            </template>

            <template #cell-server="{ row }">
              <span :class="{ 'cell-muted': !row.enabled }">{{ serverName(row.server_id) }}</span>
            </template>

            <template #cell-protocol="{ row }">
              <div class="proto-cell">
                <CsBadge class="nowrap-badge" :color="inboundChipColor(row)">{{ inboundChipText(row) }}</CsBadge>
                <span class="cell-mono" :class="{ 'cell-muted': !row.enabled }">:{{ row.port }}</span>
              </div>
            </template>

            <template #cell-transport="{ row }">
              <span class="cell-mono" :class="{ 'cell-muted': !row.enabled }">{{ transportOf(row) }}</span>
            </template>

            <template #cell-traffic="{ row }">
              <div class="traffic-cell" :title="inboundTrafficTitle(row)">
                <svg
                  v-if="row.total_gb"
                  class="traffic-ring"
                  :class="{ 'is-danger': usagePercent(row) > 90 }"
                  viewBox="0 0 20 20"
                  aria-hidden="true"
                >
                  <circle class="traffic-ring-track" cx="10" cy="10" r="7.5" />
                  <circle
                    v-if="usedBytes(row) > 0"
                    class="traffic-ring-fill"
                    cx="10"
                    cy="10"
                    r="7.5"
                    transform="rotate(-90 10 10)"
                    :stroke-dasharray="RING_CIRCUMFERENCE"
                    :stroke-dashoffset="ringDashOffset(row)"
                  />
                </svg>
                <span class="traffic-text cell-mono" :class="{ 'cell-muted': !row.enabled }">
                  <strong>{{ formatBytes(usedBytes(row)) }}</strong> /
                  {{ row.total_gb ? formatBytes(quotaBytes(row)) : '不限' }}
                </span>
              </div>
            </template>

            <template #cell-status="{ row }">
              <div class="status-cell">
                <CsTooltip :content="row.enabled ? '已启用，点击禁用' : '已禁用，点击启用'">
                  <CsToggle
                    :model-value="row.enabled"
                    :aria-label="`启用入站 ${row.tag}`"
                    @change="toggle(row)"
                  />
                </CsTooltip>
                <CsBadge v-if="stopReason(row)" class="nowrap-badge" color="red">{{ stopReason(row) }}</CsBadge>
              </div>
            </template>

            <template #cell-actions="{ row }">
              <CsButton :wrap-text="false" @click="openEdit(row)">编辑</CsButton>
            </template>
          </CsTable>

          <template #footer>
            <CsPaginationBar v-if="filteredInbounds.length > 0">
              <CsPagination
                v-model:current-page="inboundPage"
                v-model:page-size="inboundSize"
                :total="filteredInbounds.length"
                :page-sizes="[10, 20, 50, 100]"
                layout="total, sizes, prev, pager, next"
                @size-change="inboundPage = 1"
              />
            </CsPaginationBar>
          </template>
        </CsContainer>
      </div>

      <!-- ==================== Tab 2: 用户接入点 (Access Points) ==================== -->
      <div v-else class="nodes-tab-body">
        <CsToolbar>
          <CsInput
            v-model="apKeyword"
            placeholder="搜索名称 / 备注 / 目标"
            clearable
            prefix-icon="search"
            class="filter-search"
          />
          <CsSelect
            v-model="apGroupFilter"
            :options="apGroupFilterOptions"
            clearable
            placeholder="全部权限组"
            aria-label="按权限组筛选接入点"
            class="filter-select filter-select--md"
          />
          <CsSelect
            v-model="apStatusFilter"
            :options="apStatusOptions"
            clearable
            placeholder="全部状态"
            aria-label="按状态筛选接入点"
            class="filter-select filter-select--sm"
          />
          <CsButton v-if="hasApFilters" @click="resetApFilters">重置</CsButton>
          <CsButton icon-name="refresh" @click="loadAccessPoints">刷新</CsButton>
          <TipIcon
            content="接入点是用户订阅中实际看到的「节点」，通过白名单权限组控制哪些权限组可见。接入点可直接绑定入站，亦可在拓扑中参与链式转发中转。"
          />
          <template #actions>
            <CsButton variant="primary" icon-name="add-plus" @click="openCreateAccessPoint">新建接入点</CsButton>
          </template>
        </CsToolbar>

        <CsContainer header="接入点分发列表">
          <CsTable
            compact
            :wrap-lines="false"
            track-by="id"
            :items="pagedAccessPoints"
            :columns="apColumns"
            :loading="apLoading"
            loading-text="加载中…"
            :empty-text="
              hasApFilters
                ? '未找到匹配当前筛选的接入点'
                : '尚未创建任何接入点。新建后即可作为用户订阅的入口（订阅仅从接入点生成）。'
            "
          >
            <template #cell-id="{ row }">
              <span class="row-id">#{{ row.id }}</span>
            </template>

            <template #cell-name="{ row }">
              <div class="name-cell">
                <div class="name-cell-main">
                  <button
                    type="button"
                    class="name-link-btn"
                    :class="{ 'is-muted': !row.enabled }"
                    title="点击编辑接入点"
                    @click="openEditAccessPoint(row)"
                  >
                    {{ row.name }}
                  </button>
                </div>
                <div v-if="row.remark" class="row-remark" :title="row.remark">备注：{{ row.remark }}</div>
              </div>
            </template>

            <template #cell-target="{ row }">
              <CsBadge
                class="nowrap-badge"
                :color="apTargetColor(row)"
                :title="apTargetDesc(row) === '无' ? '未绑定入站（可在编辑或拓扑中连线）' : undefined"
              >
                {{ apTargetDesc(row) }}
              </CsBadge>
            </template>

            <template #cell-endpoint="{ row }">
              <span v-if="row.custom_host" class="cell-mono" :class="{ 'cell-muted': !row.enabled }">
                {{ row.custom_host }}:{{ row.custom_port || '自动' }}
              </span>
              <span v-else class="cell-muted">自动继承</span>
            </template>

            <template #cell-groups="{ row }">
              <div class="ap-group-badges" :title="apGroupTitle(row)">
                <template v-if="row.permission_group_ids && row.permission_group_ids.length > 0">
                  <CsBadge
                    v-for="gid in row.permission_group_ids.slice(0, 3)"
                    :key="gid"
                    class="nowrap-badge"
                    color="blue"
                  >
                    {{ groupName(gid) }}
                  </CsBadge>
                  <CsBadge v-if="row.permission_group_ids.length > 3" class="nowrap-badge" color="grey">
                    +{{ row.permission_group_ids.length - 3 }}
                  </CsBadge>
                </template>
                <CsBadge v-else class="nowrap-badge" color="severity-medium">无</CsBadge>
              </div>
            </template>

            <template #cell-status="{ row }">
              <div class="status-cell">
                <CsTooltip :content="row.enabled ? '已启用，点击禁用' : '已禁用，点击启用'">
                  <CsToggle
                    :model-value="row.enabled"
                    :aria-label="`启用接入点 ${row.name}`"
                    @change="toggleAccessPoint(row)"
                  />
                </CsTooltip>
              </div>
            </template>

            <template #cell-actions="{ row }">
              <CsButton :wrap-text="false" @click="openEditAccessPoint(row)">编辑</CsButton>
            </template>
          </CsTable>

          <template #footer>
            <CsPaginationBar v-if="filteredAccessPoints.length > 0">
              <CsPagination
                v-model:current-page="apPage"
                v-model:page-size="apSize"
                :total="filteredAccessPoints.length"
                :page-sizes="[10, 20, 50, 100]"
                layout="total, sizes, prev, pager, next"
                @size-change="apPage = 1"
              />
            </CsPaginationBar>
          </template>
        </CsContainer>

        <!-- 新建/编辑用户接入点（弹窗本体仍在后续批次迁移） -->
        <AccessPointDialog
          v-model="apDialogOpen"
          :access-point="apEditingItem"
          :servers="servers"
          :inbounds="allInbounds"
          :permission-groups="apGroups"
          @saved="loadAccessPoints"
          @deleted="loadAccessPoints"
        />
      </div>
    </CsTabs>

    <!-- 新增/编辑入站（编辑器本体仍在后续批次迁移） -->
    <CsModal
      v-model:visible="editorOpen"
      :header="editing ? `编辑入站：${editing.tag}` : '新建入站'"
      size="large"
      scrollable
      :close-on-click-overlay="false"
      @close="closeEditor"
    >
      <CsForm v-if="!editing" label-position="top">
        <CsFormField label="所属目标服务器" required>
          <CsSelect
            v-model="formServerId"
            :options="serverFormOptions"
            placeholder="请选择要绑定入站的服务器"
          />
        </CsFormField>
      </CsForm>

      <InboundConfigEditor
        :key="editing ? `edit-${editing.id}` : 'create'"
        :model-value="editorModelValue()"
        :protocol="editing?.protocol || 'vless'"
        :inbound-type="formType"
        :listen="editing?.listen || '0.0.0.0'"
        :inbound-id="editing?.id || 0"
        :cert-id="formCertId"
        :server-id="formServerId"
        :layer-id="editing?.layer_id || 0"
        :target-inbound-id="editing?.target_inbound_id"
        :target-address="editing?.target_address || ''"
        :target-port="editing?.target_port || 0"
        @change="onInboundEditorChange"
        @update:inbound-type="(v: string) => (formType = v)"
        @update:cert-id="(v: number) => (formCertId = v || 0)"
      />

      <template #footer>
        <div class="modal-footer-split">
          <div class="modal-footer-left">
            <CsButton
              v-if="editing"
              variant="danger-outline"
              :disabled="saving"
              @click="removeFromEditor"
            >
              删除入站
            </CsButton>
          </div>
          <div class="modal-footer-right">
            <CsButton @click="closeEditor">取消</CsButton>
            <CsButton variant="primary" :loading="saving" @click="save">保存入站</CsButton>
          </div>
        </div>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
/* CsTabs 内容区默认只有上 16px 内边距（左右为 0），这里补工具栏与列表之间的 20px */
.nodes-tab-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-inline-size: 0;
}

.filter-search {
  width: 220px;
}

.filter-select--lg {
  width: 190px;
}

.filter-select--md {
  width: 145px;
}

.filter-select--sm {
  width: 115px;
}

.row-id {
  flex: none;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.name-cell-main {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.name-link-btn {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font-family: inherit;
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-body-default-b2us04, #0f141a);
  text-align: start;
  cursor: pointer;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
    text-decoration: underline;
  }

  &.is-muted {
    color: var(--color-text-status-inactive-fqjc97, #656871);
  }
}

.row-remark {
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.proto-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  vertical-align: middle;
}

.nowrap-badge {
  white-space: nowrap !important;
  flex-shrink: 0;
}

.traffic-cell {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  vertical-align: middle;
}

.traffic-ring {
  width: 18px;
  height: 18px;
  flex: none;
}

.traffic-ring-track {
  fill: none;
  stroke: var(--color-border-divider-default-diwd4w, #e9ebed);
  stroke-width: 2.5;
}

.traffic-ring-fill {
  fill: none;
  stroke: var(--color-text-link-default-qabnuk, #006ce0);
  stroke-width: 2.5;
  stroke-linecap: round;
  transition: stroke-dashoffset 0.2s ease;
}

.traffic-ring.is-danger .traffic-ring-fill {
  stroke: var(--color-text-status-error-ri38gt, #d91515);
}

.traffic-text {
  font-variant-numeric: tabular-nums;
}

.status-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  vertical-align: middle;
}

.ap-group-badges {
  display: inline-flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 4px;
}

.modal-footer-split {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  width: 100%;
}

.modal-footer-left,
.modal-footer-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: $bp-tablet-max) {
  .cs-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-xs-m9b7jf, 8px);
  }

  :deep(.cs-toolbar-main) {
    inline-size: 100%;

    .filter-search {
      inline-size: 100% !important;
    }

    .filter-select {
      flex: 1 1 calc(50% - 4px);
      min-inline-size: 120px;
      inline-size: auto;
    }
  }

  :deep(.cs-toolbar-actions) {
    display: flex;
    flex-wrap: wrap;
    inline-size: 100%;
    gap: 6px;
  }
}
</style>
