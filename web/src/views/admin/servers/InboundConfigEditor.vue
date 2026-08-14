<script setup lang="ts">
import { ref, reactive, watch, onMounted } from 'vue'
import {
  Plus,
  Delete,
  Refresh,
  MagicStick,
  Document,
  Setting,
  Check,
  Warning,
  CopyDocument,
  QuestionFilled,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getCerts, getPermissionGroups, getXrayKeys, rotateInternalInbound, type CertItem, type PermissionGroup } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { FallbackItem, InboundSettings, RealitySettings, TLSSettings, WSSettings, XHTTPSettings } from '@/api/types'

export interface InboundEditorChangePayload {
  settingsJson: string
  streamSettings: string
  sniffing: string
  protocol: string
  port: number
  tag: string
  listen: string
  flow: string
  ratio: number
  total_gb: number
  expiry_time: string | null
  shareAddrStrategy: string
  shareAddr: string
  sharePort: number
  permissionGroupIds: number[]
}

export interface InboundEditorEmits {
  (e: 'update:modelValue', value: string): void
  (e: 'change', payload: InboundEditorChangePayload): void
}

const props = withDefaults(
  defineProps<{
    modelValue?: string
    protocol?: string
    network?: string
    tlsType?: string
    port?: number
    tag?: string
    showBaseFields?: boolean
    inboundType?: string
    internalUUID?: string
    inboundId?: number
    certId?: number
    permissionGroupIds?: number[]
  }>(),
  {
    modelValue: '{}',
    protocol: 'vless',
    network: 'tcp',
    tlsType: 'reality',
    port: 443,
    tag: '',
    showBaseFields: true,
    inboundType: 'user',
    internalUUID: '',
    inboundId: 0,
    certId: 0,
    permissionGroupIds: () => [],
  },
)

const emit = defineEmits<InboundEditorEmits & {
  (e: 'update:inboundType', value: string): void
  (e: 'update:certId', value: number): void
  (e: 'internal-uuid-changed', value: string): void
}>()

// 视图模式：form 表单 | json 源码
const activeView = ref<'form' | 'json'>('form')

// 表单 Tab：basic 基础信息 | network_security 协议与安全 | advanced 高级扩展
const activeTab = ref('basic')

// 基础字段
const localProtocol = ref(props.protocol || 'vless')
const localNetwork = ref(props.network || 'tcp')
const localTlsType = ref(props.tlsType || 'reality')
const localPort = ref(props.port || 443)
const localTag = ref(props.tag || '')

// 入站级扩展字段
const localFlow = ref('')
const localRatio = ref(1)
const localTotalGB = ref(0)
const localExpiryTime = ref<string | null>(null)
const localShareStrategy = ref('node')
const localShareAddr = ref('')
const localSharePort = ref(0)
const localPermissionGroupIds = ref<number[]>(props.permissionGroupIds ? [...props.permissionGroupIds] : [])
const permissionGroups = ref<PermissionGroup[]>([])

// Phase T 入站三态与证书
const localInboundType = ref(props.inboundType || 'user')
const localInternalUUID = ref(props.internalUUID || '')
const localCertId = ref<number>(props.certId || 0)
const certs = ref<CertItem[]>([])

// Fallbacks 列表
const fallbacks = ref<FallbackItem[]>([])

// 传输层参数
const wsForm = reactive<WSSettings>({ path: '/', host: '' })
const xhttpForm = reactive<XHTTPSettings>({ mode: 'auto', path: '/', host: '' })
const grpcForm = reactive({ service_name: 'grpc', authority: '', multi_mode: false })
const tcpForm = reactive({ header_type: 'none', request_host: '', request_path: '/' })
const acceptProxyProtocol = ref(false)
const fingerprint = ref('chrome')

// REALITY 参数
const realityForm = reactive<RealitySettings>({
  dest: 'gateway.icloud.com:443',
  server_name: 'gateway.icloud.com',
  public_key: '',
  private_key: '',
  short_id: 'abcdef0123456789',
  spider_x: '/',
})
const realityMinClientVer = ref('')
const realityMaxClientVer = ref('')
const realityMaxTimeDiff = ref(0)

// TLS 参数
const tlsForm = reactive<TLSSettings>({
  server_name: '',
  cert_file: '/etc/xray/cert.pem',
  key_file: '/etc/xray/key.pem',
  alpn: [],
})
const tlsAlpnText = ref('h2,http/1.1')
const tlsMinVersion = ref('')
const tlsMaxVersion = ref('')
const tlsCipherSuites = ref('')
const tlsAllowInsecure = ref(false)

// Sniffing 参数
const sniffingForm = reactive({
  enabled: false,
  destOverride: ['http', 'tls', 'quic'] as string[],
  metadataOnly: false,
  routeOnly: false,
})

// REALITY 优质预设
const REALITY_PRESETS = [
  { label: 'Apple iCloud (gateway.icloud.com:443)', dest: 'gateway.icloud.com:443', sni: 'gateway.icloud.com' },
  { label: 'Apple iTunes (itunes.apple.com:443)', dest: 'itunes.apple.com:443', sni: 'itunes.apple.com' },
  { label: 'Apple (www.apple.com:443)', dest: 'www.apple.com:443', sni: 'www.apple.com' },
  { label: 'Cloudflare (www.cloudflare.com:443)', dest: 'www.cloudflare.com:443', sni: 'www.cloudflare.com' },
  { label: 'Amazon AWS (aws.amazon.com:443)', dest: 'aws.amazon.com:443', sni: 'aws.amazon.com' },
]

