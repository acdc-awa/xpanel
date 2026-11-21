<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { MQ, useMediaQuery } from '@/utils/media'
import {
  toast,
  CsAlert,
  CsBadge,
  CsButton,
  CsContainer,
  CsEmpty,
  CsHeader,
  CsIcon,
  CsInput,
  CsLink,
  CsPage,
  CsSegmentedControl,
  CsSelect,
  CsSpaceBetween,
  CsTable,
  CsTabs,
  CsToggle,
  CsToolbar,
  CsTooltip,
  type BadgeColor,
  type SegmentedOption,
  type TabItem,
  type TableColumn,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import RoutingRuleEditor from './servers/RoutingRuleEditor.vue'
import ServerSettingsModal from './servers/ServerSettingsModal.vue'
import TopologyCanvas from '@/components/topology/TopologyCanvas.vue'
import {
  getServerOutbounds,
  getServerRoutingRules,
  getTopology,
  updateServer,
  updateServerRoutingRule,
  type ServerOutbound,
  type ServerRoutingRule,
  type ServerItem,
  type TopologyData,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { useServerInbounds, useServerScope } from '@/composables/useServerScope'

const route = useRoute()
const router = useRouter()

const isMobile = useMediaQuery(MQ.tablet)

const { servers, serverFilter, currentServer, serverLoading, loadServers, syncServerQuery } =
  useServerScope()
const { inboundTags, ownerTagOf, loadServerInbounds } = useServerInbounds(serverFilter)

const serverOptions = computed(() =>
  servers.value.map((s) => ({ label: `${s.name} (${s.host})`, value: s.id }))
)

// CsSelect / CsSegmentedControl 的 v-model 类型是 string | number，这里显式收窄。
function onServerFilterChange(value: string | number | (string | number)[] | '') {
  const raw = Array.isArray(value) ? value[0] : value
  const id = Number(raw)
  serverFilter.value = Number.isFinite(id) && id > 0 ? id : undefined
}

// ---- 默认出口 & 路由匹配策略（随服务器切换） ----
// 出站域名解析策略（freedom settings.domainStrategy）不在此处：它是出站级属性，
// 唯一入口是出站管理页的编辑面板。
const defaultOutboundTag = ref('direct')
const routingDomainStrategy = ref('AsIs')
const defaultSaving = ref(false)
const activeTab = ref<'rules' | 'settings'>('rules')

const tabItems: TabItem[] = [
  { id: 'rules', label: '路由规则' },
  { id: 'settings', label: '全局策略' },
]

const viewOptions: SegmentedOption[] = [
  { id: 'canvas', text: '拓扑图', iconName: 'share' },
  { id: 'table', text: '表格', iconName: 'grid-view' },
]

const domainStrategyOptions = [
  { label: 'AsIs（保持原样，仅按客户端原始请求匹配）', value: 'AsIs' },
  { label: 'IPIfNonMatch（先域名后 IP，域名未命中则解析 IP 再次匹配）', value: 'IPIfNonMatch' },
  { label: 'IPOnDemand（按需解析 IP，遇到 IP 规则时立即解析）', value: 'IPOnDemand' },
]

function onViewChange(detail: { selectedId: string | number }) {
  viewMode.value = detail.selectedId === 'canvas' ? 'canvas' : 'table'
}

function onTabChange(detail: { activeTabId: string }) {
  activeTab.value = detail.activeTabId === 'settings' ? 'settings' : 'rules'
}

function onDefaultOutboundChange(value: string | number | (string | number)[] | '') {
  defaultOutboundTag.value = Array.isArray(value) ? String(value[0] ?? '') : String(value)
}

function onDomainStrategyChange(value: string | number | (string | number)[] | '') {
  routingDomainStrategy.value = Array.isArray(value) ? String(value[0] ?? '') : String(value)
}

watch(currentServer, (s) => {
  if (s) {
    defaultOutboundTag.value = s.default_outbound_tag || 'direct'
    routingDomainStrategy.value = s.routing_domain_strategy || 'AsIs'
  }
})

interface RuleChip {
  label: string
  value: string
  full: string
}

const VISIBLE_RULE_CHIP_LIMIT = 4

type EffectiveRoutingRule = ServerRoutingRule & {
  /** 派生自四层直通：可编辑（改指向/加条件/调优先级），不可删除 */
  ownerInboundTag?: string
  inboundTagList: string[]
  visibleChips: RuleChip[]
  hiddenCount: number
  hiddenTooltip: string
}

// 出站列表只作为路由规则与默认出口的选项来源；出站对象的增删改在「出站管理」页。
const outbounds = ref<ServerOutbound[]>([])
const outboundTags = computed(() => outbounds.value.map((o) => o.tag))
const outboundOptions = computed(() => outboundTags.value.map((t) => ({ label: t, value: t })))

async function loadOutboundOptions() {
  if (!serverFilter.value) {
    outbounds.value = []
    return
  }
  try {
    const { data } = await getServerOutbounds(serverFilter.value)
    if (data.code === 0) outbounds.value = data.data.items
    else toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '加载出站选项失败'))
  }
}

