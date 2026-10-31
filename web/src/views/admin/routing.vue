<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { MQ, useMediaQuery } from '@/utils/media'
import {
  ElMessage,
  ElMessageBox,
  CsAlert,
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsEmpty,
  CsHeader,
  CsIcon,
  CsLink,
  CsLoading,
  CsPage,
  CsSegmentedControl,
  CsSelect,
  CsSpaceBetween,
  CsStatusIndicator,
  CsTabs,
  CsToggle,
  CsToolbar,
  CsTooltip,
  type SegmentedOption,
  type TabItem,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import RoutingRuleEditor from './servers/RoutingRuleEditor.vue'
import ServerSettingsModal from './servers/ServerSettingsModal.vue'
import TopologyCanvas from '@/components/topology/TopologyCanvas.vue'
import {
  deleteServerRoutingRule,
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
  { id: 'rules', label: '路由规则 (Routing Rules)' },
  { id: 'settings', label: '全局策略 (Global Settings)' },
]

const viewOptions: SegmentedOption[] = [
  { id: 'canvas', text: '拓扑画布', iconName: 'share' },
  { id: 'table', text: '表格视图', iconName: 'grid-view' },
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

type EffectiveRoutingRule = ServerRoutingRule & {
  /** 派生自四层直通：可编辑（改指向/加条件/调优先级），不可删除 */
  ownerInboundTag?: string
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
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载出站选项失败'))
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
      ElMessage.success('默认出口与路由匹配策略已更新，下次配置推送生效')
      const s = currentServer.value
      if (s) {
        s.default_outbound_tag = defaultOutboundTag.value
        s.routing_domain_strategy = routingDomainStrategy.value
      }
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存默认出口失败'))
  } finally {
    defaultSaving.value = false
  }
}

// ---- 路由规则 ----
const routingRules = ref<ServerRoutingRule[]>([])
const routingLoading = ref(false)
const ruleEditorOpen = ref(false)
const ruleEditing = ref<ServerRoutingRule | null>(null)

const displayRoutingRules = computed<EffectiveRoutingRule[]>(() =>
  routingRules.value.map((r) => ({
    ...r,
    ownerInboundTag: r.owner_inbound_id ? ownerTagOf.value[r.owner_inbound_id] || '' : '',
  }))
)

async function loadRouting() {
  if (!serverFilter.value) {
    routingRules.value = []
    return
  }
  routingLoading.value = true
  try {
    const { data } = await getServerRoutingRules(serverFilter.value)
    if (data.code === 0) routingRules.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载路由规则失败'))
  } finally {
    routingLoading.value = false
  }
}

function openRuleCreate() {
  ruleEditing.value = null
  ruleEditorOpen.value = true
}

function openRuleEdit(row: any) {
  ruleEditing.value = row
  ruleEditorOpen.value = true
}

async function toggleRule(row: any) {
  if (!serverFilter.value) return
  try {
    const { data } = await updateServerRoutingRule(serverFilter.value, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      ElMessage.success(data.data.rule.enabled ? '已启用' : '已停用')
      loadRouting()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function removeRule(row: any) {
  if (!serverFilter.value) return
  try {
    await ElMessageBox.confirm(`确认删除路由规则「${row.outbound_tag}」？`, '删除路由规则', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteServerRoutingRule(serverFilter.value, row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadRouting()
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// 辅助：将多行或逗号分隔的文本拆解为数组
function parseTagList(val?: string): string[] {
  if (!val) return []
  return val.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean)
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
const canvasEdit = ref(true)

async function loadTopology() {
  try {
    const { data } = await getTopology()
    if (data.code === 0) topology.value = data.data
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载拓扑失败'))
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
        <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
          <CsStatusIndicator type="success" text="全局可视化拓扑画布" />
          <span class="cs-text-secondary">（共 {{ topology?.servers?.length || 0 }} 个服务器）</span>
        </CsSpaceBetween>
        <CsButton icon-name="refresh" @click="loadTopology">刷新拓扑</CsButton>
      </template>

      <template #actions>
        <CsSegmentedControl :model-value="viewMode" :options="viewOptions" label="视图" @change="onViewChange" />
      </template>
    </CsToolbar>

    <!-- 表格视图 -->
    <template v-if="viewMode === 'table'">
      <CsContainer>
        <CsTabs :model-value="activeTab" :tabs="tabItems" @change="onTabChange">
          <!-- Tab 1: 路由规则管理 -->
          <template v-if="activeTab === 'rules'">
            <CsSpaceBetween size="s">
              <CsToolbar>
                <template #actions>
                  <CsButton :disabled="!serverFilter" icon-name="refresh" @click="loadRouting">刷新</CsButton>
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

              <CsLoading :loading="routingLoading" min-height="200px">
                <CsEmpty
                  v-if="displayRoutingRules.length === 0"
                  description="该服务器暂无路由规则，点击右上角「新增规则」"
                  icon-name="list-view"
                />

                <CsCardGrid v-else>
                  <CsCard v-for="row in displayRoutingRules" :key="row.id">
                    <template #header>
                      <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                        <span class="cs-text-secondary">目标出站：</span>
                        <CsBadge color="blue">{{ row.outbound_tag }}</CsBadge>
                        <CsBadge v-if="row.owner_inbound_id" color="grey">系统内置·直通</CsBadge>
                      </CsSpaceBetween>
                    </template>

                    <template #actions>
                      <CsToggle
                        :model-value="row.enabled"
                        :disabled="!!row.owner_inbound_id"
                        aria-label="启用此路由规则"
                        @change="toggleRule(row)"
                      />
                    </template>

                    <CsCardFacts>
                      <CsCardFact full-width label="域名规则 (Domain)">
                        <div v-if="parseTagList(row.domain).length" class="chip-container">
                          <code v-for="(item, idx) in parseTagList(row.domain)" :key="idx" class="rule-chip">
                            {{ item }}
                          </code>
                        </div>
                        <span v-else class="cs-text-secondary">—</span>
                      </CsCardFact>

                      <CsCardFact full-width label="IP 规则 (IP)">
                        <div v-if="parseTagList(row.ip).length" class="chip-container">
                          <code v-for="(item, idx) in parseTagList(row.ip)" :key="idx" class="rule-chip">
                            {{ item }}
                          </code>
                        </div>
                        <span v-else class="cs-text-secondary">—</span>
                      </CsCardFact>

                      <CsCardFact label="入站来源" :value="row.inbound_tag || '全部入站'" mono />
                      <CsCardFact label="规则优先级" :value="row.priority ?? 0" mono />

                      <CsCardFact label="端口 / 协议">
                        <div class="chip-container">
                          <span v-if="row.port" class="cs-text-mono">{{ row.port }}</span>
                          <span v-else class="cs-text-secondary">全部端口</span>
                          <CsBadge v-for="p in parseTagList(row.protocol)" :key="p" color="blue">{{ p }}</CsBadge>
                        </div>
                      </CsCardFact>
                    </CsCardFacts>

                    <template #footer>
                      <div class="card-foot-actions">
                        <CsButton icon-name="edit" @click="openRuleEdit(row)">编辑规则</CsButton>
                        <CsTooltip
                          v-if="row.owner_inbound_id"
                          :content="`随四层直通「${row.ownerInboundTag || '#' + row.owner_inbound_id}」自动维护，不可删除；要停用请停用或删除该直通`"
                        >
                          <span><CsButton variant="danger-outline" icon-name="remove" disabled>删除</CsButton></span>
                        </CsTooltip>
                        <CsButton v-else variant="danger-outline" icon-name="remove" @click="removeRule(row)">
                          删除
                        </CsButton>
                      </div>
                    </template>
                  </CsCard>
                </CsCardGrid>
              </CsLoading>
            </CsSpaceBetween>
          </template>

          <!-- Tab 2: 全局策略管理 -->
          <template v-else>
            <CsEmpty v-if="!currentServer" description="请在顶部工具栏选择目标服务器" />

            <CsSpaceBetween v-else size="l">
              <CsHeader
                variant="h2"
                :description="`配置「${currentServer.name}」的默认兜底出口与路由匹配策略。保存后系统将自动重新编译并推送配置到节点。`"
              >
                服务器全局路由与出口策略
              </CsHeader>

              <CsContainer>
                <template #header>
                  <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                    <CsIcon name="status-info" size="small" />
                    <span>默认兜底出口 (Default Outbound)</span>
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
                    <span>路由匹配策略 (DomainStrategy)</span>
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
                    注：此为全局路由匹配策略；freedom 出站自身的连接域名解析策略在出站管理页的出站卡片内设置。
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
                <span class="cs-text-secondary">修改后自动重新编译并推送配置</span>
              </div>
            </CsSpaceBetween>
          </template>
        </CsTabs>
      </CsContainer>

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
      <div class="canvas-bar">
        <div class="canvas-tip">
          <span class="tip-pill"><span class="legend-solid" /> 实线 InboundRef 引用</span>
          <span class="tip-pill"><span class="legend-dash" /> 虚线 路由规则</span>
          <span class="cs-text-secondary">点击连线可删除；拖拽把手可建线；双击卡片管理服务器</span>
        </div>
        <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
          <span>编辑模式</span>
          <CsToggle v-model="canvasEdit" aria-label="编辑模式" />
        </CsSpaceBetween>
      </div>
      <TopologyCanvas
        :topology="topology"
        :editable="canvasEdit"
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
.chip-container {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}
.rule-chip {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-size: 11px;
  background: var(--color-background-container-content-mjfil9, #fff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  padding: 1px 6px;
  border-radius: 4px;
  white-space: nowrap;
}
.card-foot-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.settings-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.canvas-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  background: var(--color-background-container-content-mjfil9, #fff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  padding: 10px 16px;
  border-radius: 8px;
}
.canvas-tip {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.tip-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  background: var(--color-background-container-content-mjfil9, #fff);
  padding: 2px 8px;
  border-radius: 4px;
}
.legend-solid {
  display: inline-block;
  inline-size: 12px;
  block-size: 2px;
  background: var(--color-text-link-default-qabnuk, #0972d3);
}
.legend-dash {
  display: inline-block;
  inline-size: 12px;
  block-size: 2px;
  border-block-start: 2px dashed var(--color-text-status-warning-fabq6h, #e6a23c);
}

@media (max-width: $bp-tablet-max) {
  .server-filter {
    inline-size: 100%;
  }
  .canvas-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