function applyRealityPreset(preset: { dest: string; sni: string }) {
  realityForm.dest = preset.dest
  realityForm.server_name = preset.sni
  ElMessage.success(`已应用借壳预设: ${preset.sni}`)
}

// JSON 编辑状态
const rawJsonText = ref('{}')
const jsonError = ref('')
const isInternalUpdating = ref(false)
const genKeyLoading = ref(false)

function genShortId() {
  const chars = '0123456789abcdef'
  let result = ''
  for (let i = 0; i < 16; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  realityForm.short_id = result
}

async function fetchRealityKeys() {
  genKeyLoading.value = true
  try {
    const { data } = await getXrayKeys()
    if (data.code === 0) {
      realityForm.private_key = data.data.private_key
      realityForm.public_key = data.data.public_key
      ElMessage.success('已生成 x25519 密钥对')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成密钥失败'))
  } finally {
    genKeyLoading.value = false
  }
}

async function rotateInternal() {
  if (!props.inboundId) {
    ElMessage.warning('请先保存入站')
    return
  }
  try {
    await ElMessageBox.confirm('将重新生成内部 UUID，引用该落地入站的中转配置会自动更新，确认轮换？', '轮换内部账户', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await rotateInternalInbound(props.inboundId)
    if (data.code === 0) {
      localInternalUUID.value = data.data.internal_uuid
      emit('internal-uuid-changed', data.data.internal_uuid)
      ElMessage.success('内部 UUID 已轮换，配置已重新生成推送')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '轮换失败'))
  }
}

function addFallback() {
  fallbacks.value.push({ name: '', alpn: '', path: '', dest: '80', xver: 0 })
}

function removeFallback(index: number) {
  fallbacks.value.splice(index, 1)
}

function moveFallback(index: number, direction: -1 | 1) {
  const target = index + direction
  if (target < 0 || target >= fallbacks.value.length) return
  const temp = fallbacks.value[index]
  fallbacks.value[index] = fallbacks.value[target]
  fallbacks.value[target] = temp
}

function buildSettingsJSON(): string {
  const s: Record<string, any> = { decryption: 'none' }
  if (fallbacks.value.length > 0) {
    s.fallbacks = fallbacks.value.map((f) => ({
      name: f.name || undefined,
      alpn: f.alpn || undefined,
      dest: f.dest,
      path: f.path || undefined,
      xver: f.xver || 0,
    }))
  }
  return JSON.stringify(s)
}

function buildStreamSettingsJSON(): string {
  const s: Record<string, any> = {
    network: localNetwork.value,
    security: localTlsType.value,
    fingerprint: fingerprint.value,
  }
  if (acceptProxyProtocol.value) s.acceptProxyProtocol = true

  if (localNetwork.value === 'ws') {
    const ws: Record<string, any> = { path: wsForm.path || '/' }
    if (wsForm.host) ws.host = wsForm.host
    s.wsSettings = ws
  } else if (localNetwork.value === 'xhttp') {
    s.xhttpSettings = { mode: xhttpForm.mode || 'auto', path: xhttpForm.path || '/' }
    if (xhttpForm.host) s.xhttpSettings.host = xhttpForm.host
  } else if (localNetwork.value === 'grpc') {
    const g: Record<string, any> = { serviceName: grpcForm.service_name || 'grpc' }
    if (grpcForm.authority) g.authority = grpcForm.authority
    if (grpcForm.multi_mode) g.multiMode = true
    s.grpcSettings = g
  } else if (localNetwork.value === 'tcp' && tcpForm.header_type !== 'none') {
    const hdr: Record<string, any> = { type: tcpForm.header_type }
    if (tcpForm.header_type === 'http') {
      hdr.request = {}
      if (tcpForm.request_host) hdr.request.headers = { Host: tcpForm.request_host.split(',') }
      if (tcpForm.request_path) hdr.request.path = tcpForm.request_path.split(',')
    }
    s.tcpSettings = { header: hdr }
  }

  if (localTlsType.value === 'reality') {
    s.realitySettings = {
      show: false,
      dest: realityForm.dest,
      serverNames: realityForm.server_name ? realityForm.server_name.split(',').map((x: string) => x.trim()).filter(Boolean) : [],
      privateKey: realityForm.private_key,
      shortIds: realityForm.short_id ? realityForm.short_id.split(',').map((x: string) => x.trim()).filter(Boolean) : [],
    }
    if (realityForm.public_key) s.realitySettings.publicKey = realityForm.public_key
    if (realityForm.spider_x && realityForm.spider_x !== '/') s.realitySettings.spiderX = realityForm.spider_x
    if (realityMinClientVer.value) s.realitySettings.minClientVer = realityMinClientVer.value
    if (realityMaxClientVer.value) s.realitySettings.maxClientVer = realityMaxClientVer.value
    if (realityMaxTimeDiff.value > 0) s.realitySettings.maxTimeDiff = realityMaxTimeDiff.value
  } else if (localTlsType.value === 'tls') {
    const alpnArr = tlsAlpnText.value ? tlsAlpnText.value.split(',').map((x: string) => x.trim()).filter(Boolean) : []
    const t: Record<string, any> = {
      serverName: tlsForm.server_name || undefined,
      certificates: [{ certificateFile: tlsForm.cert_file, keyFile: tlsForm.key_file }],
    }
    if (alpnArr.length > 0) t.alpn = alpnArr
    if (tlsMinVersion.value) t.minVersion = tlsMinVersion.value
    if (tlsMaxVersion.value) t.maxVersion = tlsMaxVersion.value
    if (tlsCipherSuites.value) t.cipherSuites = tlsCipherSuites.value
    if (tlsAllowInsecure.value) t.allowInsecure = true
    s.tlsSettings = t
  }

  return JSON.stringify(s)
}

function buildSniffingJSON(): string {
  if (!sniffingForm.enabled) return ''
  return JSON.stringify({
    enabled: true,
    destOverride: sniffingForm.destOverride,
    metadataOnly: sniffingForm.metadataOnly,
    routeOnly: sniffingForm.routeOnly,
  })
}

function syncFormToJson() {
  if (isInternalUpdating.value) return
  isInternalUpdating.value = true

  const settingsJson = buildSettingsJSON()
  const streamJson = buildStreamSettingsJSON()
  const sniffJson = buildSniffingJSON()

  rawJsonText.value = JSON.stringify({
    settings_json: JSON.parse(settingsJson),
    stream_settings: JSON.parse(streamJson),
    sniffing: sniffJson ? JSON.parse(sniffJson) : null,
  }, null, 2)
  jsonError.value = ''

  emit('update:modelValue', rawJsonText.value)
  emit('change', {
    settingsJson,
    streamSettings: streamJson,
    sniffing: sniffJson,
    protocol: localProtocol.value,
    port: localPort.value,
    tag: localTag.value,
    listen: '0.0.0.0',
    flow: localFlow.value,
    ratio: localRatio.value,
    total_gb: localTotalGB.value,
    expiry_time: localExpiryTime.value,
    shareAddrStrategy: localShareStrategy.value,
    shareAddr: localShareAddr.value,
    sharePort: localSharePort.value,
    permissionGroupIds: localPermissionGroupIds.value,
  })
  isInternalUpdating.value = false
}

function parseJsonToForm(str: string) {
  if (!str || !str.trim()) return
  try {
    const parsed = JSON.parse(str)
    jsonError.value = ''

    let s: InboundSettings = parsed
    if (parsed.settings_json && typeof parsed.settings_json === 'string') {
      try {
        s = JSON.parse(parsed.settings_json)
      } catch { /* ignore */ }
    } else if (parsed.settings && typeof parsed.settings === 'object') {
      s = parsed.settings
    }

    if (parsed.protocol && typeof parsed.protocol === 'string') localProtocol.value = parsed.protocol
    if (parsed.network && typeof parsed.network === 'string') localNetwork.value = parsed.network
    if (parsed.tls_type && typeof parsed.tls_type === 'string') localTlsType.value = parsed.tls_type
    else if (parsed.streamSettings?.security) localTlsType.value = parsed.streamSettings.security
    if (typeof parsed.port === 'number') localPort.value = parsed.port
    if (parsed.tag && typeof parsed.tag === 'string') localTag.value = parsed.tag

    if (typeof parsed.flow === 'string') localFlow.value = parsed.flow
    if (typeof parsed.ratio === 'number') localRatio.value = parsed.ratio
    if (typeof parsed.total_gb === 'number') localTotalGB.value = parsed.total_gb
    if (typeof parsed.expiry_time === 'string' && parsed.expiry_time) localExpiryTime.value = parsed.expiry_time
    if (typeof parsed.share_addr_strategy === 'string') localShareStrategy.value = parsed.share_addr_strategy
    if (typeof parsed.share_addr === 'string') localShareAddr.value = parsed.share_addr
    if (typeof parsed.share_port === 'number') localSharePort.value = parsed.share_port
    if (Array.isArray(parsed.permission_group_ids)) localPermissionGroupIds.value = parsed.permission_group_ids

    if (Array.isArray(s.fallbacks)) {
      fallbacks.value = s.fallbacks.map((f) => ({
        name: f.name || '',
        alpn: f.alpn || '',
        path: f.path || '',
        dest: f.dest || '',
        xver: f.xver || 0,
      }))
    }

    if (s.ws) {
      wsForm.path = s.ws.path || '/'
      wsForm.host = s.ws.host || s.ws.headers?.Host || ''
    }
    if (s.xhttp) {
      xhttpForm.mode = s.xhttp.mode || 'auto'
      xhttpForm.path = s.xhttp.path || '/'
      xhttpForm.host = s.xhttp.host || ''
    }
    if (s.grpc) {
      grpcForm.service_name = s.grpc.service_name || 'grpc'
      grpcForm.authority = s.grpc.authority || ''
      grpcForm.multi_mode = !!s.grpc.multi_mode
    }

    if (s.sniffing) {
      sniffingForm.enabled = !!s.sniffing.enabled
      sniffingForm.destOverride = Array.isArray(s.sniffing.destOverride)
        ? s.sniffing.destOverride
        : Array.isArray(s.sniffing.dest_override)
          ? s.sniffing.dest_override
          : ['http', 'tls', 'quic']
      sniffingForm.metadataOnly = !!s.sniffing.metadataOnly || !!s.sniffing.metadata_only
      sniffingForm.routeOnly = !!s.sniffing.routeOnly || !!s.sniffing.route_only
    }

    if (s.reality) {
      realityForm.dest = s.reality.dest || ''
      realityForm.server_name = s.reality.server_name || (s.reality.server_names?.[0] ?? '')
      realityForm.private_key = s.reality.private_key || ''
      realityForm.public_key = s.reality.public_key || ''
      realityForm.short_id = s.reality.short_id || (s.reality.short_ids?.[0] ?? '')
      realityForm.spider_x = s.reality.spider_x || '/'
    }

    if (s.tls) {
      tlsForm.server_name = s.tls.server_name || ''
      tlsForm.cert_file = s.tls.cert_file || ''
      tlsForm.key_file = s.tls.key_file || ''
      if (Array.isArray(s.tls.alpn)) {
        tlsAlpnText.value = s.tls.alpn.join(',')
      }
    }
  } catch (e: any) {
    jsonError.value = `JSON 语法错误: ${e?.message || '解析失败'}`
  }
}

function onRawJsonInput(val: string) {
  if (isInternalUpdating.value) return
  rawJsonText.value = val
  parseJsonToForm(val)
  if (!jsonError.value) {
    syncFormToJson()
  }
}

watch(
  () => props.modelValue,
  (newVal) => {
    if (isInternalUpdating.value) return
    if (newVal !== rawJsonText.value) {
      rawJsonText.value = newVal || '{}'
      parseJsonToForm(rawJsonText.value)
    }
  },
  { immediate: true },
)

watch(
  [
    localProtocol,
    localNetwork,
    localTlsType,
    localPort,
    localTag,
    localFlow,
    localRatio,
    localShareStrategy,
    localShareAddr,
    localSharePort,
    localPermissionGroupIds,
    fallbacks,
    wsForm,
    xhttpForm,
    grpcForm,
    tcpForm,
    acceptProxyProtocol,
    realityForm,
    realityMinClientVer,
    realityMaxClientVer,
    realityMaxTimeDiff,
    tlsForm,
    tlsAlpnText,
    tlsMinVersion,
    tlsMaxVersion,
    tlsCipherSuites,
    tlsAllowInsecure,
    fingerprint,
    sniffingForm,
  ],
  () => {
    syncFormToJson()
  },
  { deep: true },
)

async function loadCerts() {
  try {
    const { data } = await getCerts()
    if (data.code === 0) certs.value = data.data.items
  } catch { /* ignore */ }
}

async function loadPermissionGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) permissionGroups.value = data.data.items
  } catch { /* ignore */ }
}