async function saveDefaultOutbound() {
  if (!currentServer.value) return
  defaultSaving.value = true
  try {
    const { data } = await updateServer(currentServer.value.id, {
      default_outbound_tag: defaultOutboundTag.value,
      routing_domain_strategy: routingDomainStrategy.value,
    })
    if (data.code === 0) {
      toast.success('默认出口与路由匹配策略已更新，下次配置推送生效')
      const s = currentServer.value
      if (s) {
        s.default_outbound_tag = defaultOutboundTag.value
        s.routing_domain_strategy = routingDomainStrategy.value
      }
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '保存默认出口失败'))
  } finally {
    defaultSaving.value = false
  }
}

// 辅助：将多行或逗号分隔的文本拆解为数组
function parseTagList(val?: string): string[] {
  if (!val) return []
  return val.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean)
}

function buildRuleChips(r: ServerRoutingRule): RuleChip[] {
  const chips: RuleChip[] = []
  if (r.rule_json?.trim()) {
    chips.push({ label: 'JSON', value: '自定义规则', full: '自定义 JSON 规则' })
  }
  for (const item of parseTagList(r.domain)) {
    chips.push({ label: '域名', value: item, full: `域名: ${item}` })
  }
  for (const item of parseTagList(r.ip)) {
    chips.push({ label: 'IP', value: item, full: `IP: ${item}` })
  }
  if (r.port?.trim()) {
    const port = r.port.trim()
    chips.push({ label: '端口', value: port, full: `端口: ${port}` })
  }
  if (r.network?.trim()) {
    const net = r.network.trim().toUpperCase()
    chips.push({ label: '网络', value: net, full: `网络: ${net}` })
  }
  for (const p of parseTagList(r.protocol)) {
    chips.push({ label: '协议', value: p, full: `协议: ${p}` })
  }
  return chips
}

function outboundBadgeColor(tag: string): BadgeColor {
  if (tag === 'direct') return 'green'
  if (tag === 'blocked' || tag === 'blackhole') return 'red'
  return 'blue'
}

// ---- 路由规则 ----
const routingRules = ref<ServerRoutingRule[]>([])
const routingLoading = ref(false)
const ruleEditorOpen = ref(false)
const ruleEditing = ref<ServerRoutingRule | null>(null)
const ruleKeyword = ref('')
const ruleOutboundFilter = ref('')

const ruleOutboundOptions = computed(() => [
  { label: '全部出站', value: '' },
  ...outboundTags.value.map((t) => ({ label: t, value: t })),
])

const hasRuleFilters = computed(() =>
  Boolean(ruleKeyword.value.trim() || ruleOutboundFilter.value)
)

function onRuleOutboundFilterChange(value: string | number | (string | number)[] | '') {
  ruleOutboundFilter.value = Array.isArray(value) ? String(value[0] ?? '') : String(value)
}

function resetRuleFilters() {
  ruleKeyword.value = ''
  ruleOutboundFilter.value = ''
}

const routingColumns: TableColumn[] = [
  { id: 'priority', header: '优先级' },
  { id: 'inbound', header: '来源入站' },
  { id: 'rules', header: '匹配规则' },
  { id: 'outbound', header: '目标出站' },
  { id: 'remark', header: '备注' },
  { id: 'enabled', header: '启用' },
  { id: 'actions', header: '操作', width: '1%' },
]

