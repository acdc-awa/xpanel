<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Edit, Delete, Loading, Connection, Promotion } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from './servers/InboundConfigEditor.vue'
import AccessPointDialog from './servers/AccessPointDialog.vue'
import {
  createInbound,
  deleteInbound,
  getInbounds,
  getPermissionGroups,
  getServers,
  toggleInbound,
  updateInbound,
  updateAccessPoint,
  deleteAccessPoint,
  getAccessPoints,
  type InboundItem,
  type InboundPayload,
  type PermissionGroup,
  type ServerItem,
  type UserAccessPoint,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatDateOnly } from '@/utils/timezone'

const route = useRoute()
const router = useRouter()

// 入站 / 用户接入点 双分页
const activeTab = ref<'inbounds' | 'access_points'>('inbounds')
const list = ref<InboundItem[]>([])
const servers = ref<ServerItem[]>([])
const loading = ref(false)
const serverFilter = ref<number | undefined>(undefined)
const typeFilter = ref<'' | 'user' | 'relay'>('')

// 本地类型过滤（列表数据已全量在本地；历史行 type 为空按用户入站口径处理）
const filteredInbounds = computed(() =>
  typeFilter.value ? list.value.filter((r) => (r.type || 'user') === typeFilter.value) : list.value,
)

function serverName(id: number) {
  return servers.value.find((s) => s.id === id)?.name ?? `#${id}`
}

async function loadServers() {
  try {
    const { data } = await getServers()
    if (data.code === 0) servers.value = data.data.items
  } catch {
    /* 忽略 */
  }
}

async function load() {
  loading.value = true
  try {
    const { data } = await getInbounds(serverFilter.value)
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载入站失败'))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadServers()])
  // 从服务器页跳转进入：?server_id=X 预选过滤
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  load()
  await Promise.all([loadAccessPoints(), loadPermissionGroups()])
  if (route.query.tab === 'access_points') activeTab.value = 'access_points'
})

watch(serverFilter, (v) => {
  router.replace({ query: v ? { server_id: v } : {} })
  load()
})

// ---- 用户接入点（Access Points）统一管理 ----
const apList = ref<UserAccessPoint[]>([])
const apLoading = ref(false)
const apGroups = ref<PermissionGroup[]>([])
const allInbounds = ref<InboundItem[]>([])

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

function apTargetDesc(ap: any): string {
  if (ap.target_type === 'inbound' && ap.target_inbound_tag) return `直连 ➜ ${ap.target_inbound_tag}`
  return '待连线'
}