onMounted(() => {
  if (!realityForm.private_key && localTlsType.value === 'reality') {
    genShortId()
  }
  loadCerts()
  loadPermissionGroups()
})

watch(
  () => props.inboundType,
  (v) => { if (v) localInboundType.value = v },
)
watch(
  () => props.internalUUID,
  (v) => { if (v) localInternalUUID.value = v },
)
watch(
  () => props.certId,
  (v) => { if (v) localCertId.value = v },
)
watch(
  () => props.permissionGroupIds,
  (v) => { if (v) localPermissionGroupIds.value = [...v] },
)

function onTypeChange(v: any) {
  localInboundType.value = String(v || 'user')
  emit('update:inboundType', localInboundType.value)
}

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}
</script>

<template>
  <div class="inbound-editor">
    <!-- 顶部功能区：左侧 Tabs，右侧 源码切换 -->
    <div class="editor-top-nav">
      <el-radio-group v-if="activeView === 'form'" v-model="activeTab" size="small" class="category-tabs">
        <el-radio-button value="basic">基础配置</el-radio-button>
        <el-radio-button value="network_security">协议与安全</el-radio-button>
        <el-radio-button value="advanced">高级设置</el-radio-button>
      </el-radio-group>
      <div v-else class="json-title">
        <span>原始 JSON 模式</span>
      </div>

      <div class="view-toggle">
        <el-radio-group v-model="activeView" size="small">
          <el-radio-button value="form">
            <el-icon><Setting /></el-icon>&nbsp;表单
          </el-radio-button>
          <el-radio-button value="json">
            <el-icon><Document /></el-icon>&nbsp;JSON
          </el-radio-button>
        </el-radio-group>
      </div>
    </div>

    <!-- 表单视图 -->
    <div v-if="activeView === 'form'" class="form-body">
      <el-form label-position="top">
        <!-- ==================== 1. 基础配置 Tab ==================== -->
        <div v-show="activeTab === 'basic'" class="tab-pane">
          <div class="form-card">
            <div class="card-title">接入点模式</div>
            <el-radio-group v-model="localInboundType" class="type-radios" @change="onTypeChange">
              <el-radio value="user">
                <span>用户入站</span>
                <span class="type-sub">进入客户端订阅</span>
              </el-radio>
              <el-radio value="relay">
                <span>转发落地</span>
                <span class="type-sub">中转内部节点</span>
              </el-radio>
              <el-radio value="idle">
                <span>闲置预留</span>
                <span class="type-sub">暂不生成配置</span>
              </el-radio>
            </el-radio-group>

            <div v-if="localInboundType === 'relay'" class="relay-box">
              <el-form-item label="内部 UUID（由节点自治生成）">
                <div style="display: flex; gap: 8px; width: 100%">
                  <el-input :model-value="localInternalUUID" placeholder="保存并下发后由节点回填" disabled />
                  <el-button v-if="localInternalUUID" @click="copyText(localInternalUUID, '内部 UUID')">
                    <el-icon><CopyDocument /></el-icon>
                  </el-button>
                  <el-button type="primary" plain :disabled="!props.inboundId" @click="rotateInternal">
                    <el-icon><Refresh /></el-icon>&nbsp;轮换
                  </el-button>
                </div>
              </el-form-item>
            </div>
          </div>

          <div class="form-card">
            <div class="card-title">网络与计费</div>
            <div class="form-grid">
              <el-form-item label="节点标签 (Tag)">
                <el-input v-model="localTag" placeholder="如 Tokyo-01-VLESS" />
              </el-form-item>

              <el-form-item label="监听端口">
                <el-input-number v-model="localPort" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>

              <el-form-item v-if="localInboundType === 'user'">
                <template #label>
                  <span>开放权限组</span>
                  <el-tooltip content="多选指定允许订阅该入站的权限组。留空则对所有正常用户开放。" placement="top">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <el-select
                  v-model="localPermissionGroupIds"
                  multiple
                  placeholder="留空 = 全员开放"
                  style="width: 100%"
                  collapse-tags
                  collapse-tags-tooltip
                >
                  <el-option
                    v-for="g in permissionGroups"
                    :key="g.id"
                    :label="g.name"
                    :value="g.id"
                  >
                    <span>{{ g.name }}</span>
                    <span v-if="g.remark" class="opt-remark">({{ g.remark }})</span>
                  </el-option>
                </el-select>
              </el-form-item>

              <el-form-item>
                <template #label>
                  <span>流量计费倍率</span>
                  <el-tooltip content="扣费倍率：1.00 为正常计费，1.50 表示消耗 1GB 扣减 1.5GB 配额。" placement="top">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <el-input-number v-model="localRatio" :min="0.1" :step="0.1" :precision="2" style="width: 100%" />
              </el-form-item>

              <el-form-item>
                <template #label>
                  <span>入站总流量上限 (GB)</span>
                  <el-tooltip content="该入站所有用户合计流量上限，0 = 不限；跑满后自动停用（临时节点按量租用场景）。" placement="top">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <el-input-number v-model="localTotalGB" :min="0" :step="10" :precision="0" style="width: 100%" />
              </el-form-item>

              <el-form-item label="入站到期时间">
                <el-date-picker
                  v-model="localExpiryTime"
                  type="datetime"
                  placeholder="留空 = 永久"
                  style="width: 100%"
                  value-format="YYYY-MM-DDTHH:mm:ssZ"
                />
              </el-form-item>
            </div>
          </div>
        </div>

        <!-- ==================== 2. 协议与安全 Tab ==================== -->
        <div v-show="activeTab === 'network_security'" class="tab-pane">
          <!-- 核心协议选择器 -->
          <div class="form-card">
            <div class="card-title">核心协议选择</div>
            <div class="form-grid">
              <el-form-item label="传输协议 (Network)">
                <el-select v-model="localNetwork" style="width: 100%">
                  <el-option label="TCP (REALITY 最佳)" value="tcp" />
                  <el-option label="WebSocket (WS 兼容)" value="ws" />
                  <el-option label="xhttp (新一代高性能)" value="xhttp" />
                  <el-option label="gRPC" value="grpc" />
                </el-select>
              </el-form-item>

              <el-form-item label="安全层 (Security)">
                <el-select v-model="localTlsType" style="width: 100%">
                  <el-option label="REALITY (免证书伪装 · 推荐)" value="reality" />
                  <el-option label="TLS (SSL 证书托管)" value="tls" />
                  <el-option label="none (无加密明文)" value="none" />
                </el-select>
              </el-form-item>

              <el-form-item v-if="localInboundType !== 'relay'">
                <template #label>
                  <span>用户流控 (Flow)</span>
                  <el-tooltip content="TCP + REALITY 推荐开启 xtls-rprx-vision 提升传输效率与抗封锁能力。" placement="top">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <el-select v-model="localFlow" style="width: 100%">
                  <el-option label="自动 (TCP+REALITY 自动注入 Vision)" value="" />
                  <el-option label="开启 (xtls-rprx-vision)" value="xtls-rprx-vision" />
                  <el-option label="关闭 (禁用流控)" value="none" />
                </el-select>
              </el-form-item>

              <el-form-item v-if="localTlsType === 'reality'">
                <template #label>
                  <span>客户端指纹 (uTLS)</span>
                  <el-tooltip content="模拟主流浏览器 TLS 握手指纹特征。" placement="top">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <el-select v-model="fingerprint" style="width: 100%">
                  <el-option label="chrome (推荐)" value="chrome" />
                  <el-option label="firefox" value="firefox" />
                  <el-option label="safari" value="safari" />
                  <el-option label="edge" value="edge" />
                  <el-option label="360" value="360" />
                  <el-option label="qq" value="qq" />
                  <el-option label="random" value="random" />
                  <el-option label="randomized (随机)" value="randomized" />
                </el-select>
              </el-form-item>
            </div>
          </div>

          <!-- 传输层专属参数 (随 Network 动态切换) -->
          <div class="form-card">
            <div class="card-title">传输层参数 ({{ localNetwork.toUpperCase() }})</div>

            <!-- TCP -->
            <div v-if="localNetwork === 'tcp'" class="form-grid">
              <el-form-item label="伪装类型 (Header)">
                <el-select v-model="tcpForm.header_type" style="width: 100%">
                  <el-option label="none (无伪装)" value="none" />
                  <el-option label="http (HTTP 报文伪装)" value="http" />
                </el-select>
              </el-form-item>

              <el-form-item label="HAProxy 代理协议 (acceptProxyProtocol)">
                <div style="padding-top: 6px">
                  <el-switch v-model="acceptProxyProtocol" active-text="接收 Proxy Protocol" />
                </div>
              </el-form-item>

              <template v-if="tcpForm.header_type === 'http'">
                <el-form-item label="Request Host">
                  <el-input v-model="tcpForm.request_host" placeholder="如 example.com" />
                </el-form-item>
                <el-form-item label="Request Path">
                  <el-input v-model="tcpForm.request_path" placeholder="/" />
                </el-form-item>
              </template>
            </div>

            <!-- WebSocket -->
            <div v-if="localNetwork === 'ws'" class="form-grid">
              <el-form-item label="Path 路径">
                <el-input v-model="wsForm.path" placeholder="/ws" />
              </el-form-item>
              <el-form-item label="Host 域名标头 (可选)">
                <el-input v-model="wsForm.host" placeholder="留空默认" />
              </el-form-item>
              <el-form-item label="HAProxy 代理协议 (acceptProxyProtocol)">
                <div style="padding-top: 6px">
                  <el-switch v-model="acceptProxyProtocol" active-text="接收 Proxy Protocol" />
                </div>
              </el-form-item>
            </div>

            <!-- xhttp -->
            <div v-if="localNetwork === 'xhttp'" class="form-grid">
              <el-form-item label="Mode 传输模式">
                <el-select v-model="xhttpForm.mode" style="width: 100%">
                  <el-option label="auto (自动)" value="auto" />
                  <el-option label="packet-up" value="packet-up" />
                  <el-option label="stream-up" value="stream-up" />
                  <el-option label="stream-one" value="stream-one" />
                </el-select>
              </el-form-item>
              <el-form-item label="Path 路径">
                <el-input v-model="xhttpForm.path" placeholder="/" />
              </el-form-item>
              <el-form-item label="Host 域名标头 (可选)">
                <el-input v-model="xhttpForm.host" placeholder="留空默认" />
              </el-form-item>
              <el-form-item label="HAProxy 代理协议 (acceptProxyProtocol)">
                <div style="padding-top: 6px">
                  <el-switch v-model="acceptProxyProtocol" active-text="接收 Proxy Protocol" />
                </div>
              </el-form-item>
            </div>

            <!-- gRPC -->
            <div v-if="localNetwork === 'grpc'" class="form-grid">
              <el-form-item label="Service Name">
                <el-input v-model="grpcForm.service_name" placeholder="grpc" />
              </el-form-item>
              <el-form-item label="Authority 标头 (可选)">
                <el-input v-model="grpcForm.authority" placeholder="留空默认" />
              </el-form-item>
              <el-form-item label="Multi Mode (多路复用)">
                <div style="padding-top: 6px">
                  <el-switch v-model="grpcForm.multi_mode" active-text="开启 Multi Mode" />
                </div>
              </el-form-item>
            </div>
          </div>

          <!-- REALITY 详细配置 -->
          <div v-if="localTlsType === 'reality'" class="form-card">
            <div class="card-head-flex">
              <div class="card-title">REALITY 伪装参数</div>
              <el-dropdown trigger="click" @command="applyRealityPreset">
                <el-button size="small" text type="primary">
                  <span>优质目标预设</span>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-for="p in REALITY_PRESETS" :key="p.dest" :command="p">
                      {{ p.label }}
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>

            <div class="form-grid">
              <el-form-item label="借壳目标 (Dest)">
                <el-input v-model="realityForm.dest" placeholder="如 gateway.icloud.com:443" />
              </el-form-item>

              <el-form-item label="伪装域名 (SNI / server_name)">
                <el-input v-model="realityForm.server_name" placeholder="如 gateway.icloud.com" />
              </el-form-item>

              <el-form-item label="Short ID">
                <div style="display: flex; gap: 8px">
                  <el-input v-model="realityForm.short_id" placeholder="16 位十六进制" />
                  <el-button @click="genShortId"><el-icon><Refresh /></el-icon></el-button>
                </div>
              </el-form-item>

              <el-form-item label="SpiderX 爬虫路径">
                <el-input v-model="realityForm.spider_x" placeholder="/" />
              </el-form-item>
            </div>

            <div class="key-box">
              <div class="key-row">
                <el-form-item label="服务端私钥 (Private Key)" style="margin-bottom: 0">
                  <el-input v-model="realityForm.private_key" placeholder="x25519 私钥" />
                </el-form-item>
                <el-form-item label="客户端公钥 (Public Key)" style="margin-bottom: 0">
                  <el-input v-model="realityForm.public_key" placeholder="x25519 公钥" />
                </el-form-item>
              </div>
              <div class="key-actions">
                <el-button type="primary" plain size="small" :loading="genKeyLoading" @click="fetchRealityKeys">
                  <el-icon><MagicStick /></el-icon>&nbsp;一键生成 x25519 密钥对
                </el-button>
                <el-button v-if="realityForm.public_key" size="small" text @click="copyText(realityForm.public_key, '公钥')">
                  <el-icon><CopyDocument /></el-icon>&nbsp;复制公钥
                </el-button>
                <el-button v-if="realityForm.private_key" size="small" text @click="copyText(realityForm.private_key, '私钥')">
                  <el-icon><CopyDocument /></el-icon>&nbsp;复制私钥
                </el-button>
              </div>
            </div>

            <div class="form-grid" style="margin-top: 14px">
              <el-form-item label="minClientVer (最低客户端版本)">
                <el-input v-model="realityMinClientVer" placeholder="留空不限，如 1.8.0" />
              </el-form-item>
              <el-form-item label="maxClientVer (最高客户端版本)">
                <el-input v-model="realityMaxClientVer" placeholder="留空不限" />
              </el-form-item>
              <el-form-item label="maxTimeDiff (最大时间差 ms)">
                <el-input-number v-model="realityMaxTimeDiff" :min="0" style="width: 100%" placeholder="0=不限制" />
              </el-form-item>
            </div>
          </div>

          <!-- TLS 详细配置 -->
          <div v-if="localTlsType === 'tls'" class="form-card">
            <div class="card-title">TLS 证书设置</div>
            <div class="form-grid">
              <el-form-item label="托管证书">
                <el-select
                  v-model="localCertId"
                  style="width: 100%"
                  clearable
                  placeholder="不绑定（使用下方本地路径）"
                  @change="(v: any) => emit('update:certId', Number(v || 0))"
                >
                  <el-option v-for="ct in certs" :key="ct.id" :label="`${ct.domain}（到期 ${ct.not_after}）`" :value="ct.id" />
                </el-select>
              </el-form-item>

              <el-form-item label="SNI (server_name)">
                <el-input v-model="tlsForm.server_name" placeholder="example.com" />
              </el-form-item>

              <el-form-item label="ALPN (逗号分隔)">
                <el-input v-model="tlsAlpnText" placeholder="h2,http/1.1" />
              </el-form-item>

              <el-form-item label="证书路径 (Cert File)">
                <el-input v-model="tlsForm.cert_file" placeholder="/etc/xray/cert.pem" />
              </el-form-item>

              <el-form-item label="私钥路径 (Key File)">
                <el-input v-model="tlsForm.key_file" placeholder="/etc/xray/key.pem" />
              </el-form-item>

              <el-form-item label="Cipher Suites (加密套件)">
                <el-input v-model="tlsCipherSuites" placeholder="留空默认" />
              </el-form-item>

              <el-form-item label="TLS 版本限制">
                <div style="display: flex; gap: 8px; width: 100%">
                  <el-select v-model="tlsMinVersion" placeholder="最低版本" clearable style="width: 50%">
                    <el-option label="TLS 1.2" value="1.2" />
                    <el-option label="TLS 1.3" value="1.3" />
                  </el-select>
                  <el-select v-model="tlsMaxVersion" placeholder="最高版本" clearable style="width: 50%">
                    <el-option label="TLS 1.2" value="1.2" />
                    <el-option label="TLS 1.3" value="1.3" />
                  </el-select>
                </div>
              </el-form-item>

              <el-form-item label="allowInsecure (跳过证书校验)">
                <div style="padding-top: 6px">
                  <el-switch v-model="tlsAllowInsecure" active-text="允许不安全证书" />
                </div>
              </el-form-item>
            </div>
          </div>

          <!-- none 明文提示 -->
          <div v-if="localTlsType === 'none'" class="form-card">
            <el-alert
              type="warning"
              :closable="false"
              show-icon
              title="当前为明文传输 (none)"
              description="无 TLS / REALITY 加密的流量极易被 DPI 识别与封锁，建议在生产环境选用 REALITY 或 TLS。"
            />
          </div>
        </div>

        <!-- ==================== 3. 高级设置 Tab ==================== -->
        <div v-show="activeTab === 'advanced'" class="tab-pane">
          <!-- 订阅分享地址与端口 -->
          <div v-if="localInboundType === 'user'" class="form-card">
            <div class="card-title">
              <span>订阅分享端点 (与节点监听解耦)</span>
              <el-tooltip content="订阅链接中生成的节点连接地址/端口，常用于端口转发或 CDN 转发场景。" placement="top">
                <el-icon class="help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="form-grid">
              <el-form-item label="分享地址策略">
                <el-select v-model="localShareStrategy" style="width: 100%">
                  <el-option label="跟随服务器地址 (默认)" value="node" />
                  <el-option label="监听地址 (Listen)" value="listen" />
                  <el-option label="自定义端点 (CDN/转发)" value="custom" />
                </el-select>
              </el-form-item>
              <template v-if="localShareStrategy === 'custom'">
                <el-form-item label="自定义分享地址">
                  <el-input v-model="localShareAddr" placeholder="如 cdn.example.com" />
                </el-form-item>
                <el-form-item label="自定义分享端口">
                  <el-input-number v-model="localSharePort" :min="0" :max="65535" placeholder="0 = 使用入站端口" style="width: 100%" />
                </el-form-item>
              </template>
            </div>
          </div>

          <!-- 流量嗅探 -->
          <div class="form-card">
            <div class="card-title">流量嗅探 (Sniffing)</div>
            <div class="form-grid">
              <el-form-item label="启用嗅探">
                <el-switch v-model="sniffingForm.enabled" active-text="开启流量嗅探" />
              </el-form-item>

              <el-form-item label="嗅探目标协议 (destOverride)">
                <el-checkbox-group v-model="sniffingForm.destOverride" :disabled="!sniffingForm.enabled">
                  <el-checkbox value="http" label="HTTP" />
                  <el-checkbox value="tls" label="TLS" />
                  <el-checkbox value="quic" label="QUIC" />
                </el-checkbox-group>
              </el-form-item>

              <el-form-item label="MetadataOnly (仅元数据嗅探)">
                <el-switch v-model="sniffingForm.metadataOnly" :disabled="!sniffingForm.enabled" active-text="仅元数据" />
              </el-form-item>

              <el-form-item label="RouteOnly (仅用于路由，不改写目标)">
                <el-switch v-model="sniffingForm.routeOnly" :disabled="!sniffingForm.enabled" active-text="仅用于路由" />
              </el-form-item>
            </div>
          </div>

          <!-- 回落 Fallbacks -->
          <div v-if="localInboundType !== 'relay'" class="form-card">
            <div class="card-head-flex">
              <div class="card-title">回落设置 (Fallbacks)</div>
              <el-button size="small" type="primary" plain @click="addFallback">
                <el-icon><Plus /></el-icon>&nbsp;添加回落
              </el-button>
            </div>
            <div v-if="fallbacks.length === 0" class="empty-tip">未配置回落规则</div>
            <div v-else class="fallback-list">
              <div v-for="(item, idx) in fallbacks" :key="idx" class="fallback-item">
                <div class="fb-grid">
                  <el-form-item label="SNI 匹配"><el-input v-model="item.name" placeholder="任意" /></el-form-item>
                  <el-form-item label="ALPN"><el-input v-model="item.alpn" placeholder="如 h2" /></el-form-item>
                  <el-form-item label="Path 路径"><el-input v-model="item.path" placeholder="如 /" /></el-form-item>
                  <el-form-item label="目标地址 (Dest)"><el-input v-model="item.dest" placeholder="80 或 127.0.0.1:80" /></el-form-item>
                  <el-form-item label="PROXY Protocol (xver)"><el-input-number v-model="item.xver" :min="0" :max="2" style="width: 100%" /></el-form-item>
                </div>
                <div class="fb-actions">
                  <el-button size="small" text :disabled="idx === 0" @click="moveFallback(idx, -1)">上移</el-button>
                  <el-button size="small" text :disabled="idx === fallbacks.length - 1" @click="moveFallback(idx, 1)">下移</el-button>
                  <el-button size="small" text type="danger" @click="removeFallback(idx)"><el-icon><Delete /></el-icon></el-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </el-form>
    </div>

    <!-- 原始 JSON 视图 -->
    <div v-else class="json-body">
      <div class="json-status">
        <el-tag v-if="!jsonError" type="success" size="small">
          <el-icon><Check /></el-icon>&nbsp;语法正确
        </el-tag>
        <el-tag v-else type="danger" size="small">
          <el-icon><Warning /></el-icon>&nbsp;{{ jsonError }}
        </el-tag>
      </div>
      <el-input
        :model-value="rawJsonText"
        type="textarea"
        :rows="16"
        class="code-textarea"
        @input="onRawJsonInput"
      />
    </div>
  </div>
