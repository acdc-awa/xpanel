<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Plus,
  Delete,
  Edit,
  Refresh,
  Key,
  CopyDocument,
  VideoPlay,
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import InboundConfigEditor from './InboundConfigEditor.vue'
import OutboundConfigEditor from './OutboundConfigEditor.vue'
import RoutingRuleEditor from './RoutingRuleEditor.vue'
import {
  createInbound,
  deleteInbound,
  deleteServer,
  deleteServerOutbound,
  deleteServerRoutingRule,
  generateAndPushConfig,
  getInbounds,
  getServerOutbounds,
  getServerRoutingRules,
  resetServerSecret,
  toggleInbound,
  updateInbound,
  updateServerOutbound,
  updateServerRoutingRule,
  type InboundItem,
  type InboundPayload,
  type ServerItem,
  type ServerOutbound,
  type ServerRoutingRule,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import type { InboundEditorChangePayload } from './InboundConfigEditor.vue'

const props = defineProps<{
  modelValue: boolean
  server: ServerItem | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'removed'): void
  (e: 'changed'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const activeTab = ref('overview')

function fmtTime(t: string | null) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

// ---- 概览：重置密钥 / 删除 ----
const secretInfo = ref<{ node_id: string; secret: string } | null>(null)
const resetting = ref(false)

async function resetSecret() {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(
      `确认重置节点「${props.server.name}」的密钥？旧密钥立即失效，需在节点 Agent 配置（/etc/xray-agent/config.yml）中更新。`,
      '重置密钥',
      { type: 'warning' },
    )
  } catch {
    return
  }
  resetting.value = true
  try {
    const { data } = await resetServerSecret(props.server.id)
    if (data.code === 0) {
      secretInfo.value = { node_id: data.data.node_id, secret: data.data.secret }
      ElMessage.success('密钥已重置')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  } finally {
    resetting.value = false
  }
}

const deleting = ref(false)
async function removeServer() {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(
      `确认删除节点「${props.server.name}」？关联的入站、出站、路由规则、待推送配置与节点上报将一并删除，该节点 Agent 将无法再连接。`,
      '删除服务器',
      { type: 'error' },
    )
  } catch {
    return
  }
  deleting.value = true
  try {
    const { data } = await deleteServer(props.server.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      emit('removed')
      visible.value = false
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  } finally {
    deleting.value = false
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

// ---- 入站 Tab ----
const inbounds = ref<InboundItem[]>([])
const inboundsLoading = ref(false)

async function loadInbounds() {
  if (!props.server) return
  inboundsLoading.value = true
  try {
    const { data } = await getInbounds(props.server.id)
    if (data.code === 0) inbounds.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载入站失败'))
  } finally {
    inboundsLoading.value = false
  }
}

const inboundEditorOpen = ref(false)
const inboundEditing = ref<InboundItem | null>(null)
const inboundSaving = ref(false)

const inboundChange = ref<InboundEditorChangePayload | null>(null)

function onInboundChange(payload: InboundEditorChangePayload) {
  inboundChange.value = payload
}

function openInboundCreate() {
  inboundEditing.value = null
  inboundChange.value = {
    settingsJson: '{}',
    streamSettings: '{"network":"tcp","security":"reality"}',
    sniffing: '',
    protocol: 'vless',
    port: 443,
    tag: '',
    listen: '0.0.0.0',
  }
  inboundEditorOpen.value = true
}

function openInboundEdit(row: any) {
  inboundEditing.value = row
  inboundChange.value = null
  inboundEditorOpen.value = true
}

async function saveInbound() {
  if (!props.server) return
  const c = inboundChange.value
  if (!c) {
    ElMessage.warning('请先在表单中编辑参数')
    return
  }
  if (!c.tag.trim() || !c.port) {
    ElMessage.warning('请填写标签与端口')
    return
  }
  inboundSaving.value = true
  try {
    const payload: Partial<InboundPayload> = {
      server_id: props.server.id,
      tag: c.tag,
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing || undefined,
      ratio: inboundEditing.value?.ratio ?? 1,
    }
    const { data } = inboundEditing.value
      ? await updateInbound(inboundEditing.value.id, payload)
      : await createInbound(payload as InboundPayload)
    if (data.code === 0) {
      ElMessage.success(inboundEditing.value ? '入站已更新' : '入站已创建')
      inboundEditorOpen.value = false
      inboundEditing.value = null
      loadInbounds()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    inboundSaving.value = false
  }
}

async function toggleInboundRow(row: any) {
  try {
    const { data } = await toggleInbound(row.id)
    if (data.code === 0) {
      ElMessage.success(data.data.enabled ? '已启用' : '已停用')
      loadInbounds()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function removeInbound(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除入站「${row.tag}」？`, '删除入站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteInbound(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadInbounds()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ---- 出站 Tab ----
const outbounds = ref<ServerOutbound[]>([])
const outboundsLoading = ref(false)
const outboundEditorOpen = ref(false)
const outboundEditing = ref<ServerOutbound | null>(null)

async function loadOutbounds() {
  if (!props.server) return
  outboundsLoading.value = true
  try {
    const { data } = await getServerOutbounds(props.server.id)
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
  if (!props.server) return
  try {
    const { data } = await updateServerOutbound(props.server.id, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      ElMessage.success(data.data.outbound.enabled ? '已启用' : '已停用')
      loadOutbounds()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function removeOutbound(row: any) {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(`确认删除出站「${row.tag}」？`, '删除出站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteServerOutbound(props.server.id, row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadOutbounds()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ---- 路由 Tab ----
const routingRules = ref<ServerRoutingRule[]>([])
const routingLoading = ref(false)
const ruleEditorOpen = ref(false)
const ruleEditing = ref<ServerRoutingRule | null>(null)

async function loadRouting() {
  if (!props.server) return
  routingLoading.value = true
  try {
    const { data } = await getServerRoutingRules(props.server.id)
    if (data.code === 0) routingRules.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载路由规则失败'))
  } finally {
    routingLoading.value = false
  }
}

const outboundTags = computed(() => outbounds.value.map((o) => o.tag))

function openRuleCreate() {
  ruleEditing.value = null
  ruleEditorOpen.value = true
}

function openRuleEdit(row: any) {
  ruleEditing.value = row
  ruleEditorOpen.value = true
}

async function toggleRule(row: any) {
  if (!props.server) return
  try {
    const { data } = await updateServerRoutingRule(props.server.id, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      ElMessage.success(data.data.rule.enabled ? '已启用' : '已停用')
      loadRouting()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function removeRule(row: any) {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(`确认删除路由规则「${row.outbound_tag}」？`, '删除路由规则', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteServerRoutingRule(props.server.id, row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadRouting()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ---- 配置预览 Tab ----
const cfgLoading = ref(false)
const cfgText = ref('')
const cfgMessage = ref('')

async function generatePreview() {
  if (!props.server) return
  cfgLoading.value = true
  cfgText.value = ''
  cfgMessage.value = ''
  try {
    const { data } = await generateAndPushConfig(props.server.id)
    if (data.code === 0) {
      cfgText.value = data.data.config || ''
      cfgMessage.value = data.data.message || ''
      if (data.data.ok) ElMessage.success(data.data.message || '配置已生成')
      else ElMessage.warning(data.data.message || '配置已保存但未推送')
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    cfgMessage.value = `生成失败：${errMsg(e)}`
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    cfgLoading.value = false
  }
}

watch(
  () => [props.modelValue, props.server],
  () => {
    if (props.modelValue && props.server) {
      activeTab.value = 'overview'
      secretInfo.value = null
      cfgText.value = ''
      cfgMessage.value = ''
      loadInbounds()
      loadOutbounds()
      loadRouting()
    }
  },
  { immediate: true },
)
</script>

<template>
  <el-drawer
    :model-value="visible"
    :title="`节点管理 · ${server?.name ?? ''}`"
    size="78%"
    class="node-drawer"
    @update:model-value="(v: boolean) => (visible = v)"
  >
    <el-tabs v-model="activeTab" class="drawer-tabs">
      <!-- 概览 -->
      <el-tab-pane label="概览" name="overview">
        <template v-if="server">
          <div class="desc-grid">
            <div class="desc-row"><span class="k">名称</span><span class="v">{{ server.name }}</span></div>
            <div class="desc-row"><span class="k">地址</span><span class="v"><code class="cell-mono">{{ server.host }}</code></span></div>
            <div class="desc-row"><span class="k">Node ID</span><span class="v"><code class="cell-mono">{{ server.node_id }}</code>
              <el-button size="small" text @click="copyText(server.node_id, 'node_id')"><el-icon><CopyDocument /></el-icon></el-button>
            </span></div>
            <div class="desc-row"><span class="k">地区</span><span class="v">{{ server.location || '—' }}</span></div>
            <div class="desc-row"><span class="k">备注</span><span class="v">{{ server.remark || '—' }}</span></div>
            <div class="desc-row"><span class="k">状态</span><span class="v">
              <el-tag :type="server.status === 1 ? 'success' : 'info'" size="small">
                <span class="x-status-dot" :class="server.status === 1 ? 'online' : 'offline'" />{{ server.status === 1 ? '在线' : '离线' }}
              </el-tag>
            </span></div>
            <div class="desc-row"><span class="k">配置同步</span><span class="v">
              <el-tag v-if="server.config_status === 'pushed'" type="success" size="small">已同步</el-tag>
              <el-tag v-else-if="server.config_status === 'pending'" type="warning" size="small">待推送</el-tag>
              <el-tag v-else type="info" size="small" effect="plain">未生成</el-tag>
            </span></div>
            <div class="desc-row"><span class="k">最后心跳</span><span class="v">{{ fmtTime(server.last_seen_at) }}</span></div>
          </div>

          <el-divider />
          <div class="sec-title">节点密钥</div>
          <p class="muted tip">
            密钥仅在创建/重置时显示一次；重置后需更新节点 /etc/xray-agent/config.yml 中的 secret。
          </p>
          <div v-if="secretInfo" class="secret-box">
            <div class="secret-row"><span class="k">node_id</span><code>{{ secretInfo.node_id }}</code></div>
            <div class="secret-row">
              <span class="k">secret</span><code>{{ secretInfo.secret }}</code>
              <el-button size="small" text @click="copyText(secretInfo.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </div>
          <div class="action-row">
            <el-button :loading="resetting" @click="resetSecret"><el-icon><Key /></el-icon>&nbsp;重置密钥</el-button>
            <el-button type="danger" plain :loading="deleting" @click="removeServer"><el-icon><Delete /></el-icon>&nbsp;删除节点</el-button>
          </div>
        </template>
        <el-empty v-else description="未选择节点" />
      </el-tab-pane>

      <!-- 入站 -->
      <el-tab-pane label="入站" name="inbounds">
        <div class="tab-toolbar">
          <el-button size="small" @click="loadInbounds"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          <el-button size="small" type="primary" @click="openInboundCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
        </div>
        <el-table v-loading="inboundsLoading" :data="inbounds" size="small">
          <el-table-column prop="tag" label="标签" min-width="120">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
          </el-table-column>
          <el-table-column prop="protocol" label="协议" width="80" />
          <el-table-column label="端口" width="80">
            <template #default="{ row }"><code class="cell-mono">{{ row.port }}</code></template>
          </el-table-column>
          <el-table-column label="传输/TLS" width="120">
            <template #default="{ row }">
              <code class="cell-mono" style="font-size: 11px">{{ row.stream_settings ? JSON.parse(row.stream_settings).network + '/' + JSON.parse(row.stream_settings).security : '—' }}</code>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }"><el-switch :model-value="row.enabled" @change="toggleInboundRow(row)" /></template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text @click="openInboundEdit(row)"><el-icon><Edit /></el-icon></el-button>
              <el-button size="small" text type="danger" @click="removeInbound(row)"><el-icon><Delete /></el-icon></el-button>
            </template>
          </el-table-column>
          <template #empty><div class="table-empty">尚未配置入站，点击右上角「新增入站」</div></template>
        </el-table>
      </el-tab-pane>

      <!-- 出站 -->
      <el-tab-pane label="出站" name="outbounds">
        <div class="tab-toolbar">
          <el-button size="small" @click="loadOutbounds"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          <el-button size="small" type="primary" @click="openOutboundCreate"><el-icon><Plus /></el-icon>&nbsp;新增出站</el-button>
        </div>
        <el-table v-loading="outboundsLoading" :data="outbounds" size="small">
          <el-table-column prop="tag" label="标签" min-width="120">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
          </el-table-column>
          <el-table-column prop="protocol" label="协议" width="100" />
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

      <!-- 路由 -->
      <el-tab-pane label="路由" name="routing">
        <div class="tab-toolbar">
          <el-button size="small" @click="loadRouting"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          <el-button size="small" type="primary" @click="openRuleCreate"><el-icon><Plus /></el-icon>&nbsp;新增规则</el-button>
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
          <el-table-column prop="port" label="端口" width="80">
            <template #default="{ row }">{{ row.port || '—' }}</template>
          </el-table-column>
          <el-table-column prop="network" label="网络" width="70">
            <template #default="{ row }">{{ row.network || '—' }}</template>
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

      <!-- 配置预览 -->
      <el-tab-pane label="配置预览" name="config">
        <div class="tab-toolbar">
          <el-button size="small" type="primary" :loading="cfgLoading" @click="generatePreview">
            <el-icon><VideoPlay /></el-icon>&nbsp;生成并预览
          </el-button>
          <el-button
            size="small"
            :disabled="!cfgText"
            @click="copyText(cfgText, '配置 JSON')"
          >
            <el-icon><CopyDocument /></el-icon>&nbsp;复制配置
          </el-button>
        </div>
        <p v-if="cfgMessage" class="cfg-message">{{ cfgMessage }}</p>
        <p class="muted tip" style="margin: 0 0 8px">
          按该节点启用入站 + 出站 + 路由规则 + 全部启用用户生成完整 Xray 配置；生成即保存待推送（节点在线自动下发）。
        </p>
        <pre v-loading="cfgLoading" class="cfg-view">{{ cfgText || '点击「生成并预览」生成配置…' }}</pre>
      </el-tab-pane>
    </el-tabs>

    <!-- 入站编辑器（复用 3x-ui InboundConfigEditor） -->
    <el-dialog
      v-model="inboundEditorOpen"
      :title="inboundEditing ? '编辑入站' : '新增入站'"
      width="780px"
      :append-to-body="true"
      @closed="inboundEditing = null"
    >
      <InboundConfigEditor
        :key="inboundEditing ? `edit-${inboundEditing.id}` : 'create'"
        :model-value="inboundEditing?.settings_json ?? '{}'"
        @change="onInboundChange"
      />
      <template #footer>
        <el-button @click="inboundEditorOpen = false">取消</el-button>
        <el-button type="primary" :loading="inboundSaving" @click="saveInbound">保存</el-button>
      </template>
    </el-dialog>

    <!-- 出站编辑器 -->
    <OutboundConfigEditor
      v-if="outboundEditorOpen"
      :server-id="server?.id ?? 0"
      :outbound="outboundEditing"
      @saved="loadOutbounds"
      @close="outboundEditorOpen = false"
    />

    <!-- 路由编辑器 -->
    <RoutingRuleEditor
      v-if="ruleEditorOpen"
      :server-id="server?.id ?? 0"
      :rule="ruleEditing"
      :outbound-tags="outboundTags"
      @saved="loadRouting"
      @close="ruleEditorOpen = false"
    />
  </el-drawer>
</template>

<style scoped lang="scss">
.drawer-tabs {
  height: 100%;
}
.tab-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 12px;
}
.sec-title {
  font-weight: 600;
  font-size: 13px;
  margin: 4px 0 10px;
  color: var(--x-primary);
}
.muted {
  color: var(--x-text-3);
}
.tip {
  font-size: 12px;
}
.desc-grid {
  display: grid;
  gap: 0;
}
.desc-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid var(--x-border);
  font-size: 13.5px;
  .k {
    color: var(--x-text-2);
    flex: none;
  }
  .v {
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 4px;
  }
}
.cell-mono {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--x-text-2);
}
.secret-box {
  display: grid;
  gap: 10px;
  margin-bottom: 12px;
}
.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-primary-soft);
  border-radius: 8px;
  padding: 10px 12px;
  .k {
    color: var(--x-text-2);
    font-size: 12.5px;
    flex: none;
    width: 56px;
  }
  code {
    font-family: ui-monospace, Menlo, Consolas, monospace;
    font-size: 12.5px;
    color: var(--x-primary);
    word-break: break-all;
    flex: 1;
  }
}
.action-row {
  display: flex;
  gap: 10px;
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
.cfg-message {
  color: var(--x-text-2);
  font-size: 13px;
  margin: 0 0 8px;
}
.cfg-view {
  background: #171b2e;
  color: #c7d2fe;
  border-radius: 8px;
  padding: 14px;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 560px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
