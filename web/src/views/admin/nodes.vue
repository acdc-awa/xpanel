<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Plus, Search, Edit, Delete, Refresh, VideoPlay, MagicStick } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  createInbound,
  deleteInbound,
  generateAndPushConfig,
  getInbounds,
  getServers,
  getXrayKeys,
  previewConfig,
  toggleInbound,
  updateInbound,
  type InboundItem,
  type InboundSettings,
  type ServerItem,
} from '@/api/admin'
import type { FallbackItem } from '@/api/types'
import { errMsg } from '@/api/http'

const list = ref<InboundItem[]>([])
const servers = ref<ServerItem[]>([])
const loading = ref(false)
const serverFilter = ref<number | undefined>(undefined)

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

// ---- 表单（动态参数区） ----
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
  ratio: 1,
  // Fallbacks
  fallbacks: [] as FallbackItem[],
  // 动态参数
  reality: { server_name: '', dest: '', public_key: '', private_key: '', short_id: 'abcdef0123456789' },
  ws: { path: '/', host: '' },
  xhttp: { mode: 'auto', path: '/' },
  grpc: { service_name: 'grpc', authority: '', multi_mode: false },
  tls: { server_name: '', cert_file: '', key_file: '' },
  sniffing: { enabled: false, destOverride: ['http', 'tls', 'quic', 'fakedns'] as string[], metadataOnly: false, routeOnly: false },
})
const tlsAlpnText = ref('h2,http/1.1')
const saving = ref(false)
const genKeyLoading = ref(false)

const settingsTip = computed(() => {
  if (form.tls_type === 'reality') return 'REALITY：借壳目标需可达且 TLS1.3 稳定（如 www.apple.com:443）'
  if (form.tls_type === 'tls') return 'TLS：cert_file/key_file 为节点服务器上的证书路径'
  return '无 TLS'
})

function parseSettings(sj: string): InboundSettings | null {
  try {
    return JSON.parse(sj || '{}')
  } catch {
    return null
  }
}

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
    ratio: 1,
    fallbacks: [],
    reality: { server_name: '', dest: '', public_key: '', private_key: '', short_id: 'abcdef0123456789' },
    ws: { path: '/', host: '' },
    xhttp: { mode: 'auto', path: '/' },
    grpc: { service_name: 'grpc', authority: '', multi_mode: false },
    tls: { server_name: '', cert_file: '', key_file: '' },
    sniffing: { enabled: false, destOverride: ['http', 'tls', 'quic', 'fakedns'], metadataOnly: false, routeOnly: false },
  })
  tlsAlpnText.value = 'h2,http/1.1'
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = true
  const s = parseSettings(row.settings_json)
  Object.assign(form, {
    id: row.id,
    server_id: row.server_id,
    tag: row.tag,
    protocol: row.protocol,
    port: row.port,
    network: row.network,
    tls_type: row.tls_type,
    ratio: row.ratio,
    fallbacks: Array.isArray(s?.fallbacks)
      ? s!.fallbacks!.map((f) => ({ name: f.name || '', alpn: f.alpn || '', path: f.path || '', dest: f.dest || '', xver: f.xver || 0 }))
      : [],
    reality: {
      server_name: s?.reality?.server_name ?? '',
      dest: s?.reality?.dest ?? '',
      public_key: s?.reality?.public_key ?? '',
      private_key: s?.reality?.private_key ?? '',
      short_id: s?.reality?.short_id ?? 'abcdef0123456789',
    },
    ws: { path: s?.ws?.path ?? '/', host: s?.ws?.host ?? '' },
    xhttp: { mode: s?.xhttp?.mode ?? 'auto', path: s?.xhttp?.path ?? '/' },
    grpc: {
      service_name: s?.grpc?.service_name ?? 'grpc',
      authority: s?.grpc?.authority ?? '',
      multi_mode: !!s?.grpc?.multi_mode,
    },
    tls: { server_name: s?.tls?.server_name ?? '', cert_file: s?.tls?.cert_file ?? '', key_file: s?.tls?.key_file ?? '' },
    sniffing: {
      enabled: !!s?.sniffing?.enabled,
      destOverride: Array.isArray(s?.sniffing?.destOverride)
        ? s!.sniffing!.destOverride!
        : Array.isArray(s?.sniffing?.dest_override)
          ? s!.sniffing!.dest_override!
          : ['http', 'tls', 'quic', 'fakedns'],
      metadataOnly: !!s?.sniffing?.metadataOnly || !!s?.sniffing?.metadata_only,
      routeOnly: !!s?.sniffing?.routeOnly || !!s?.sniffing?.route_only,
    },
  })
  tlsAlpnText.value = Array.isArray(s?.tls?.alpn) && s!.tls!.alpn!.length > 0 ? s!.tls!.alpn!.join(',') : 'h2,http/1.1'
  formOpen.value = true
}