</template>

<style scoped lang="scss">
.inbound-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.editor-top-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--x-border);
}

.category-tabs {
  :deep(.el-radio-button__inner) {
    font-weight: 500;
  }
}

.json-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text);
}

.form-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.tab-pane {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-card {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 16px 18px;
}

.card-title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--x-text);
  margin-bottom: 14px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.card-head-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
  .card-title { margin-bottom: 0; }
}

.help-icon {
  color: var(--x-text-3);
  font-size: 13px;
  margin-left: 4px;
  cursor: help;
  &:hover { color: var(--x-primary); }
}

.type-radios {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  :deep(.el-radio) {
    margin-right: 0;
    padding: 10px 16px;
    border: 1px solid var(--x-border);
    border-radius: var(--x-radius);
    background: var(--x-card);
    transition: all 0.15s ease;
    &.is-checked {
      border-color: var(--x-primary);
      background: var(--x-primary-soft);
    }
  }
}

.type-sub {
  font-size: 11.5px;
  color: var(--x-text-3);
  margin-left: 6px;
}

.relay-box {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px dashed var(--x-border);
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.opt-remark {
  color: var(--x-text-3);
  font-size: 12px;
  margin-left: 6px;
}

.key-box {
  margin-top: 8px;
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 14px 16px;
}

.key-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.key-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 10px;
}

.empty-tip {
  color: var(--x-text-3);
  font-size: 13px;
  padding: 10px 0;
  text-align: center;
}

.fallback-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.fallback-item {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-card);
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid var(--x-border);
}

.fb-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr 1fr 1fr;
  gap: 0 10px;
  flex: 1;
  :deep(.el-form-item) { margin-bottom: 0; }
}

.fb-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.json-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.json-status {
  display: flex;
  justify-content: flex-end;
}

.code-textarea {
  :deep(textarea) {
    font-family: var(--font-mono, monospace);
    font-size: 12.5px;
    background: var(--x-bg);
    color: var(--x-text);
  }
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .key-row {
    grid-template-columns: 1fr;
  }
  .fb-grid {
    grid-template-columns: 1fr;
  }
}
</style>
