<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Edit, Delete, VideoPlay } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from './servers/InboundConfigEditor.vue'
import {
  createInbound,
  deleteInbound,
  generateAndPushConfig,
  getInbounds,
  getServers,
  toggleInbound,
  updateInbound,
  type InboundItem,
  type InboundPayload,
  type ServerItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const route = useRoute()
const router = useRouter()

const list = ref<InboundItem[]>([])
const servers = ref<ServerItem[]>([])
const loading = ref(false)
const serverFilter = ref<number | undefined>(undefined)

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
  await loadServers()
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
    flow: editing.value.flow || '',
    ratio: editing.value.ratio ?? 1,
    share_addr_strategy: editing.value.share_addr_strategy || 'node',
    share_addr: editing.value.share_addr || '',
    share_port: editing.value.share_port || 0,
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

// ---- 生成并下发配置（按当前筛选服务器） ----
const deployOpen = ref(false)
const deployLoading = ref(false)
const deployResult = ref('')

async function deployFor(serverId: number, serverLabel: string) {
  try {
    await ElMessageBox.confirm(
      `将按「${serverLabel}」的全部启用入站 + 出站 + 路由 + 用户生成配置并自动推送（节点离线时保存，上线自动补推），确认？`,
      '生成并下发配置',
      { type: 'warning' },
    )
  } catch {
    return
  }
  deployOpen.value = true
  deployLoading.value = true
  deployResult.value = ''
  try {
    const { data } = await generateAndPushConfig(serverId)
    if (data.code === 0 && data.data.ok) {
      ElMessage.success(data.data.message || '部署成功')
      deployResult.value = data.data.config
    } else {
      ElMessage.error(data.data?.message || data.message)
      deployResult.value = data.data?.config ?? ''
    }
  } catch (e) {
    deployResult.value = `失败：${errMsg(e)}`
    ElMessage.error(errMsg(e, '部署失败'))
  } finally {
    deployLoading.value = false
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
        <el-button
          :disabled="!serverFilter"
          @click="deployFor(serverFilter!, serverName(serverFilter!))"
        >
          <el-icon><VideoPlay /></el-icon>&nbsp;生成并下发配置
        </el-button>
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon title="在此为服务器添加接入点（入站）：用户入站进订阅、转发入站作为内部落地（需执行「生成内部 UUID」）、闲置仅预留。新增/编辑/停用后自动生成配置推送到节点（离线保存，上线补推）。" style="margin-bottom: 14px" />

    <BaseCard>
      <el-table v-loading="loading" :data="list">
        <el-table-column prop="id" label="ID" width="64">
          <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
        </el-table-column>
        <el-table-column label="服务器" min-width="110">
          <template #default="{ row }">{{ serverName(row.server_id) }}</template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.type === 'relay'" size="small" type="warning">转发</el-tag>
            <el-tag v-else-if="row.type === 'idle'" size="small" type="info">闲置</el-tag>
            <el-tag v-else size="small" type="success">用户</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="tag" label="标签" min-width="130">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="80" />
        <el-table-column label="端口" width="80">
          <template #default="{ row }"><code class="cell-mono">{{ row.port }}</code></template>
        </el-table-column>
        <el-table-column label="传输/TLS" width="130">
          <template #default="{ row }"><code class="cell-mono" style="font-size: 11px">{{ transportOf(row) }}</code></template>
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
    </BaseCard>

    <!-- 新增/编辑入站 -->
    <el-dialog
      v-model="editorOpen"
      :title="editing ? `编辑入站 · ${editing.tag}` : '新增入站'"
      width="780px"
      :append-to-body="true"
      @closed="editing = null"
    >
      <el-form-item label="所属服务器" style="margin-bottom: 14px">
        <el-select v-model="formServerId" style="width: 280px" :disabled="!!editing">
          <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
        <span v-if="editing" class="muted" style="font-size: 12px; margin-left: 8px">（编辑时不可更换服务器）</span>
      </el-form-item>

      <InboundConfigEditor
        :key="editing ? `edit-${editing.id}` : 'create'"
        :model-value="editorModelValue()"
        :inbound-type="formType"
        :internal-uuid="editing?.internal_uuid || ''"
        :inbound-id="editing?.id || 0"
        :cert-id="formCertId"
        @change="onInboundChange"
        @update:inbound-type="(v: string) => (formType = v)"
        @update:cert-id="(v: number) => (formCertId = v || 0)"
        @internal-uuid-changed="load"
      />
      <template #footer>
        <el-button @click="editorOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 部署结果 -->
    <el-dialog v-model="deployOpen" title="生成并下发配置" width="640px">
      <pre v-loading="deployLoading" class="cfg-view">{{ deployResult || '正在生成并下发…' }}</pre>
      <template #footer>
        <el-button type="primary" @click="deployOpen = false">关闭</el-button>
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