// ---- Fallbacks 操作 ----
function addFallback() {
  form.fallbacks.push({ dest: '80', path: '', xver: 0 })
}

function removeFallback(index: number) {
  form.fallbacks.splice(index, 1)
}

// 生成 REALITY 密钥对
async function genRealityKeys() {
  genKeyLoading.value = true
  try {
    const { data } = await getXrayKeys()
    if (data.code === 0) {
      form.reality.private_key = data.data.private_key
      form.reality.public_key = data.data.public_key
      ElMessage.success('已生成密钥对')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    genKeyLoading.value = false
  }
}

// 组装 settings 对象（仅包含当前生效的参数）
function buildSettings(): InboundSettings {
  const s: InboundSettings = {}
  if (form.fallbacks.length > 0) {
    s.fallbacks = form.fallbacks.map((f) => ({
      dest: f.dest,
      path: f.path || undefined,
      xver: f.xver || 0,
    }))
  }
  if (form.tls_type === 'reality') s.reality = { ...form.reality }
  if (form.tls_type === 'tls') {
    const alpnArr = tlsAlpnText.value
      ? tlsAlpnText.value.split(',').map((x) => x.trim()).filter(Boolean)
      : []
    s.tls = { ...form.tls, alpn: alpnArr.length > 0 ? alpnArr : undefined }
  }
  if (form.network === 'ws') s.ws = { ...form.ws }
  if (form.network === 'xhttp') s.xhttp = { ...form.xhttp }
  if (form.network === 'grpc') s.grpc = { ...form.grpc }
  if (form.sniffing.enabled) {
    s.sniffing = {
      enabled: true,
      destOverride: form.sniffing.destOverride,
      metadataOnly: form.sniffing.metadataOnly,
      routeOnly: form.sniffing.routeOnly,
    }
  }
  return s
}

// ---- 原始 Xray Config 预览（防抖） ----
const previewOpen = ref(false)
const previewLoading = ref(false)
const previewConfigText = ref('')

function buildPreviewPayload() {
  return {
    id: form.id,
    server_id: form.server_id,
    tag: form.tag,
    protocol: form.protocol,
    port: form.port,
    network: form.network,
    tls_type: form.tls_type,
    settings: buildSettings(),
    ratio: form.ratio,
  }
}

let previewTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => ({ ...form }),
  () => {
    if (!previewOpen.value) return
    if (previewTimer) clearTimeout(previewTimer)
    previewTimer = setTimeout(async () => {
      if (!form.server_id || !form.port || !form.tag) {
        previewConfigText.value = '请先填写服务器、标签与端口'
        return
      }
      previewLoading.value = true
      try {
        const { data } = await previewConfig(form.server_id, buildPreviewPayload())
        if (data.code === 0) previewConfigText.value = data.data.config
        else previewConfigText.value = data.message
      } catch (e) {
        previewConfigText.value = `预览失败：${errMsg(e)}`
      } finally {
        previewLoading.value = false
      }
    }, 500)
  },
  { deep: true },
)

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
      settings: buildSettings(),
      ratio: form.ratio,
    }
    const { data } = editing.value ? await updateInbound(form.id, payload) : await createInbound(payload)
    if (data.code === 0) {
      ElMessage.success(editing.value ? '已保存（将自动推送到节点）' : '已创建（将自动推送到节点）')
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

// ---- 一键部署：生成该服务器配置并下发 ----
const deployOpen = ref(false)
const deployLoading = ref(false)
const deployResult = ref('')

async function deploy(row: any) {
  try {
    await ElMessageBox.confirm(
      `将按「${serverName(row.server_id)}」的全部启用入站 + 用户生成配置并自动推送（节点离线时保存，上线自动补推），确认？`,
      '一键部署',
      { type: 'warning' },
    )
  } catch {
    return
  }
  deployOpen.value = true
  deployLoading.value = true
  deployResult.value = ''
  try {
    const { data } = await generateAndPushConfig(row.server_id)
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

// 顶部"生成并下发"（按当前筛选服务器）
async function deployFiltered() {
  if (!serverFilter.value) {
    ElMessage.warning('请先筛选服务器')
    return
  }
  const fake: InboundItem = {
    id: 0,
    server_id: serverFilter.value,
    server_name: serverName(serverFilter.value),
    tag: '',
    protocol: 'vless',
    port: 0,
    network: '',
    tls_type: '',
    settings_json: '',
    ratio: 1,
    enabled: true,
    created_at: '',
  }
  await deploy(fake)
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-select v-model="serverFilter" placeholder="全部服务器" clearable style="width: 180px" @change="load">
          <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button @click="deployFiltered"><el-icon><VideoPlay /></el-icon>&nbsp;生成并下发配置</el-button>
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon title="新增/编辑/停用入站后将自动生成配置并推送到节点（离线时保存，上线自动补推）；也可点操作列「部署」立即生成并下发" style="margin-bottom: 14px" />

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
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="190" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="success" plain @click="deploy(row)"><el-icon><VideoPlay /></el-icon>&nbsp;部署</el-button>
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
    <el-dialog v-model="formOpen" :title="editing ? '编辑入站' : '新增入站'" width="680px">
      <div class="dialog-head">
        <el-button size="small" :type="!previewOpen ? 'primary' : 'default'" @click="previewOpen = false">参数配置</el-button>
        <el-button size="small" :type="previewOpen ? 'primary' : 'default'" @click="previewOpen = true">原始 Xray Config</el-button>
      </div>
      <el-form v-if="!previewOpen" label-position="top">
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
              <el-option label="vless（当前支持）" value="vless" />
            </el-select>
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
          <el-form-item label="传输层">
            <el-select v-model="form.network" style="width: 100%">
              <el-option label="tcp（REALITY 推荐）" value="tcp" />
              <el-option label="ws（WebSocket）" value="ws" />
              <el-option label="xhttp" value="xhttp" />
              <el-option label="grpc" value="grpc" />
            </el-select>
          </el-form-item>
          <el-form-item label="TLS">
            <el-select v-model="form.tls_type" style="width: 100%">
              <el-option label="reality（推荐）" value="reality" />
              <el-option label="tls（证书）" value="tls" />
              <el-option label="none" value="none" />
            </el-select>
          </el-form-item>
        </div>

        <!-- Fallbacks 回落 -->
        <div class="fallbacks-card">
          <div class="card-head">
            <span class="head-title">回落设置（Fallbacks，tcp 传输生效）</span>
            <el-button size="small" type="primary" plain @click="addFallback"><el-icon><Plus /></el-icon>&nbsp;添加</el-button>
          </div>
          <div v-if="form.fallbacks.length === 0" class="empty-tip">未配置回落规则</div>
          <div v-else class="fallback-list">
            <div v-for="(item, idx) in form.fallbacks" :key="idx" class="fallback-item">
              <div class="fb-grid">
                <el-form-item label="Dest 目标地址">
                  <el-input v-model="item.dest" placeholder="80 或 127.0.0.1:8080" />
                </el-form-item>
                <el-form-item label="Path 匹配">
                  <el-input v-model="item.path" placeholder="如 /" />
                </el-form-item>
                <el-form-item label="PROXY protocol (xver)">
                  <el-input-number v-model="item.xver" :min="0" :max="2" style="width: 100%" />
                </el-form-item>
              </div>
              <div class="fb-actions">
                <el-button size="small" text type="danger" @click="removeFallback(idx)"><el-icon><Delete /></el-icon></el-button>
              </div>
            </div>
          </div>
        </div>

        <!-- REALITY 参数 -->
        <template v-if="form.tls_type === 'reality'">
          <div class="sec-title">REALITY 参数</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="借壳目标（dest）">
              <el-input v-model="form.reality.dest" placeholder="www.apple.com:443" />
            </el-form-item>
            <el-form-item label="SNI（server_name）">
              <el-input v-model="form.reality.server_name" placeholder="www.apple.com" />
            </el-form-item>
            <el-form-item label="Short ID">
              <el-input v-model="form.reality.short_id" placeholder="abcdef0123456789" />
            </el-form-item>
          </div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="Private Key（服务端）">
              <el-input v-model="form.reality.private_key" placeholder="x25519 私钥" />
            </el-form-item>
            <el-form-item label="Public Key（客户端）">
              <el-input v-model="form.reality.public_key" placeholder="x25519 公钥" />
            </el-form-item>
          </div>
          <el-button size="small" :loading="genKeyLoading" @click="genRealityKeys">
            <el-icon><MagicStick /></el-icon>&nbsp;生成密钥对
          </el-button>
        </template>

        <!-- WS 参数 -->
        <template v-if="form.network === 'ws'">
          <div class="sec-title">WebSocket 参数</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="Path"><el-input v-model="form.ws.path" placeholder="/" /></el-form-item>
            <el-form-item label="Host（可选）"><el-input v-model="form.ws.host" placeholder="留空即可" /></el-form-item>
          </div>
        </template>

        <!-- XHTTP 参数 -->
        <template v-if="form.network === 'xhttp'">
          <div class="sec-title">XHTTP 参数</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="Mode">
              <el-select v-model="form.xhttp.mode" style="width: 100%">
                <el-option label="auto" value="auto" />
                <el-option label="packet-up" value="packet-up" />
                <el-option label="stream-up" value="stream-up" />
                <el-option label="stream-one" value="stream-one" />
              </el-select>
            </el-form-item>
            <el-form-item label="Path"><el-input v-model="form.xhttp.path" placeholder="/" /></el-form-item>
          </div>
        </template>

        <!-- gRPC 参数 -->
        <template v-if="form.network === 'grpc'">
          <div class="sec-title">gRPC 参数</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="Service Name">
              <el-input v-model="form.grpc.service_name" placeholder="grpc" />
            </el-form-item>
            <el-form-item label="Authority（自定义 Host，可选）">
              <el-input v-model="form.grpc.authority" placeholder="留空使用默认" />
            </el-form-item>
            <el-form-item label="Multi Mode">
              <el-switch v-model="form.grpc.multi_mode" />
            </el-form-item>
          </div>
        </template>

        <!-- TLS 参数 -->
        <template v-if="form.tls_type === 'tls'">
          <div class="sec-title">TLS 参数（节点服务器上的证书路径）</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="证书文件">
              <el-input v-model="form.tls.cert_file" placeholder="/etc/xray/cert.pem" />
            </el-form-item>
            <el-form-item label="私钥文件">
              <el-input v-model="form.tls.key_file" placeholder="/etc/xray/key.pem" />
            </el-form-item>
            <el-form-item label="SNI（可选）">
              <el-input v-model="form.tls.server_name" placeholder="example.com" />
            </el-form-item>
            <el-form-item label="ALPN（逗号分隔）">
              <el-input v-model="tlsAlpnText" placeholder="h2,http/1.1" />
            </el-form-item>
          </div>
        </template>

        <!-- 流量嗅探 -->
        <div class="sec-title">流量嗅探（Sniffing）</div>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="启用嗅探">
            <el-switch v-model="form.sniffing.enabled" />
          </el-form-item>
          <el-form-item label="DestOverride（覆盖目标类型）">
            <el-checkbox-group v-model="form.sniffing.destOverride" :disabled="!form.sniffing.enabled">
              <el-checkbox label="http" />
              <el-checkbox label="tls" />
              <el-checkbox label="quic" />
              <el-checkbox label="fakedns" />
            </el-checkbox-group>
          </el-form-item>
          <el-form-item label="MetadataOnly（仅元数据嗅探）">
            <el-switch v-model="form.sniffing.metadataOnly" :disabled="!form.sniffing.enabled" />
          </el-form-item>
          <el-form-item label="RouteOnly（仅用于路由，不改写目标）">
            <el-switch v-model="form.sniffing.routeOnly" :disabled="!form.sniffing.enabled" />
          </el-form-item>
        </div>

        <p class="muted tip">{{ settingsTip }}</p>
      </el-form>
      <div v-else class="preview-pane">
        <p class="muted tip" style="margin: 0 0 8px">按当前表单 + 该服务器已保存入站 + 全部启用用户生成的完整 Xray 配置（实时预览）：</p>
        <pre v-loading="previewLoading" class="cfg-view">{{ previewConfigText || '正在生成预览…' }}</pre>
      </div>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 部署结果 -->
    <el-dialog v-model="deployOpen" title="一键部署" width="640px">
      <pre v-loading="deployLoading" class="cfg-view">{{ deployResult || '正在生成并下发…' }}</pre>
      <template #footer>
        <el-button type="primary" @click="deployOpen = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.tip { font-size: 12px; margin: 0; }
.sec-title { font-weight: 600; font-size: 13px; margin: 4px 0 10px; padding-top: 6px; border-top: 1px dashed var(--x-border); }
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
.dialog-head { display: flex; gap: 8px; margin-bottom: 14px; }
.preview-pane { min-height: 360px; }

.fallbacks-card {
  background: var(--x-primary-soft);
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 14px;
  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
    .head-title {
      font-size: 12.5px;
      font-weight: 600;
      color: var(--x-text-2);
    }
  }
  .empty-tip {
    font-size: 12px;
    color: var(--x-text-3);
    text-align: center;
    padding: 10px 0;
  }
  .fallback-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .fallback-item {
    background: var(--x-card);
    border: 1px solid var(--x-border);
    border-radius: 6px;
    padding: 10px;
    .fb-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
      gap: 0 10px;
    }
    .fb-actions {
      display: flex;
      justify-content: flex-end;
      margin-top: 4px;
    }
  }
}

</style>