<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Edit, Delete, VideoPlay } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from './servers/InboundConfigEditor.vue'
import {
  createInbound,
  deleteInbound,
  getInbounds,
  getPermissionGroups,
  getServers,
  toggleInbound,
  updateInbound,
  type InboundItem,
  type InboundPayload,
  type PermissionGroup,
  type ServerItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const route = useRoute()
const router = useRouter()

const list = ref<InboundItem[]>([])
const servers = ref<ServerItem[]>([])
const groups = ref<PermissionGroup[]>([])
const loading = ref(false)
const serverFilter = ref<number | undefined>(undefined)

function serverName(id: number) {
  return servers.value.find((s) => s.id === id)?.name ?? `#${id}`
}

function groupName(id: number) {
  return groups.value.find((g) => g.id === id)?.name ?? `组#${id}`
}

async function loadServers() {
  try {
    const { data } = await getServers()
    if (data.code === 0) servers.value = data.data.items
  } catch {
    /* 忽略 */
  }
}

async function loadGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) groups.value = data.data.items
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
  await Promise.all([loadServers(), loadGroups()])
  // 从服务器页跳转进入：?server_id=X 预选过滤
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  load()
})

watch(serverFilter, (v) => {
  router.replace({ query: v ? { server_id: v } : {} })
  load()
})

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
    settings_json: editing.value.settings_json,
    network,
    tls_type: tlsType,
    protocol: editing.value.protocol,
    port: editing.value.port,
    tag: editing.value.tag,
    listen: editing.value.listen || '0.0.0.0',
    flow: editing.value.flow || '',
    ratio: editing.value.ratio ?? 1,
    share_addr_strategy: editing.value.share_addr_strategy || 'node',
    share_addr: editing.value.share_addr || '',
    share_port: editing.value.share_port || 0,
    permission_group_ids: editing.value.permission_group_ids || [],
  })
}

function openCreate() {
  editing.value = null
  inboundChange.value = null
  formServerId.value = serverFilter.value ?? servers.value[0]?.id ?? 0
  formType.value = 'user'
  formCertId.value = 0
  editorOpen.value = true
}

function openEdit(row: any) {
  editing.value = row
  inboundChange.value = null
  formServerId.value = row.server_id
  formType.value = row.type || 'user'
  formCertId.value = row.cert_id || 0
  editorOpen.value = true
}

function onInboundChange(payload: InboundEditorChangePayload) {
  inboundChange.value = payload
}