const displayRoutingRules = computed<EffectiveRoutingRule[]>(() =>
  routingRules.value.map((r) => {
    const chips = buildRuleChips(r)
    const hidden = chips.slice(VISIBLE_RULE_CHIP_LIMIT)
    return {
      ...r,
      ownerInboundTag: r.owner_inbound_id ? ownerTagOf.value[r.owner_inbound_id] || '' : '',
      inboundTagList: parseTagList(r.inbound_tag),
      visibleChips: chips.slice(0, VISIBLE_RULE_CHIP_LIMIT),
      hiddenCount: hidden.length,
      hiddenTooltip: hidden.map((c) => c.full).join('、'),
    }
  })
)

const filteredRoutingRules = computed<EffectiveRoutingRule[]>(() => {
  const kw = ruleKeyword.value.trim().toLowerCase()
  return displayRoutingRules.value.filter((row) => {
    if (ruleOutboundFilter.value && row.outbound_tag !== ruleOutboundFilter.value) {
      return false
    }
    if (kw) {
      const haystack = [
        row.inbound_tag,
        row.outbound_tag,
        row.domain,
        row.ip,
        row.port,
        row.protocol,
        row.network,
        row.remark,
        row.rule_json,
        row.ownerInboundTag,
      ]
        .filter(Boolean)
        .join('\n')
        .toLowerCase()
      if (!haystack.includes(kw)) return false
    }
    return true
  })
})

const ruleEmptyText = computed(() => {
  if (!serverFilter.value) return '请在顶部工具栏选择目标服务器'
  if (hasRuleFilters.value) return '没有匹配的路由规则'
  return '该服务器暂无路由规则，点击右上角「新增规则」'
})

async function loadRouting() {
  if (!serverFilter.value) {
    routingRules.value = []
    return
  }
  routingLoading.value = true
  try {
    const { data } = await getServerRoutingRules(serverFilter.value)
    if (data.code === 0) routingRules.value = data.data.items
    else toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '加载路由规则失败'))
  } finally {
    routingLoading.value = false
  }
}

function openRuleCreate() {
  ruleEditing.value = null
  ruleEditorOpen.value = true
}

function openRuleEdit(row: EffectiveRoutingRule) {
  ruleEditing.value = row
  ruleEditorOpen.value = true
}

async function toggleRule(row: EffectiveRoutingRule) {
  if (!serverFilter.value) return
  try {
    const { data } = await updateServerRoutingRule(serverFilter.value, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      toast.success(data.data.rule.enabled ? '已启用' : '已停用')
      loadRouting()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '操作失败'))
  }
}

function goOutbounds() {
  router.push({
    path: '/admin/outbounds',
    query: serverFilter.value ? { server_id: String(serverFilter.value) } : {},
  })
}

// ---- 服务器切换：重载表格数据（保留 view/tab 等其他 query） ----
let initialized = false
watch(serverFilter, (v) => {
  if (!initialized) return
  resetRuleFilters()
  syncServerQuery()
  if (!v) {
    outbounds.value = []
    routingRules.value = []
  }
  loadOutboundOptions()
  loadRouting()
  loadServerInbounds()
})

async function reloadTable() {
  await loadServers()
  await Promise.all([loadOutboundOptions(), loadRouting(), loadServerInbounds()])
}

// ---- T8：画布视图 ----
const viewMode = ref<'table' | 'canvas'>('canvas')
const topology = ref<TopologyData | null>(null)

async function loadTopology() {
  try {
    const { data } = await getTopology()
    if (data.code === 0) topology.value = data.data
    else toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '加载拓扑失败'))
  }
}

watch(viewMode, (m) => {
  if (m === 'canvas') loadTopology()
})

// 双击画布盒子 → 打开服务器详情弹窗
const drawerOpen = ref(false)
const drawerServer = ref<ServerItem | null>(null)

function openServer(serverId: number) {
  drawerServer.value = servers.value.find((s) => s.id === serverId) ?? null
  drawerOpen.value = true
}

