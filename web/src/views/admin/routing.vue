<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Edit, Delete } from '@element-plus/icons-vue'
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
      if (!serverFilter.value && servers.value.length > 0) serverFilter.value = servers.value[0].id
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载服务器失败'))
  } finally {
    serverLoading.value = false
  }
}

// ---- 默认出口 & 域名策略（随服务器切换） ----
const defaultOutboundTag = ref('direct')
const routingDomainStrategy = ref('AsIs')
const defaultSaving = ref(false)

watch(currentServer, (s) => {
  if (s) {
    defaultOutboundTag.value = s.default_outbound_tag || 'direct'
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
      routing_domain_strategy: routingDomainStrategy.value,
    })
    if (data.code === 0) {
      ElMessage.success('默认出口已更新，下次生成配置生效')
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
const viewMode = ref<'table' | 'canvas'>('table')
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

// 双击画布盒子 → 打开服务器抽屉
const drawerOpen = ref(false)
const drawerServer = ref<ServerItem | null>(null)

function openServer(serverId: number) {
  drawerServer.value = servers.value.find((s) => s.id === serverId) ?? null
  drawerOpen.value = true
}

onMounted(async () => {
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  await loadServers()
})
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-select v-model="serverFilter" placeholder="选择服务器" style="width: 200px" :loading="serverLoading">
          <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
        <el-button @click="viewMode === 'table' ? reloadTable() : loadTopology()">
          <el-icon><Refresh /></el-icon>&nbsp;刷新
        </el-button>
      </div>
      <el-radio-group v-model="viewMode" size="default">
        <el-radio-button value="table">表格</el-radio-button>
        <el-radio-button value="canvas">画布</el-radio-button>
      </el-radio-group>
    </div>

    <el-alert
      type="info"
      :closable="false"
      show-icon
      :title="viewMode === 'table'
        ? '按服务器管理出站与路由规则；切到「画布」可拖线编辑：出站→入站建引用（跨服务器实线）、入站→出站建路由规则（盒内虚线）、点线删除、双击盒子打开节点管理。'
        : '拖线编辑：出站右侧 → 其他服务器入站左侧 = 建立 InboundRef 引用（实线）；入站右侧 → 本盒出站左侧 = 建立路由规则（虚线）；点击已有连线可删除；双击盒子打开节点管理。'"
      style="margin-bottom: 14px"
    />

    <!-- 表格视图 -->
    <template v-if="viewMode === 'table'">
      <BaseCard v-if="currentServer" title="默认出口与域名策略" style="margin-bottom: 14px">
        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap">
          <span style="font-weight: 600; font-size: 13px; color: var(--x-text-2); white-space: nowrap">默认出口：</span>
          <el-select v-model="defaultOutboundTag" style="width: 160px" size="small" :disabled="outboundTags.length === 0">
            <el-option v-for="t in outboundTags" :key="t" :label="t" :value="t" />
          </el-select>
          <span style="font-weight: 600; font-size: 13px; color: var(--x-text-2); white-space: nowrap; margin-left: 12px">域名策略：</span>
          <el-select v-model="routingDomainStrategy" style="width: 180px" size="small">
            <el-option label="AsIs（保持原样）" value="AsIs" />
            <el-option label="IPIfNonMatch（先域名后 IP）" value="IPIfNonMatch" />
            <el-option label="IPOnDemand（按需解析 IP）" value="IPOnDemand" />
          </el-select>
          <el-button size="small" type="primary" :loading="defaultSaving" @click="saveDefaultOutbound">保存</el-button>
          <span v-if="outboundTags.length === 0" class="muted" style="font-size: 12px">（暂无出站，请先在「出站」中添加）</span>
        </div>
      </BaseCard>

      <BaseCard>
        <el-tabs>
          <!-- 出站 -->
          <el-tab-pane label="出站">
            <div class="tab-toolbar">
              <el-button size="small" :disabled="!serverFilter" @click="loadOutbounds"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
              <el-button size="small" type="primary" :disabled="!serverFilter" @click="openOutboundCreate"><el-icon><Plus /></el-icon>&nbsp;新增出站</el-button>
            </div>
            <el-table v-loading="outboundsLoading" :data="outbounds" size="small">
              <el-table-column prop="tag" label="标签" min-width="130">
                <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
              </el-table-column>
              <el-table-column prop="protocol" label="协议" width="110">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.protocol === 'vless' ? 'warning' : row.protocol === 'blackhole' ? 'danger' : 'success'">{{ row.protocol }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="引用落地" width="100">
                <template #default="{ row }">
                  <el-tag v-if="row.inbound_ref" size="small" type="warning">InboundRef</el-tag>
                  <span v-else class="muted">—</span>
                </template>
              </el-table-column>
              <el-table-column prop="send_through" label="发送 IP" width="120">
                <template #default="{ row }">{{ row.send_through || '—' }}</template>
              </el-table-column>
              <el-table-column prop="priority" label="优先级" width="80" />
              <el-table-column label="状态" width="80">
                <template #default="{ row }"><el-switch :model-value="row.enabled" @change="toggleOutbound(row)" /></template>
              </el-table-column>
              <el-table-column prop="remark" label="备注" min-width="120">
                <template #default="{ row }">{{ row.remark || '—' }}</template>
              </el-table-column>
              <el-table-column label="操作" width="120" fixed="right">
                <template #default="{ row }">
                  <el-button size="small" text @click="openOutboundEdit(row)"><el-icon><Edit /></el-icon></el-button>
                  <el-button size="small" text type="danger" @click="removeOutbound(row)"><el-icon><Delete /></el-icon></el-button>
                </template>
              </el-table-column>
              <template #empty><div class="table-empty">尚未配置出站，点击右上角「新增出站」</div></template>
            </el-table>
          </el-tab-pane>

          <!-- 路由规则 -->
          <el-tab-pane label="路由规则">
            <div class="tab-toolbar">
              <el-button size="small" :disabled="!serverFilter" @click="loadRouting"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
              <el-button size="small" type="primary" :disabled="!serverFilter" @click="openRuleCreate"><el-icon><Plus /></el-icon>&nbsp;新增规则</el-button>
            </div>
            <el-table v-loading="routingLoading" :data="routingRules" size="small">
              <el-table-column prop="outbound_tag" label="出站标签" min-width="120">
                <template #default="{ row }"><el-tag size="small">{{ row.outbound_tag }}</el-tag></template>
              </el-table-column>
              <el-table-column label="域名匹配" min-width="140">
                <template #default="{ row }"><span class="ellipsis-text">{{ row.domain || '—' }}</span></template>
              </el-table-column>
              <el-table-column label="IP 匹配" min-width="140">
                <template #default="{ row }"><span class="ellipsis-text">{{ row.ip || '—' }}</span></template>
              </el-table-column>
              <el-table-column prop="protocol" label="协议" width="100">
                <template #default="{ row }"><span class="ellipsis-text">{{ row.protocol || '—' }}</span></template>
              </el-table-column>
              <el-table-column label="入站标签" min-width="110">
                <template #default="{ row }"><span class="ellipsis-text">{{ row.inbound_tag || '—' }}</span></template>
              </el-table-column>
              <el-table-column prop="network" label="网络" width="70">
                <template #default="{ row }">{{ row.network || '—' }}</template>
              </el-table-column>
              <el-table-column prop="port" label="端口" width="80">
                <template #default="{ row }">{{ row.port || '—' }}</template>
              </el-table-column>
              <el-table-column prop="priority" label="优先级" width="80" />
              <el-table-column label="状态" width="80">
                <template #default="{ row }">
                  <el-switch :model-value="row.enabled" @change="toggleRule(row)" />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="120" fixed="right">
                <template #default="{ row }">
                  <el-button size="small" text @click="openRuleEdit(row)"><el-icon><Edit /></el-icon></el-button>
                  <el-button size="small" text type="danger" @click="removeRule(row)"><el-icon><Delete /></el-icon></el-button>
                </template>
              </el-table-column>
              <template #empty><div class="table-empty">尚未配置路由规则，点击右上角「新增规则」</div></template>
            </el-table>
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
      <div class="canvas-toolbar">
        <span class="muted" style="font-size: 12px">
          实线 = InboundRef 引用（跨服务器）；虚线 = 路由规则（盒内）；连线可点选删除
        </span>
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
      />
    </template>

    <!-- 双击画布盒子 → 节点管理抽屉 -->
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
.canvas-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.muted {
  color: var(--x-text-3);
}
.table-empty {
  padding: 30px 0;
  color: var(--x-text-3);
}
.ellipsis-text {
  display: inline-block;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
