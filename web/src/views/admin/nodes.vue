<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Search, Edit, Delete, Refresh, VideoPlay } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  createInbound,
  deleteInbound,
  generateAndPushConfig,
  getInbounds,
  getServers,
  toggleInbound,
  updateInbound,
  type InboundItem,
  type ServerItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<InboundItem[]>([])
const servers = ref<ServerItem[]>([])
const loading = ref(false)
const serverFilter = ref<number | undefined>(undefined)
const keyword = ref('')

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
onMounted(() => {
  loadServers()
  load()
})

function serverName(id: number) {
  return servers.value.find((s) => s.id === id)?.name ?? `#${id}`
}

// ---- 新增/编辑 ----
const formOpen = ref(false)
const editing = ref(false)
const form = reactive({
  id: 0,
  server_id: 0,
  tag: '',
  protocol: 'vless',
  port: 443,
  network: 'tcp',
  tls_type: 'reality',
  settings_json: '',
  ratio: 1,
})
const saving = ref(false)

const settingsTemplate = `{
  "reality": {
    "server_name": "www.apple.com",
    "public_key": "<客户端公钥>",
    "short_id": "abcdef0123456789",
    "private_key": "<服务端私钥>",
    "dest": "www.apple.com:443"
  },
  "ws": { "path": "/", "host": "" },
  "xhttp": { "mode": "auto", "path": "/" },
  "tls": { "server_name": "", "cert_file": "/path/cert.pem", "key_file": "/path/key.pem" }
}`

function openCreate() {
  editing.value = false
  Object.assign(form, {
    id: 0,
    server_id: servers.value[0]?.id ?? 0,
    tag: '',
    protocol: 'vless',
    port: 443,
    network: 'tcp',
    tls_type: 'reality',
    settings_json: settingsTemplate,
    ratio: 1,
  })
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = true
  Object.assign(form, {
    id: row.id,
    server_id: row.server_id,
    tag: row.tag,
    protocol: row.protocol,
    port: row.port,
    network: row.network,
    tls_type: row.tls_type,
    settings_json: row.settings_json || settingsTemplate,
    ratio: row.ratio,
  })
  formOpen.value = true
}

async function save() {
  if (!form.tag || !form.port || !form.server_id) {
    ElMessage.warning('请填写服务器、标签与端口')
    return
  }
  saving.value = true
  try {
    const payload = {
      server_id: form.server_id,
      tag: form.tag,
      protocol: form.protocol,
      port: form.port,
      network: form.network,
      tls_type: form.tls_type,
      settings_json: form.settings_json,
      ratio: form.ratio,
    }
    const { data } = editing.value ? await updateInbound(form.id, payload) : await createInbound(payload)
    if (data.code === 0) {
      ElMessage.success(editing.value ? '已保存' : '已创建')
      formOpen.value = false
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

async function toggle(row: any) {
  try {
    const { data } = await toggleInbound(row.id)
    if (data.code === 0) {
      ElMessage.success(data.data.enabled ? '已启用' : '已停用')
      load()
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

// ---- 生成配置并下发 ----
const genOpen = ref(false)
const genResult = ref('')
const genLoading = ref(false)

async function genPush() {
  if (!serverFilter.value) {
    ElMessage.warning('请先在筛选器选择服务器')
    return
  }
  try {
    await ElMessageBox.confirm(
      `将按「${serverName(serverFilter.value)}」的启用入站 + 全部启用用户生成配置并下发，确认？`,
      '生成并下发配置',
      { type: 'warning' },
    )
  } catch {
    return
  }
  genLoading.value = true
  genResult.value = ''
  genOpen.value = true
  try {
    const { data } = await generateAndPushConfig(serverFilter.value)
    if (data.code === 0 && data.data.ok) {
      ElMessage.success('配置生成并下发成功')
      genResult.value = data.data.config
    } else {
      ElMessage.error(data.data?.error || data.message)
      genResult.value = data.data?.config ?? ''
    }
  } catch (e) {
    genResult.value = `失败：${errMsg(e)}`
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    genLoading.value = false
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-select v-model="serverFilter" placeholder="全部服务器" clearable style="width: 180px" @change="load">
          <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
        <el-input v-model="keyword" placeholder="搜索标签 / 端口" :prefix-icon="Search" clearable style="width: 200px" />
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button @click="genPush"><el-icon><VideoPlay /></el-icon>&nbsp;生成并下发配置</el-button>
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon title="P3 当前仅支持 VLESS 协议（tcp+reality+vision / ws+tls / xhttp+reality）；新增后需在「服务器」页生成并下发配置生效" style="margin-bottom: 14px" />

    <BaseCard>
      <el-table v-loading="loading" :data="list">
        <el-table-column prop="id" label="ID" width="60">
          <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
        </el-table-column>
        <el-table-column label="服务器" min-width="110">
          <template #default="{ row }">{{ serverName(row.server_id) }}</template>
        </el-table-column>
        <el-table-column prop="tag" label="标签" min-width="120">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="80" />
        <el-table-column label="端口" width="80">
          <template #default="{ row }"><code class="cell-mono">{{ row.port }}</code></template>
        </el-table-column>
        <el-table-column prop="network" label="传输" width="80" />
        <el-table-column prop="tls_type" label="TLS" width="90" />
        <el-table-column label="倍率" width="70">
          <template #default="{ row }">{{ row.ratio }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text @click="openEdit(row)"><el-icon><Edit /></el-icon></el-button>
            <el-button size="small" text type="danger" @click="remove(row)"><el-icon><Delete /></el-icon></el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div style="padding: 30px 0; color: var(--x-text-3)">
            尚未配置入站。点击右上角「新增入站」。
          </div>
        </template>
      </el-table>
    </BaseCard>

    <!-- 新增/编辑 -->
    <el-dialog v-model="formOpen" :title="editing ? '编辑入站' : '新增入站'" width="620px">
      <el-form label-position="top">
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="所属服务器">
            <el-select v-model="form.server_id" style="width: 100%">
              <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="form.tag" placeholder="如 Tokyo-VLESS" />
          </el-form-item>
          <el-form-item label="协议">
            <el-select v-model="form.protocol" style="width: 100%" disabled>
              <el-option label="vless（P3 仅支持）" value="vless" />
            </el-select>
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
          <el-form-item label="传输层">
            <el-select v-model="form.network" style="width: 100%">
              <el-option label="tcp" value="tcp" />
              <el-option label="ws" value="ws" />
              <el-option label="xhttp" value="xhttp" />
            </el-select>
          </el-form-item>
          <el-form-item label="TLS">
            <el-select v-model="form.tls_type" style="width: 100%">
              <el-option label="reality" value="reality" />
              <el-option label="tls" value="tls" />
              <el-option label="none" value="none" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="倍率">
          <el-input-number v-model="form.ratio" :min="0.1" :max="10" :step="0.1" style="width: 160px" />
        </el-form-item>
        <el-form-item label="连接参数（settings_json）">
          <el-input v-model="form.settings_json" type="textarea" :rows="9" class="mono-area" />
          <p class="muted" style="font-size: 12px; margin: 6px 0 0">
            reality 需要 server_name / public_key（客户端用）/ short_id / private_key（服务端，不下发订阅）/ dest
          </p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 生成结果 -->
    <el-dialog v-model="genOpen" title="生成并下发配置" width="640px">
      <pre v-loading="genLoading" class="cfg-view">{{ genResult || '正在生成并下发…' }}</pre>
      <template #footer>
        <el-button type="primary" @click="genOpen = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.mono-area :deep(textarea) { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; }
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