async function onSettingsChanged() {
  await Promise.all([loadTopology(), loadServers()])
  if (drawerServer.value) {
    drawerServer.value = servers.value.find((s) => s.id === drawerServer.value!.id) ?? drawerServer.value
  }
}

async function onSettingsRemoved() {
  drawerOpen.value = false
  drawerServer.value = null
  await Promise.all([loadTopology(), loadServers()])
}

function openInboundCreateForServer(serverId: number) {
  openServer(serverId)
}

// 画布里的「新建出站」已归属独立页面：带上服务器上下文跳转，由出站页自动打开新建弹窗。
function openOutboundCreateForServer(serverId: number) {
  router.push({ path: '/admin/outbounds', query: { server_id: String(serverId), create: '1' } })
}

onMounted(async () => {
  // 旧深链兼容：出站 Tab 已拆成独立页面。
  if (route.query.tab === 'outbounds') {
    const query: LocationQueryRaw = { ...route.query }
    delete query.tab
    router.replace({ path: '/admin/outbounds', query })
    return
  }

  const q = Number(route.query.server_id)
  if (q > 0) {
    serverFilter.value = q
    viewMode.value = 'table'
  }
  if (route.query.view === 'table') {
    viewMode.value = 'table'
  } else if (route.query.view === 'canvas') {
    viewMode.value = 'canvas'
  } else if (isMobile.value) {
    viewMode.value = 'table'
  }
  if (route.query.tab === 'settings') activeTab.value = 'settings'

  await loadServers()
  await Promise.all([loadOutboundOptions(), loadRouting(), loadServerInbounds()])
  initialized = true
  syncServerQuery()
  if (viewMode.value === 'canvas') {
    await loadTopology()
  }
})
</script>