async function loadAccessPoints() {
  apLoading.value = true
  try {
    const [res, inbRes] = await Promise.all([getAccessPoints(), getInbounds(undefined)])
    if (res.data.code === 0) apList.value = res.data.data.items
    else ElMessage.error(res.data.message)
    if (inbRes.data.code === 0) allInbounds.value = inbRes.data.data.items
  } catch (e) {
    ElMessage.error(errMsg(e, '加载接入点失败'))
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

async function removeAccessPoint(ap: any) {
  try {
    await ElMessageBox.confirm(`确认删除接入点「${ap.name}」？订阅将立即移除该入口。`, '删除接入点', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await deleteAccessPoint(ap.id)
    if (data.code === 0) {
      ElMessage.success('接入点已删除')
      loadAccessPoints()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
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
      ElMessage.success(ap.enabled ? '接入点已停用' : '接入点已启用')
      loadAccessPoints()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
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
  // 回填：InboundConfigEditor 只解析 settings_json + 顶层 network/tls_type/port/tag
  let network = 'tcp'
  let tlsType = 'reality'
  try {
    const ss = JSON.parse(editing.value.stream_settings || '{}')
    network = ss.network || network
    tlsType = ss.security || tlsType
  } catch {
    /* 保持默认 */
  }
  return JSON.stringify({
    tag: editing.value.tag,
    port: editing.value.port,
    listen: editing.value.listen,
    network,
    tls_type: tlsType,
    flow: editing.value.flow || '',
    settings_json: editing.value.settings_json,
    stream_settings: editing.value.stream_settings,
    sniffing: editing.value.sniffing,
    ratio: editing.value.ratio,
    total_gb: editing.value.total_gb ?? 0,
    expiry_time: editing.value.expiry_time ?? null,
    type: editing.value.type || 'user',
    cert_id: editing.value.cert_id || 0,
    share_addr_strategy: editing.value.share_addr_strategy,
    share_addr: editing.value.share_addr,
    share_port: editing.value.share_port,
    share_security: editing.value.share_security,
    share_sni: editing.value.share_sni,
    share_host: editing.value.share_host,
    share_path: editing.value.share_path,
    share_allow_insecure: editing.value.share_allow_insecure,
    layer_id: editing.value.layer_id || 0,
  })
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
    ElMessage.warning('请先在表单中编辑入站配置')
    return
  }
  if (!formServerId.value) {
    ElMessage.warning('请选择所属服务器')
    return
  }
  if (!c.tag.trim() || !c.port) {
    ElMessage.warning('请填写标签与端口')
    return
  }

  saving.value = true
  try {
    const payload: InboundPayload = {
      server_id: formServerId.value,
      tag: c.tag,
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing || undefined,
      ratio: c.ratio,
      total_gb: c.total_gb,
      expiry_time: c.expiry_time ?? null,
      type: formType.value,
      cert_id: formCertId.value || undefined,
      flow: c.flow || undefined,
      share_addr_strategy: c.shareAddrStrategy || undefined,
      share_addr: c.shareAddr || undefined,
      share_port: c.sharePort || undefined,
      share_security: c.shareSecurity || undefined,
      share_sni: c.shareSni || undefined,
      share_host: c.shareHost || undefined,
      share_path: c.sharePath || undefined,
      share_allow_insecure: c.shareAllowInsecure,
      layer_id: c.layerId || 0,
    }

    if (editing.value) {
      const { data } = await updateInbound(editing.value.id, payload)
      if (data.code === 0) {
        ElMessage.success('入站已保存')
        editorOpen.value = false
        load()
      } else {
        ElMessage.error(data.message)
      }
    } else {
      const { data } = await createInbound(payload)
      if (data.code === 0) {
        ElMessage.success('入站已创建')
        editorOpen.value = false
        load()
      } else {
        ElMessage.error(data.message)
      }
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- 启用 / 删除 ----
async function toggle(row: any) {
  try {
    const { data } = await toggleInbound(row.id)
    if (data.code === 0) {
      ElMessage.success(row.enabled ? '已停用' : '已启用')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除入站「${row.tag}」？`, '删除入站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteInbound(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(errMsg(data.message, '删除失败'))
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function transportOf(row: any): string {
  try {
    const ss = JSON.parse(row.stream_settings || '{}')
    return `${ss.network || '—'}/${ss.security || '—'}`
  } catch {
    return '—'
  }
}

// 流量/到期摘要：如 "100 GB · 09-30 到期" / "不限 · 永久" / "100 GB · 永久"
function quotaOf(row: any): string {
  const gb = typeof row.total_gb === 'number' && row.total_gb > 0 ? `${row.total_gb} GB` : '不限'
  const exp = row.expiry_time ? formatDateOnly(row.expiry_time) + ' 到期' : '永久'
  return `${gb} · ${exp}`
}
</script>

<template>
  <div class="x-page">
    <el-tabs v-model="activeTab" class="page-tabs">
      <el-tab-pane label="入站 (Inbounds)" name="inbounds">
        <div class="x-toolbar">
          <div class="x-toolbar-left">
            <el-select v-model="serverFilter" placeholder="全部 Xray 服务器" clearable style="width: 200px">
              <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
            </el-select>
            <el-select v-model="typeFilter" placeholder="全部类型" clearable style="width: 140px">
              <el-option label="用户入站" value="user" />
              <el-option label="转发入站" value="relay" />
            </el-select>
            <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          </div>
          <div style="display: flex; gap: 10px">
            <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
          </div>
        </div>

    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="在此为 Xray 服务器配置入站：用户入站承载用户真实流量，转发入站作为内部链式落地服务器。新增/编辑/停用后自动生成配置推送到服务器（离线保存，上线补推）。"
      style="margin-bottom: 14px"
    />

    <BaseCard title="入站列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="filteredInbounds.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Connection /></el-icon>
        <p style="margin-top: 8px">{{ list.length === 0 ? '尚未为任何服务器添加入站。点击右上角「新增入站」开始配置。' : '未找到匹配当前筛选的入站' }}</p>
      </div>

      <!-- 全局统一物理入站卡片网格流 (自适应 1~4 列) -->
      <div v-else class="node-card-grid">
        <div v-for="row in filteredInbounds" :key="row.id" class="node-card" :class="{ disabled: !row.enabled }">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
              <span class="node-name" title="点击编辑入站" @click="openEdit(row)">{{ row.tag }}</span>
              <span class="x-chip" :class="row.type === 'relay' ? 'orange' : 'purple'" style="font-size: 10px; padding: 1px 5px">
                {{ row.type === 'relay' ? '转发' : '用户' }}
              </span>
            </div>
            <el-tooltip :content="row.enabled ? '已启用，点击禁用' : '已禁用，点击启用'" placement="top">
              <el-switch :model-value="row.enabled" size="small" @change="toggle(row)" />
            </el-tooltip>
          </div>

          <!-- 属性网格 -->
          <div class="card-grid">
            <div class="grid-item">
              <span class="item-label">所属服务器</span>
              <div class="item-value" style="font-weight: 600">{{ serverName(row.server_id) }}</div>
            </div>
            <div class="grid-item">
              <span class="item-label">协议与端口</span>
              <div class="item-value">
                <span class="x-chip blue" style="text-transform: uppercase; font-size: 10.5px">{{ row.protocol }}</span>
                <code class="cell-mono font-12" style="font-weight: 600; margin-left: 4px">:{{ row.port }}</code>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">传输与 TLS</span>
              <div class="item-value cell-mono font-11">{{ transportOf(row) }}</div>
            </div>
            <div class="grid-item">
              <span class="item-label">流量与到期</span>
              <div class="item-value cell-mono muted font-11">{{ quotaOf(row) }}</div>
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-foot-actions">
            <el-button size="small" type="primary" plain @click="openEdit(row)">
              <el-icon><Edit /></el-icon>&nbsp;编辑入站
            </el-button>
            <el-button size="small" type="danger" plain @click="remove(row)">
              <el-icon><Delete /></el-icon>&nbsp;删除
            </el-button>
          </div>
        </div>
      </div>
    </BaseCard>
      </el-tab-pane>

      <!-- Tab 2: 用户接入点 (Access Points) -->
      <el-tab-pane label="接入点" name="access_points">
        <div class="x-toolbar">
          <div class="x-toolbar-left">
            <span class="muted" style="font-size: 13.5px">面向订阅分发的接入点定义</span>
            <el-button @click="loadAccessPoints"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          </div>
          <div style="display: flex; gap: 10px">
            <el-button type="primary" @click="openCreateAccessPoint"><el-icon><Plus /></el-icon>&nbsp;新建接入点</el-button>
          </div>
        </div>

        <el-alert
          type="info"
          :closable="false"
          show-icon
          title="接入点是用户订阅中实际看到的“节点”，通过白名单权限组控制哪些权限组可见。接入点可直接绑定入站，亦可在拓扑中参与链式转发中转。"
          style="margin-bottom: 14px"
        />

        <BaseCard title="接入点分发列表">
          <div v-if="apLoading" style="padding: 48px 0; text-align: center">
            <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
          </div>

          <div v-else-if="apList.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
            <el-icon style="font-size: 32px; color: var(--x-text-3)"><Promotion /></el-icon>
            <p style="margin-top: 8px">尚未创建任何接入点。新建后即可作为用户订阅的入口（订阅仅从接入点生成）。</p>
          </div>

          <!-- 全局统一用户接入点卡片网格流 (自适应 1~4 列) -->
          <div v-else class="node-card-grid">
            <div v-for="row in apList" :key="row.id" class="node-card" :class="{ disabled: !row.enabled }">
              <!-- 头部 -->
              <div class="card-head">
                <div class="head-title">
                  <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
                  <span class="node-name" title="点击编辑接入点" @click="openEditAccessPoint(row)">{{ row.name }}</span>
                </div>
                <el-tooltip :content="row.enabled ? '已启用，点击禁用' : '已禁用，点击启用'" placement="top">
                  <el-switch :model-value="row.enabled" size="small" @change="toggleAccessPoint(row)" />
                </el-tooltip>
              </div>

              <!-- 接入点属性网格 -->
              <div class="card-grid">
                <div class="grid-item">
                  <span class="item-label">目标绑定</span>
                  <div class="item-value">
                    <span v-if="apTargetDesc(row) === '待连线'" class="x-chip orange" style="font-size: 10.5px">{{ apTargetDesc(row) }}</span>
                    <span v-else class="x-chip purple" style="font-size: 10.5px">{{ apTargetDesc(row) }}</span>
                  </div>
                </div>
                <div class="grid-item">
                  <span class="item-label">连接地址覆写</span>
                  <div class="item-value cell-mono font-11">
                    {{ row.custom_host ? `${row.custom_host}:${row.custom_port || '自动'}` : '自动继承入站地址' }}
                  </div>
                </div>
                <div class="grid-item full-width">
                  <span class="item-label">开放权限组 (白名单)</span>
                  <div class="item-value">
                    <template v-if="row.permission_group_ids && row.permission_group_ids.length > 0">
                      <span
                        v-for="gid in row.permission_group_ids.slice(0, 3)"
                        :key="gid"
                        class="x-chip blue"
                        style="margin-right: 4px; font-size: 10px"
                      >{{ groupName(gid) }}</span>
                      <span v-if="row.permission_group_ids.length > 3" class="x-chip gray" style="font-size: 10px">+{{ row.permission_group_ids.length - 3 }}</span>
                    </template>
                    <span v-else class="x-chip orange" style="font-size: 10px">全员不可见</span>
                  </div>
                </div>
              </div>

              <!-- 底部操作栏 -->
              <div class="card-foot-actions">
                <el-button size="small" type="primary" plain @click="openEditAccessPoint(row)">
                  <el-icon><Edit /></el-icon>&nbsp;编辑接入点
                </el-button>
                <el-button size="small" type="danger" plain @click="removeAccessPoint(row)">
                  <el-icon><Delete /></el-icon>&nbsp;删除
                </el-button>
              </div>
            </div>
          </div>
        </BaseCard>

        <!-- 新建/编辑用户接入点 -->
        <AccessPointDialog
          v-model="apDialogOpen"
          :access-point="apEditingItem"
          :servers="servers"
          :inbounds="allInbounds"
          :permission-groups="apGroups"
          @saved="loadAccessPoints"
          @deleted="loadAccessPoints"
        />
      </el-tab-pane>
    </el-tabs>

    <!-- 新增/编辑入站 -->
    <el-dialog
      v-model="editorOpen"
      :title="editing ? `编辑入站 · ${editing.tag}` : '新建入站'"
      width="640px"
      :append-to-body="true"
      @closed="editing = null"
    >
      <div v-if="!editing" style="margin-bottom: 14px">
        <el-form-item label="所属目标服务器" style="margin-bottom: 0">
          <el-select v-model="formServerId" style="width: 100%" placeholder="请选择要绑定入站的服务器">
            <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
          </el-select>
        </el-form-item>
      </div>

      <InboundConfigEditor
        :key="editing ? `edit-${editing.id}` : 'create'"
        :model-value="editorModelValue()"
        :inbound-type="formType"
        :saved-inbound-type="editing?.type || ''"
        :listen="editing?.listen || '0.0.0.0'"
        :internal-uuid="editing?.internal_uuid || ''"
        :inbound-id="editing?.id || 0"
        :cert-id="formCertId"
        :server-id="formServerId"
        :layer-id="editing?.layer_id || 0"
        @change="onInboundEditorChange"
        @update:inbound-type="(v: string) => (formType = v)"
        @update:cert-id="(v: number) => (formCertId = v || 0)"
        @internal-uuid-changed="load"
      />
      <template #footer>
        <el-button @click="editorOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存入站</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--x-text-2);
}
.muted {
  color: var(--x-text-3);
}

/* ================= 全局统一入站/接入点卡片网格流 ================= */
.node-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.node-card {
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

    .node-name {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text, #111827);
      cursor: pointer;
      &:hover {
        color: var(--x-primary);
      }
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

.cfg-view {
  background: #171b2e;
  color: #c7d2fe;
  border-radius: 8px;
  padding: 14px;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 480px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

@media (max-width: 640px) {
  .node-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