async function save() {
  const c = inboundChange.value
  if (!c) {
    ElMessage.warning('请先在表单中编辑参数')
    return
  }
  if (!c.tag.trim() || !c.port) {
    ElMessage.warning('请填写标签与端口')
    return
  }
  if (!formServerId.value) {
    ElMessage.warning('请选择所属服务器')
    return
  }
  saving.value = true
  try {
    const payload: Partial<InboundPayload> = {
      server_id: formServerId.value,
      tag: c.tag,
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing || undefined,
      ratio: c.ratio,
      type: formType.value,
      cert_id: formCertId.value || undefined,
      flow: c.flow || undefined,
      share_addr_strategy: c.shareAddrStrategy || undefined,
      share_addr: c.shareAddr || undefined,
      share_port: c.sharePort || undefined,
      permission_group_ids: c.permissionGroupIds,
    }
    const { data } = editing.value
      ? await updateInbound(editing.value.id, payload)
      : await createInbound(payload as InboundPayload)
    if (data.code === 0) {
      ElMessage.success(editing.value ? '入站已更新（将自动推送到节点）' : '入站已创建（将自动推送到节点）')
      editorOpen.value = false
      editing.value = null
      load()
    } else {
      ElMessage.error(data.message)
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
      ElMessage.success(data.data.enabled ? '已启用' : '已停用')
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
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-select v-model="serverFilter" placeholder="全部服务器" clearable style="width: 200px">
          <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon title="在此为服务器添加接入点（入站）：用户入站进订阅、转发入站作为内部落地（需执行「生成内部 UUID」）、闲置仅预留。新增/编辑/停用后自动生成配置推送到节点（离线保存，上线补推）。" style="margin-bottom: 14px" />

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column prop="id" label="ID" width="64">
            <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
          </el-table-column>
          <el-table-column label="服务器" min-width="110">
            <template #default="{ row }">{{ serverName(row.server_id) }}</template>
          </el-table-column>
          <el-table-column label="类型" width="90">
            <template #default="{ row }">
              <span v-if="row.type === 'relay'" class="x-chip orange">转发</span>
              <span v-else-if="row.type === 'idle'" class="x-chip gray">闲置</span>
              <span v-else class="x-chip purple">用户</span>
            </template>
          </el-table-column>
          <el-table-column prop="tag" label="标签" min-width="130">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
          </el-table-column>
          <el-table-column prop="protocol" label="协议" width="80">
            <template #default="{ row }">
              <span class="x-chip blue" style="text-transform: uppercase">{{ row.protocol }}</span>
            </template>
          </el-table-column>
          <el-table-column label="端口" width="80">
            <template #default="{ row }"><code class="cell-mono">{{ row.port }}</code></template>
          </el-table-column>
          <el-table-column label="传输/TLS" width="130">
            <template #default="{ row }"><code class="cell-mono" style="font-size: 11px">{{ transportOf(row) }}</code></template>
          </el-table-column>
          <el-table-column label="开放权限组" min-width="150">
            <template #default="{ row }">
              <template v-if="row.type === 'relay' || row.type === 'idle'">
                <span class="muted" style="font-size: 12px">内部/落地</span>
              </template>
              <template v-else-if="row.permission_group_ids && row.permission_group_ids.length">
                <span
                  v-for="gid in row.permission_group_ids"
                  :key="gid"
                  class="x-chip blue"
                  style="margin-right: 4px; margin-bottom: 2px"
                >
                  {{ groupName(gid) }}
                </span>
              </template>
              <span v-else class="x-chip orange" style="font-size: 11px">
                未分配 (不对外开放)
              </span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-switch :model-value="row.enabled" @change="toggle(row)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text @click="openEdit(row)"><el-icon><Edit /></el-icon></el-button>
              <el-button size="small" text type="danger" @click="remove(row)"><el-icon><Delete /></el-icon></el-button>
            </template>
          </el-table-column>
          <template #empty>
            <div style="padding: 30px 0; color: var(--x-text-3)">
              {{ serverFilter ? '该服务器尚未配置接入点，点击右上角「新增入站」' : '尚未配置任何接入点。先到「服务器」页添加服务器，再点击右上角「新增入站」' }}
            </div>
          </template>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          {{ serverFilter ? '该服务器尚未配置接入点，点击右上角「新增入站」' : '尚未配置任何接入点，点击右上角「新增入站」' }}
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
                <span style="font-weight: 700">{{ row.tag }}</span>
                <span v-if="row.type === 'relay'" class="x-chip orange">转发</span>
                <span v-else-if="row.type === 'idle'" class="x-chip gray">闲置</span>
                <span v-else class="x-chip purple">用户</span>
              </div>
              <el-switch :model-value="row.enabled" size="small" @change="toggle(row)" />
            </div>

            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">所属服务器</span>
                <div class="item-value" style="font-weight: 600">{{ serverName(row.server_id) }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">协议 / 端口</span>
                <div class="item-value">
                  <span style="text-transform: uppercase; font-weight: 600">{{ row.protocol }}</span>
                  <code class="cell-mono" style="margin-left: 4px">:{{ row.port }}</code>
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">传输与 TLS</span>
                <div class="item-value cell-mono" style="font-size: 11.5px">{{ transportOf(row) }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">开放权限组</span>
                <div class="item-value">
                  <template v-if="row.type === 'relay' || row.type === 'idle'">
                    <span class="muted" style="font-size: 11.5px">内部/落地</span>
                  </template>
                  <template v-else-if="row.permission_group_ids && row.permission_group_ids.length">
                    <el-tag
                      v-for="gid in row.permission_group_ids"
                      :key="gid"
                      size="small"
                      effect="plain"
                      style="margin-right: 4px; margin-bottom: 2px"
                    >
                      {{ groupName(gid) }}
                    </el-tag>
                  </template>
                  <el-tag v-else size="small" type="warning" effect="plain" style="font-size: 11px">
                    未分配 (不对外开放)
                  </el-tag>
                </div>
              </div>
            </div>

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
      </div>
    </BaseCard>

    <!-- 新增/编辑入站 -->
    <el-dialog
      v-model="editorOpen"
      :title="editing ? `编辑入站 · ${editing.tag}` : '新增节点入站'"
      width="720px"
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
        :listen="editing?.listen || '0.0.0.0'"
        :internal-uuid="editing?.internal_uuid || ''"
        :inbound-id="editing?.id || 0"
        :cert-id="formCertId"
        :permission-group-ids="editing?.permission_group_ids || []"
        @change="onInboundChange"
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
</style>