<template>
  <CsPage>
    <!-- 顶部工具栏 -->
    <CsToolbar>
      <template v-if="viewMode === 'table'">
        <CsSelect
          :model-value="serverFilter ?? null"
          :options="serverOptions"
          :loading="serverLoading"
          placeholder="选择目标服务器"
          aria-label="选择目标服务器"
          class="server-filter"
          @change="onServerFilterChange"
        />
        <CsButton icon-name="refresh" @click="reloadTable">刷新</CsButton>
      </template>
      <template v-else>
        <div class="topo-heading">
          <span class="topo-title">拓扑图</span>
          <span class="topo-count">{{ topology?.servers?.length || 0 }} 台服务器</span>
        </div>
      </template>

      <template #actions>
        <CsButton v-if="viewMode === 'canvas'" icon-name="refresh" @click="loadTopology">刷新</CsButton>
        <CsSegmentedControl :model-value="viewMode" :options="viewOptions" label="视图" @change="onViewChange" />
      </template>
    </CsToolbar>

    <!-- 表格视图 -->
    <template v-if="viewMode === 'table'">
      <CsTabs :model-value="activeTab" :tabs="tabItems" @change="onTabChange">
        <!-- Tab 1: 路由规则管理 -->
        <template v-if="activeTab === 'rules'">
          <CsSpaceBetween size="m">
            <CsToolbar class="rule-toolbar">
              <CsInput
                v-model="ruleKeyword"
                placeholder="搜索入站 / 出站 / 域名 / IP / 备注"
                clearable
                prefix-icon="search"
                class="rule-filter-search"
              />
              <CsSelect
                :model-value="ruleOutboundFilter"
                :options="ruleOutboundOptions"
                clearable
                placeholder="全部出站"
                aria-label="按目标出站筛选"
                class="rule-filter-select"
                @change="onRuleOutboundFilterChange"
              />
              <CsButton v-if="hasRuleFilters" @click="resetRuleFilters">重置</CsButton>
              <template #actions>
                <CsButton
                  variant="primary"
                  :disabled="!serverFilter"
                  icon-name="add-plus"
                  @click="openRuleCreate"
                >
                  新增规则
                </CsButton>
              </template>
            </CsToolbar>

            <CsContainer header="路由规则列表">
              <CsTable
                compact
                :wrap-lines="false"
                track-by="id"
                :items="filteredRoutingRules"
                :columns="routingColumns"
                :loading="routingLoading"
                loading-text="加载中…"
                :empty-text="ruleEmptyText"
                aria-label="路由规则列表"
              >
                <template #cell-priority="{ row }">
                  <span class="cell-mono" :class="{ 'cell-muted': !row.enabled }">
                    {{ row.priority ?? 0 }}
                  </span>
                </template>

                <template #cell-inbound="{ row }">
                  <div v-if="row.inboundTagList.length" class="chip-container" :class="{ 'is-muted': !row.enabled }">
                    <span v-for="tag in row.inboundTagList" :key="tag" class="tag-chip">{{ tag }}</span>
                  </div>
                  <span v-else class="cell-muted">全部入站</span>
                </template>

                <template #cell-rules="{ row }">
                  <div v-if="row.visibleChips.length" class="chip-container" :class="{ 'is-muted': !row.enabled }">
                    <span
                      v-for="(chip, idx) in row.visibleChips"
                      :key="idx"
                      class="rule-chip"
                      :title="chip.full"
                    >
                      <span class="rule-chip-label">{{ chip.label }}</span>
                      <span class="rule-chip-value">{{ chip.value }}</span>
                    </span>
                    <CsTooltip v-if="row.hiddenCount > 0" :content="row.hiddenTooltip">
                      <span class="rule-chip rule-chip--more">+{{ row.hiddenCount }}</span>
                    </CsTooltip>
                  </div>
                  <span v-else class="cell-muted">全部流量</span>
                </template>

                <template #cell-outbound="{ row }">
                  <div class="outbound-cell" :class="{ 'is-muted': !row.enabled }">
                    <CsBadge class="nowrap-badge" :color="outboundBadgeColor(row.outbound_tag)">
                      {{ row.outbound_tag }}
                    </CsBadge>
                    <CsBadge v-if="row.owner_inbound_id" class="nowrap-badge" color="grey">系统直通</CsBadge>
                  </div>
                </template>

                <template #cell-remark="{ row }">
                  <span
                    v-if="row.remark"
                    class="row-remark-cell"
                    :class="{ 'cell-muted': !row.enabled }"
                    :title="row.remark"
                  >
                    {{ row.remark }}
                  </span>
                  <span v-else class="cell-muted">—</span>
                </template>

                <template #cell-enabled="{ row }">
                  <CsTooltip
                    :disabled="!row.owner_inbound_id"
                    :content="`随四层直通「${row.ownerInboundTag || '#' + row.owner_inbound_id}」自动维护，不可单独启停`"
                  >
                    <CsToggle
                      :model-value="row.enabled"
                      :disabled="!!row.owner_inbound_id"
                      aria-label="启用此路由规则"
                      @change="toggleRule(row)"
                    />
                  </CsTooltip>
                </template>

                <template #cell-actions="{ row }">
                  <CsButton :wrap-text="false" @click="openRuleEdit(row)">编辑</CsButton>
                </template>
              </CsTable>
            </CsContainer>
          </CsSpaceBetween>
        </template>

        <!-- Tab 2: 全局策略管理 -->
        <template v-else>
          <CsEmpty v-if="!currentServer" description="请在顶部工具栏选择目标服务器" />

          <CsSpaceBetween v-else size="l">
            <CsHeader
              variant="h2"
              :description="`配置「${currentServer.name}」的默认兜底出口与路由匹配策略。保存后系统将自动重新编译并推送配置到服务器。`"
            >
              服务器全局路由与出口策略
            </CsHeader>

            <CsContainer>
              <template #header>
                <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                  <CsIcon name="status-info" size="small" />
                  <span>默认兜底出口</span>
                  <TipIcon
                    content="当客户端请求未命中任何路由规则时，默认兜底转发的目标出口出站标签。该出站会被排到 outbounds 列表首位作为默认出口。"
                  />
                </CsSpaceBetween>
              </template>

              <CsSpaceBetween size="xs">
                <CsSelect
                  :model-value="defaultOutboundTag"
                  :options="outboundOptions"
                  :disabled="outboundTags.length === 0"
                  aria-label="默认兜底出口"
                  @change="onDefaultOutboundChange"
                />
                <span v-if="outboundTags.length > 0" class="cs-text-secondary">
                  当前选用出站通道：<code class="cs-text-mono">{{ defaultOutboundTag }}</code>
                </span>
                <CsAlert v-else type="warning">
                  暂无可用出站通道，请先在 <CsLink @follow="goOutbounds">出站管理</CsLink> 添加出站
                </CsAlert>
              </CsSpaceBetween>
            </CsContainer>

            <CsContainer>
              <template #header>
                <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                  <CsIcon name="share" size="small" />
                  <span>路由匹配策略</span>
                  <TipIcon
                    content="Xray 路由规则匹配阶段的域名与 IP 解析策略，作用于全部路由规则。IPIfNonMatch=先按域名匹配，未命中则解析 IP 再次匹配。"
                  />
                </CsSpaceBetween>
              </template>

              <CsSpaceBetween size="xs">
                <CsSelect
                  :model-value="routingDomainStrategy"
                  :options="domainStrategyOptions"
                  aria-label="路由匹配策略"
                  @change="onDomainStrategyChange"
                />
                <span class="cs-text-secondary">
                  注：此为全局路由匹配策略；freedom 出站自身的连接域名解析策略在出站管理页的出站列表内设置。
                </span>
              </CsSpaceBetween>
            </CsContainer>

            <div class="settings-actions">
              <CsButton
                variant="primary"
                :loading="defaultSaving"
                icon-name="check"
                @click="saveDefaultOutbound"
              >
                保存全局策略
              </CsButton>
            </div>
          </CsSpaceBetween>
        </template>
      </CsTabs>

      <!-- 路由规则编辑器（自管 dialog） -->
      <RoutingRuleEditor
        v-if="ruleEditorOpen"
        :server-id="serverFilter ?? 0"
        :rule="ruleEditing"
        :outbound-tags="outboundTags"
        :inbound-tags="inboundTags"
        @saved="loadRouting"
        @close="ruleEditorOpen = false"
      />
    </template>

    <!-- 画布视图（T8） -->
    <template v-else>
      <TopologyCanvas
        :topology="topology"
        :editable="true"
        @changed="loadTopology"
        @open-server="openServer"
        @open-create-inbound="openInboundCreateForServer"
        @open-create-outbound="openOutboundCreateForServer"
      />
    </template>

    <!-- 双击画布盒子 → 服务器设置弹窗 -->
    <ServerSettingsModal
      v-if="drawerOpen"
      v-model="drawerOpen"
      :server="drawerServer"
      @removed="onSettingsRemoved"
      @changed="onSettingsChanged"
    />
  </CsPage>
