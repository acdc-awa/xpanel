<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Edit, Delete, Check, Aim, Compass, Connection, Grid, Share, Loading } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import OutboundConfigEditor from './servers/OutboundConfigEditor.vue'
import RoutingRuleEditor from './servers/RoutingRuleEditor.vue'
import ServerNodeDrawer from './servers/ServerNodeDrawer.vue'
import TopologyCanvas from '@/components/topology/TopologyCanvas.vue'
import {
  deleteServerOutbound,
  deleteServerRoutingRule,
  getInbounds,
  getServerOutbounds,
  getServerRoutingRules,
  getServers,
  getTopology,
  updateServer,
  updateServerOutbound,
  updateServerRoutingRule,
  type ServerItem,
  type ServerOutbound,
  type ServerRoutingRule,
  type TopologyData,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const route = useRoute()
const router = useRouter()

const isMobile = ref(false)
let mq: MediaQueryList | null = null
const onMq = (e: MediaQueryListEvent | MediaQueryList) => {
  isMobile.value = e.matches
}
onMounted(() => {
  mq = window.matchMedia('(max-width: 768px)')
  onMq(mq)
  mq.addEventListener('change', onMq)
})
onUnmounted(() => mq?.removeEventListener('change', onMq))

const servers = ref<ServerItem[]>([])
const serverFilter = ref<number | undefined>(undefined)
const serverLoading = ref(false)

const currentServer = computed(() => servers.value.find((s) => s.id === serverFilter.value) ?? null)

async function loadServers() {
  serverLoading.value = true
  try {
    const { data } = await getServers()
    if (data.code === 0) {
      servers.value = data.data.items
      // 已选服务器被移除时回落（l4_relay 类型已于 2026-08-24 退役，无需再排除）
      if (serverFilter.value && !servers.value.some((s) => s.id === serverFilter.value)) {
        serverFilter.value = undefined
      }
      if (!serverFilter.value) serverFilter.value = servers.value[0]?.id
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载服务器失败'))
  } finally {
    serverLoading.value = false
  }
}

// ---- 默认出口 & 出站/路由策略（随服务器切换） ----
const defaultOutboundTag = ref('direct')
const defaultOutboundDS = ref('AsIs')
const routingDomainStrategy = ref('AsIs')
const defaultSaving = ref(false)

watch(currentServer, (s) => {
  if (s) {
    defaultOutboundTag.value = s.default_outbound_tag || 'direct'
    defaultOutboundDS.value = s.default_outbound_domain_strategy || 'AsIs'
    routingDomainStrategy.value = s.routing_domain_strategy || 'AsIs'
  }
})

const outboundTags = computed(() => outbounds.value.map((o) => o.tag))

async function saveDefaultOutbound() {
  if (!currentServer.value) return
  defaultSaving.value = true
  try {
    const { data } = await updateServer(currentServer.value.id, {
      default_outbound_tag: defaultOutboundTag.value,
      default_outbound_domain_strategy: defaultOutboundDS.value,
      routing_domain_strategy: routingDomainStrategy.value,
    })
    if (data.code === 0) {
      ElMessage.success('默认出口与策略已更新，下次配置推送生效')
      const s = currentServer.value
      if (s) {
        s.default_outbound_tag = defaultOutboundTag.value
        s.default_outbound_domain_strategy = defaultOutboundDS.value
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

// ---- 出站 ----
const outbounds = ref<ServerOutbound[]>([])
const outboundsLoading = ref(false)
const outboundEditorOpen = ref(false)
const outboundEditing = ref<ServerOutbound | null>(null)

async function loadOutbounds() {
  if (!serverFilter.value) return
  outboundsLoading.value = true
  try {
    const { data } = await getServerOutbounds(serverFilter.value)
    if (data.code === 0) outbounds.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载出站失败'))
  } finally {
    outboundsLoading.value = false
  }
}

function openOutboundCreate() {
  outboundEditing.value = null
  outboundEditorOpen.value = true
}

function openOutboundEdit(row: any) {
  outboundEditing.value = row
  outboundEditorOpen.value = true
}

async function toggleOutbound(row: any) {
  if (!serverFilter.value) return
  try {
    const { data } = await updateServerOutbound(serverFilter.value, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      ElMessage.success(data.data.outbound.enabled ? '已启用' : '已停用')
      loadOutbounds()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function removeOutbound(row: any) {
  if (!serverFilter.value) return
  try {
    await ElMessageBox.confirm(`确认删除出站「${row.tag}」？`, '删除出站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteServerOutbound(serverFilter.value, row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadOutbounds()
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function isBlockCN(row: any): boolean {
  if (row.protocol !== 'freedom') return false
  try {
    const s = JSON.parse(row.settings_json || '{}')
    return !!s.block_cn
  } catch {
    return false
  }
}

// ---- 路由规则 ----
const routingRules = ref<ServerRoutingRule[]>([])
const routingLoading = ref(false)
const ruleEditorOpen = ref(false)
const ruleEditing = ref<ServerRoutingRule | null>(null)
const inboundTags = ref<string[]>([])

async function loadInboundTags() {
  if (!serverFilter.value) return
  try {
    const { data } = await getInbounds(serverFilter.value)
    if (data.code === 0) inboundTags.value = data.data.items.map((i) => i.tag)
  } catch {
    /* 标签加载失败不阻塞 */
  }
}

async function loadRouting() {
  if (!serverFilter.value) return
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

// ---- 服务器切换：重载表格数据 ----
watch(serverFilter, (v) => {
  router.replace({ query: v ? { server_id: v } : {} })
  if (!v) {
    outbounds.value = []
    routingRules.value = []
    inboundTags.value = []
    return
  }
  loadOutbounds()
  loadRouting()
  loadInboundTags()
})

function reloadTable() {
  loadServers()
  if (serverFilter.value) {
    loadOutbounds()
    loadRouting()
    loadInboundTags()
  }
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

function openInboundCreateForServer(serverId: number) {
  openServer(serverId)
}

function openOutboundCreateForServer(serverId: number) {
  serverFilter.value = serverId
  openOutboundCreate()
}

onMounted(async () => {
  const q = Number(route.query.server_id)
  if (q > 0) {
    serverFilter.value = q
    viewMode.value = 'table'
  }
  if (route.query.view === 'table') {
    viewMode.value = 'table'
  }
  await loadServers()
  if (viewMode.value === 'canvas') {
    await loadTopology()
  }
})
</script>

<template>
  <div class="x-page">
    <!-- 顶部工具栏 -->
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <template v-if="viewMode === 'table'">
          <el-select v-model="serverFilter" placeholder="选择目标服务器" style="width: 220px" :loading="serverLoading">
            <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
          </el-select>
          <el-button @click="reloadTable">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
        </template>
        <template v-else>
          <div class="canvas-badge">
            <span class="x-status-dot online" />
            <span style="font-weight: 600; font-size: 13.5px">全局可视化拓扑画布</span>
            <span class="muted" style="font-size: 12px; margin-left: 4px">（共 {{ topology?.servers?.length || 0 }} 个节点）</span>
          </div>
          <el-button size="small" @click="loadTopology">
            <el-icon><Refresh /></el-icon>&nbsp;刷新拓扑
          </el-button>
        </template>
      </div>

      <div style="display: flex; gap: 12px; align-items: center">
        <el-radio-group v-model="viewMode" size="default">
          <el-radio-button value="canvas">
            <span style="display: inline-flex; align-items: center; gap: 4px">
              <el-icon><Share /></el-icon>拓扑画布
            </span>
          </el-radio-button>
          <el-radio-button value="table">
            <span style="display: inline-flex; align-items: center; gap: 4px">
              <el-icon><Grid /></el-icon>表格视图
            </span>
          </el-radio-button>
        </el-radio-group>
      </div>
    </div>

    <!-- 表格视图 -->
    <template v-if="viewMode === 'table'">
      <!-- 默认出口与策略 Hero Grid -->
      <BaseCard v-if="currentServer" style="margin-bottom: 16px">
        <div class="hero-strategy-wrap">
          <div class="hero-card">
            <div class="hero-head">
              <span class="hero-icon"><el-icon><Aim /></el-icon></span>
              <span class="hero-title">默认出口 Outbound</span>
              <el-tooltip content="当客户端请求未命中任何路由规则时，默认兜底转发的目标出口出站标签。" placement="top">
                <span class="help-q">?</span>
              </el-tooltip>
            </div>
            <div class="hero-body">
              <el-select v-model="defaultOutboundTag" style="width: 100%" :disabled="outboundTags.length === 0">
                <el-option v-for="t in outboundTags" :key="t" :label="t" :value="t" />
              </el-select>
              <div class="hero-sub">
                <span v-if="outboundTags.length > 0" class="muted">当前选用：<code class="cell-mono">{{ defaultOutboundTag }}</code></span>
                <span v-else class="text-danger" style="font-size: 12px">暂无可用出站，请先在下方添加出站</span>
              </div>
            </div>
          </div>

          <div class="hero-card">
            <div class="hero-head">
              <span class="hero-icon"><el-icon><Compass /></el-icon></span>
              <span class="hero-title">出站域名解析 (Freedom DNS)</span>
              <el-tooltip content="默认出口（freedom）对目标域名的解析方式。AsIs=直连原域名，UseIP=主控/系统解析为 IP 后连接。" placement="top">
                <span class="help-q">?</span>
              </el-tooltip>
            </div>
            <div class="hero-body">
              <el-select v-model="defaultOutboundDS" style="width: 100%">
                <el-option label="AsIs（保持域名直连）" value="AsIs" />
                <el-option label="UseIP（解析为 IP 连接）" value="UseIP" />
                <el-option label="UseIPv4（仅解析 IPv4）" value="UseIPv4" />
                <el-option label="UseIPv6（仅解析 IPv6）" value="UseIPv6" />
              </el-select>
              <div class="hero-sub">
                <span class="muted">出站连接阶段生效</span>
              </div>
            </div>
          </div>

          <div class="hero-card">
            <div class="hero-head">
              <span class="hero-icon"><el-icon><Connection /></el-icon></span>
              <span class="hero-title">路由匹配策略 (DomainStrategy)</span>
              <el-tooltip content="Xray 路由规则匹配阶段的域名与 IP 解析策略。IPIfNonMatch=先按域名匹配，未命中则解析 IP 再次匹配。" placement="top">
                <span class="help-q">?</span>
              </el-tooltip>
            </div>
            <div class="hero-body">
              <el-select v-model="routingDomainStrategy" style="width: 100%">
                <el-option label="AsIs（保持原样）" value="AsIs" />
                <el-option label="IPIfNonMatch（先域名后 IP）" value="IPIfNonMatch" />
                <el-option label="IPOnDemand（按需解析 IP）" value="IPOnDemand" />
              </el-select>
              <div class="hero-sub">
                <span class="muted">规则分流阶段生效</span>
              </div>
            </div>
          </div>

          <div class="hero-action">
            <el-button type="primary" :loading="defaultSaving" style="width: 100%" @click="saveDefaultOutbound">
              <el-icon><Check /></el-icon>&nbsp;保存策略
            </el-button>
            <div class="muted" style="font-size: 11.5px; text-align: center; margin-top: 6px">
              修改后自动重编节点配置
            </div>
          </div>
        </div>
      </BaseCard>

      <BaseCard>
        <el-tabs>
          <!-- 出站管理 -->
          <el-tab-pane :label="isMobile ? '出站规则' : '出站规则 (Outbounds)'">
            <div class="tab-toolbar">
              <el-button size="small" :disabled="!serverFilter" @click="loadOutbounds"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
              <el-button size="small" type="primary" :disabled="!serverFilter" @click="openOutboundCreate"><el-icon><Plus /></el-icon>&nbsp;新增出站</el-button>
            </div>

            <div v-if="outboundsLoading" style="padding: 36px 0; text-align: center">
              <el-icon class="is-loading" style="font-size: 24px; color: var(--x-primary)"><Loading /></el-icon>
            </div>

            <div v-else-if="outbounds.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13px">
              该服务器暂无出站规则，点击右上角「新增出站」
            </div>

            <!-- 全局统一出站卡片网格流 (自适应 1~4 列) -->
            <div v-else class="routing-card-grid">
              <div v-for="row in outbounds" :key="row.id" class="routing-card" :class="{ disabled: !row.enabled }">
                <div class="card-head">
                  <div class="head-title">
                    <span class="tag-badge">{{ row.tag }}</span>
                    <span v-if="row.tag === defaultOutboundTag" class="x-chip green" style="font-size: 10px">默认出口</span>
                    <span v-if="row.tag === 'direct' || row.tag === 'blocked'" class="x-chip gray" style="font-size: 10px">系统内置</span>
                    <span v-if="isBlockCN(row)" class="x-chip red" style="font-size: 10px">阻断回国</span>
                  </div>
                  <el-switch :model-value="row.enabled" :disabled="row.tag === 'direct'" size="small" @change="toggleOutbound(row)" />
                </div>

                <div class="card-grid">
                  <div class="grid-item">
                    <span class="item-label">出站协议</span>
                    <div class="item-value">
                      <span v-if="row.protocol === 'freedom'" class="x-chip green" style="font-size: 10.5px">直连 (freedom)</span>
                      <span v-else-if="row.protocol === 'vless'" class="x-chip purple" style="font-size: 10.5px">VLESS 链</span>
                      <span v-else-if="row.protocol === 'blackhole'" class="x-chip red" style="font-size: 10.5px">黑洞 (blackhole)</span>
                      <span v-else class="x-chip blue" style="font-size: 10.5px">{{ row.protocol }}</span>
                    </div>
                  </div>
                  <div class="grid-item">
                    <span class="item-label">连接与引用</span>
                    <div class="item-value">
                      <span v-if="row.inbound_ref" class="x-chip orange" style="font-size: 10.5px">
                        引用入站 #{{ row.inbound_ref }}
                      </span>
                      <span v-else-if="row.tag === 'direct'" class="muted font-11">节点直连互联网</span>
                      <span v-else-if="row.tag === 'blocked'" class="muted font-11">黑洞丢弃阻断</span>
                      <span v-else class="muted font-11">自主直连</span>
                    </div>
                  </div>
                  <div v-if="row.send_through" class="grid-item">
                    <span class="item-label">发送出口 IP</span>
                    <div class="item-value cell-mono font-11">{{ row.send_through }}</div>
                  </div>
                  <div class="grid-item">
                    <span class="item-label">优先级</span>
                    <div class="item-value cell-mono font-11">{{ row.priority ?? 0 }}</div>
                  </div>
                  <div v-if="row.remark" class="grid-item full-width">
                    <span class="item-label">备注说明</span>
                    <div class="item-value font-11 muted">{{ row.remark }}</div>
                  </div>
                </div>

                <div class="card-foot-actions">
                  <el-button size="small" type="primary" plain @click="openOutboundEdit(row)">
                    <el-icon><Edit /></el-icon>&nbsp;编辑出站
                  </el-button>
                  <el-tooltip v-if="row.tag === 'direct' || row.tag === 'blocked'" content="系统内置出站不可删除" placement="top">
                    <span><el-button size="small" type="danger" disabled><el-icon><Delete /></el-icon>&nbsp;删除</el-button></span>
                  </el-tooltip>
                  <el-button v-else size="small" type="danger" plain @click="removeOutbound(row)">
                    <el-icon><Delete /></el-icon>&nbsp;删除
                  </el-button>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- 路由规则管理 -->
          <el-tab-pane :label="isMobile ? '分流规则' : '分流规则 (Routing Rules)'">
            <div class="tab-toolbar">
              <el-button size="small" :disabled="!serverFilter" @click="loadRouting"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
              <el-button size="small" type="primary" :disabled="!serverFilter" @click="openRuleCreate"><el-icon><Plus /></el-icon>&nbsp;新增规则</el-button>
            </div>

            <div v-if="routingLoading" style="padding: 36px 0; text-align: center">
              <el-icon class="is-loading" style="font-size: 24px; color: var(--x-primary)"><Loading /></el-icon>
            </div>

            <div v-else-if="routingRules.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13px">
              该服务器暂无分流规则，点击右上角「新增规则」
            </div>

            <!-- 全局统一分流规则卡片网格流 (自适应 1~4 列) -->
            <div v-else class="routing-card-grid">
              <div v-for="row in routingRules" :key="row.id" class="routing-card" :class="{ disabled: !row.enabled }">
                <div class="card-head">
                  <div class="head-title">
                    <span style="font-weight: 600; font-size: 12px; color: var(--x-text-3)">目标出站：</span>
                    <span class="x-chip purple" style="font-weight: 700; font-size: 11px">{{ row.outbound_tag }}</span>
                  </div>
                  <el-switch :model-value="row.enabled" size="small" @change="toggleRule(row)" />
                </div>

                <div class="card-grid">
                  <div class="grid-item full-width">
                    <span class="item-label">域名规则 (Domain)</span>
                    <div class="item-value">
                      <template v-if="parseTagList(row.domain).length">
                        <div class="chip-container">
                          <code v-for="(item, idx) in parseTagList(row.domain)" :key="idx" class="rule-chip">
                            {{ item }}
                          </code>
                        </div>
                      </template>
                      <span v-else class="muted font-11">—</span>
                    </div>
                  </div>
                  <div class="grid-item full-width">
                    <span class="item-label">IP 规则 (IP)</span>
                    <div class="item-value">
                      <template v-if="parseTagList(row.ip).length">
                        <div class="chip-container">
                          <code v-for="(item, idx) in parseTagList(row.ip)" :key="idx" class="rule-chip">
                            {{ item }}
                          </code>
                        </div>
                      </template>
                      <span v-else class="muted font-11">—</span>
                    </div>
                  </div>
                  <div class="grid-item">
                    <span class="item-label">入站来源</span>
                    <div class="item-value cell-mono font-11">{{ row.inbound_tag || '全部入站' }}</div>
                  </div>
                  <div class="grid-item">
                    <span class="item-label">端口 / 协议</span>
                    <div class="item-value">
                      <span v-if="row.port" class="cell-mono font-11">{{ row.port }}</span>
                      <span v-else class="muted font-11">全部端口</span>
                      <span v-for="p in parseTagList(row.protocol)" :key="p" class="x-chip blue" style="margin-left: 4px; font-size: 10px">
                        {{ p }}
                      </span>
                    </div>
                  </div>
                  <div class="grid-item">
                    <span class="item-label">规则优先级</span>
                    <div class="item-value cell-mono font-11">{{ row.priority ?? 0 }}</div>
                  </div>
                </div>

                <div class="card-foot-actions">
                  <el-button size="small" type="primary" plain @click="openRuleEdit(row)">
                    <el-icon><Edit /></el-icon>&nbsp;编辑规则
                  </el-button>
                  <el-button size="small" type="danger" plain @click="removeRule(row)">
                    <el-icon><Delete /></el-icon>&nbsp;删除
                  </el-button>
                </div>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </BaseCard>

      <!-- 出站编辑器（自管 dialog） -->
      <OutboundConfigEditor
        v-if="outboundEditorOpen"
        :server-id="serverFilter ?? 0"
        :outbound="outboundEditing"
        @saved="loadOutbounds"
        @close="outboundEditorOpen = false"
      />

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
          <span class="muted" style="font-size: 12px; margin-left: 6px">点击连线可删除；拖拽把手可建线；双击卡片管理节点</span>
        </div>
        <div style="display: flex; gap: 10px; align-items: center">
          <span style="font-size: 13px; color: var(--x-text-2)">编辑模式</span>
          <el-switch v-model="canvasEdit" />
        </div>
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

    <!-- 双击画布盒子 → 节点管理详情弹窗 -->
    <ServerNodeDrawer
      v-model="drawerOpen"
      :server="drawerServer"
      @removed="loadTopology"
      @changed="loadTopology"
    />
  </div>
</template>

<style scoped lang="scss">
.tab-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 12px;
}
.canvas-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.canvas-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  background: var(--x-card-bg, #fff);
  border: 1px solid var(--x-border);
  padding: 8px 14px;
  border-radius: var(--x-radius);
}
.canvas-tip {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tip-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--x-text-2);
  background: var(--x-bg);
  padding: 2px 8px;
  border-radius: 4px;
}
.legend-solid {
  display: inline-block;
  width: 12px;
  height: 2px;
  background: var(--x-primary);
}
.legend-dash {
  display: inline-block;
  width: 12px;
  height: 2px;
  border-top: 2px dashed #e6a23c;
}
.muted {
  color: var(--x-text-3);
}
.text-danger {
  color: var(--el-color-danger);
}
.cell-mono {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--x-text-2);
}
.tag-badge {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-weight: 600;
  color: var(--x-primary);
  font-size: 13px;
}
.table-empty {
  padding: 30px 0;
  color: var(--x-text-3);
}
.chip-container {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}
.rule-chip {
  font-family: var(--x-font-mono, monospace);
  font-size: 11px;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  padding: 1px 6px;
  border-radius: 4px;
  color: var(--x-text);
  white-space: nowrap;
}
.chip-more {
  font-size: 11px;
  color: var(--x-primary);
  background: var(--x-primary-soft);
  padding: 1px 5px;
  border-radius: 4px;
  cursor: pointer;
}

// Hero Strategy Grid
.hero-strategy-wrap {
  display: grid;
  grid-template-columns: repeat(3, 1fr) 140px;
  gap: 16px;
  align-items: center;
}
@media (max-width: 1100px) {
  .hero-strategy-wrap {
    grid-template-columns: 1fr 1fr;
  }
}
@media (max-width: 680px) {
  .hero-strategy-wrap {
    grid-template-columns: 1fr;
  }
}
.hero-card {
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 10px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.hero-head {
  display: flex;
  align-items: center;
  gap: 6px;
}
.hero-icon {
  display: flex;
  align-items: center;
  color: var(--x-primary);
  font-size: 15px;
}
.hero-title {
  font-weight: 600;
  font-size: 12.5px;
  color: var(--x-text-1);
}
.help-q {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--x-border);
  color: var(--x-text-3);
  font-size: 10px;
  cursor: help;
  margin-left: auto;
}
.hero-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.hero-sub {
  font-size: 11px;
  line-height: 1.3;
}
.hero-action {
  display: flex;
  flex-direction: column;
  justify-content: center;
}

/* ================= 全局统一出站/分流规则卡片网格流 ================= */
.routing-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.routing-card {
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

  &.disabled {
    opacity: 0.75;
    background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
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

@media (max-width: 640px) {
  .routing-card-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .x-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;

    .x-toolbar-left {
      width: 100%;
      flex-direction: column;
      align-items: stretch;
      gap: 8px;

      .el-select {
        width: 100% !important;
      }
    }

    .el-radio-group {
      width: 100%;
      display: flex;

      .el-radio-button {
        flex: 1;

        :deep(.el-radio-button__inner) {
          width: 100%;
          display: flex;
          justify-content: center;
        }
      }
    }
  }

  .canvas-bar {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;

    .canvas-tip {
      flex-wrap: wrap;
    }
  }

  :deep(.el-tabs__header) {
    margin-bottom: 12px;
  }
  :deep(.el-tabs__nav-wrap) {
    padding: 0 !important;
  }
  :deep(.el-tabs__nav-prev),
  :deep(.el-tabs__nav-next) {
    display: none !important;
  }
  :deep(.el-tabs__item) {
    padding: 0 14px !important;
    font-size: 13.5px !important;
  }
}
</style>