</template>

<style scoped lang="scss">
.server-filter {
  inline-size: 220px;
}
.rule-filter-search {
  inline-size: 260px;
}
.rule-filter-select {
  inline-size: 160px;
}
.chip-container {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;

  &.is-muted {
    opacity: 0.6;
  }
}
.tag-chip,
.rule-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  line-height: 1.4;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-body-default-b2us04, #0f141a);
  background: var(--x-card-soft, #f8fafc);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  padding: 1px 6px;
  border-radius: 4px;
  white-space: nowrap;
}
.rule-chip-label {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
.rule-chip-value {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rule-chip--more {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  cursor: default;
}
.outbound-cell {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;

  &.is-muted {
    opacity: 0.6;
  }
}
.row-remark-cell {
  display: inline-block;
  max-width: 180px;
  vertical-align: middle;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}
.settings-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.topo-heading {
  display: flex;
  align-items: baseline;
  gap: 8px;
  white-space: nowrap;
}
.topo-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-heading-default-e3tuze, #0f141a);
}
.topo-count {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

@media (max-width: $bp-tablet-max) {
  .server-filter {
    inline-size: 100%;
  }

  .rule-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-xs-m9b7jf, 8px);
  }

  :deep(.rule-toolbar .cs-toolbar-main) {
    inline-size: 100%;

    .rule-filter-search,
    .rule-filter-select {
      inline-size: 100%;
    }
  }

  :deep(.rule-toolbar .cs-toolbar-actions) {
    inline-size: 100%;
    justify-content: flex-end;
  }
}
</style>
