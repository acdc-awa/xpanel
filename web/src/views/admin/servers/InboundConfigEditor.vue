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
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getXrayKeys } from '@/api/admin'
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
  }>(),
  {
    modelValue: '{}',
    protocol: 'vless',
    network: 'tcp',
    tlsType: 'reality',
    port: 443,
    tag: '',
    showBaseFields: true,
  },
)

const emit = defineEmits<InboundEditorEmits>()

// 视图模式：form 可视化表单 | json 源码
const activeTab = ref<'form' | 'json'>('form')

// 基础字段
const localProtocol = ref(props.protocol || 'vless')
const localNetwork = ref(props.network || 'tcp')
const localTlsType = ref(props.tlsType || 'reality')
const localPort = ref(props.port || 443)
const localTag = ref(props.tag || '')

// Fallbacks 列表
const fallbacks = ref<FallbackItem[]>([])

// 传输层设置
const wsForm = reactive<WSSettings>({
  path: '/',
  host: '',
})

const xhttpForm = reactive<XHTTPSettings>({
  mode: 'auto',
  path: '/',
  host: '',
})

const grpcForm = reactive({
  service_name: 'grpc',
  authority: '',
  multi_mode: false,
})

// 流量嗅探（Sniffing）
const sniffingForm = reactive({
  enabled: false,
  destOverride: ['http', 'tls', 'quic', 'fakedns'] as string[],
  metadataOnly: false,
  routeOnly: false,
})

const tcpForm = reactive({
  header_type: 'none',
  request_host: '',
  request_path: '/',
})

// uTLS fingerprint
const fingerprint = ref('chrome')

// TLS & REALITY 设置
const realityForm = reactive<RealitySettings>({
  dest: 'www.apple.com:443',
  server_name: 'www.apple.com',
  public_key: '',
  private_key: '',
  short_id: 'abcdef0123456789',
  spider_x: '/',
})

const tlsForm = reactive<TLSSettings>({
  server_name: '',
  cert_file: '/etc/xray/cert.pem',
  key_file: '/etc/xray/key.pem',
  alpn: [],
})

const tlsAlpnText = ref('h2,http/1.1')

// Raw JSON 编辑与校验状态
const rawJsonText = ref('{}')
const jsonError = ref('')
const isInternalUpdating = ref(false)
const genKeyLoading = ref(false)

// 生成 Random Short ID
function genShortId() {
  const chars = '0123456789abcdef'
  let result = ''
  for (let i = 0; i < 16; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  realityForm.short_id = result
}

// 请求后端生成 REALITY x25519 密钥对
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

// Fallback 操作
function addFallback() {
  fallbacks.value.push({
    name: '',
    alpn: '',
    path: '',
    dest: '80',
    xver: 0,
  })
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

// 从当前表单对象构建 InboundSettings JSON
function buildSettingsJSON(): string {
  const s: Record<string, any> = { decryption: 'none' }
  if (fallbacks.value.length > 0) {
    s.fallbacks = fallbacks.value.map((f) => ({
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

  if (localNetwork.value === 'ws') {
    const ws: Record<string, any> = { path: wsForm.path || '/' }
    if (wsForm.host) ws.headers = { Host: wsForm.host }
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
  } else if (localTlsType.value === 'tls') {
    const alpnArr = tlsAlpnText.value ? tlsAlpnText.value.split(',').map((x: string) => x.trim()).filter(Boolean) : []
    const t: Record<string, any> = {
      serverName: tlsForm.server_name || undefined,
      certificates: [{ certificateFile: tlsForm.cert_file, keyFile: tlsForm.key_file }],
    }
    if (alpnArr.length > 0) t.alpn = alpnArr
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

// 同步表单数据 -> JSON & emits
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
  })
  isInternalUpdating.value = false
}

// 解析外部传入/输入的 JSON -> 表单数据
function parseJsonToForm(str: string) {
  if (!str || !str.trim()) return
  try {
    const parsed = JSON.parse(str)
    jsonError.value = ''

    // 支持解析纯 InboundSettings 或 完整 Inbound 节点对象
    let s: InboundSettings = parsed
    if (parsed.settings_json && typeof parsed.settings_json === 'string') {
      try {
        s = JSON.parse(parsed.settings_json)
      } catch {
        /* ignore */
      }
    } else if (parsed.settings && typeof parsed.settings === 'object') {
      s = parsed.settings
    }

    if (parsed.protocol && typeof parsed.protocol === 'string') localProtocol.value = parsed.protocol
    if (parsed.network && typeof parsed.network === 'string') localNetwork.value = parsed.network
    if (parsed.tls_type && typeof parsed.tls_type === 'string') localTlsType.value = parsed.tls_type
    else if (parsed.streamSettings?.security) localTlsType.value = parsed.streamSettings.security
    if (typeof parsed.port === 'number') localPort.value = parsed.port
    if (parsed.tag && typeof parsed.tag === 'string') localTag.value = parsed.tag

    // Fallbacks
    if (Array.isArray(s.fallbacks)) {
      fallbacks.value = s.fallbacks.map((f) => ({
        name: f.name || '',
        alpn: f.alpn || '',
        path: f.path || '',
        dest: f.dest || '',
        xver: f.xver || 0,
      }))
    }

    // Transport
    if (s.ws) {
      wsForm.path = s.ws.path || '/'
      wsForm.host = s.ws.host || ''
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
          : ['http', 'tls', 'quic', 'fakedns']
      sniffingForm.metadataOnly = !!s.sniffing.metadataOnly || !!s.sniffing.metadata_only
      sniffingForm.routeOnly = !!s.sniffing.routeOnly || !!s.sniffing.route_only
    }

    // Security
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

// 监听源码 Tab 中手动修改 rawJsonText
function onRawJsonInput(val: string) {
  if (isInternalUpdating.value) return
  rawJsonText.value = val
  parseJsonToForm(val)
  if (!jsonError.value) {
    syncFormToJson()
  }
}

// 监听 Props 变化
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
  () => [props.protocol, props.network, props.tlsType, props.port, props.tag],
  () => {
    if (props.protocol && props.protocol !== localProtocol.value) localProtocol.value = props.protocol
    if (props.network && props.network !== localNetwork.value) localNetwork.value = props.network
    if (props.tlsType && props.tlsType !== localTlsType.value) localTlsType.value = props.tlsType
    if (props.port && props.port !== localPort.value) localPort.value = props.port
    if (props.tag && props.tag !== localTag.value) localTag.value = props.tag
  },
)

// 监听内部表单值变动 -> 同步导出
watch(
  [
    localProtocol,
    localNetwork,
    localTlsType,
    localPort,
    localTag,
    fallbacks,
    wsForm,
    xhttpForm,
    grpcForm,
    tcpForm,
    realityForm,
    tlsForm,
    tlsAlpnText,
    fingerprint,
    sniffingForm,
  ],
  () => {
    syncFormToJson()
  },
  { deep: true },
)

onMounted(() => {
  if (!realityForm.private_key && localTlsType.value === 'reality') {
    // 自动为未填充私钥的 REALITY 生成一组默认 ID
    genShortId()
  }
})

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}
</script>

<template>
  <div class="inbound-config-editor">
    <!-- 顶部 Mode 切换 -->
    <div class="editor-header">
      <div class="header-title">
        <el-icon class="title-icon"><Setting /></el-icon>
        <span>VLESS 入站配置编辑器</span>
      </div>
      <el-radio-group v-model="activeTab" size="small">
        <el-radio-button label="form">
          <el-icon><Setting /></el-icon>&nbsp;可视化表单
        </el-radio-button>
        <el-radio-button label="json">
          <el-icon><Document /></el-icon>&nbsp;原始 JSON
        </el-radio-button>
      </el-radio-group>
    </div>

    <!-- 视图 1: 可视化表单 -->
    <div v-if="activeTab === 'form'" class="form-container">
      <!-- 基础参数 (可选显示) -->
      <div v-if="showBaseFields" class="form-section">
        <div class="section-title">入站基础网络</div>
        <div class="grid-2">
          <el-form-item label="标签 Tag">
            <el-input v-model="localTag" placeholder="如 Tokyo-VLESS-REALITY" />
          </el-form-item>
          <el-form-item label="监听端口">
            <el-input-number v-model="localPort" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
          <el-form-item label="协议 Protocol">
            <el-select v-model="localProtocol" style="width: 100%" disabled>
              <el-option label="VLESS (推荐)" value="vless" />
            </el-select>
          </el-form-item>
          <el-form-item label="传输协议 Network">
            <el-select v-model="localNetwork" style="width: 100%">
              <el-option label="TCP (REALITY 最佳)" value="tcp" />
              <el-option label="WebSocket (WS)" value="ws" />
              <el-option label="xhttp (Xray 1.8.21+)" value="xhttp" />
              <el-option label="gRPC" value="grpc" />
            </el-select>
          </el-form-item>
          <el-form-item label="安全类型 TLS / Security">
            <el-select v-model="localTlsType" style="width: 100%">
              <el-option label="REALITY (伪装/防封)" value="reality" />
              <el-option label="TLS (自定义证书)" value="tls" />
              <el-option label="none (无加密)" value="none" />
            </el-select>
          </el-form-item>
        </div>
      </div>

      <!-- VLESS 协议设置 -->
      <div class="form-section">
        <div class="section-title">VLESS 基础设置</div>
        <!-- Fallbacks 回落 -->
        <div class="fallbacks-card">
          <div class="card-head">
            <span class="head-title">回落设置 (Fallbacks)</span>
            <el-button size="small" type="primary" plain @click="addFallback">
              <el-icon><Plus /></el-icon>&nbsp;添加 Fallback
            </el-button>
          </div>
          <div v-if="fallbacks.length === 0" class="empty-tip">未配置回落规则</div>
          <div v-else class="fallback-list">
            <div v-for="(item, idx) in fallbacks" :key="idx" class="fallback-item">
              <div class="fb-grid">
                <el-form-item label="SNI 匹配">
                  <el-input v-model="item.name" placeholder="任意" />
                </el-form-item>
                <el-form-item label="ALPN 匹配">
                  <el-input v-model="item.alpn" placeholder="如 h2" />
                </el-form-item>
                <el-form-item label="Path 匹配">
                  <el-input v-model="item.path" placeholder="如 /" />
                </el-form-item>
                <el-form-item label="Dest 目标地址">
                  <el-input v-model="item.dest" placeholder="80 或 127.0.0.1:8080" />
                </el-form-item>
                <el-form-item label="PROXY protocol (xver)">
                  <el-input-number v-model="item.xver" :min="0" :max="2" style="width: 100%" />
                </el-form-item>
              </div>
              <div class="fb-actions">
                <el-button size="small" text :disabled="idx === 0" @click="moveFallback(idx, -1)">上移</el-button>
                <el-button size="small" text :disabled="idx === fallbacks.length - 1" @click="moveFallback(idx, 1)">下移</el-button>
                <el-button size="small" text type="danger" @click="removeFallback(idx)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 传输层配置 -->
      <div class="form-section">
        <div class="section-title">传输层参数 ({{ localNetwork.toUpperCase() }})</div>

        <!-- TCP -->
        <template v-if="localNetwork === 'tcp'">
          <div class="grid-2">
            <el-form-item label="伪装类型 (Header)">
              <el-select v-model="tcpForm.header_type" style="width: 100%">
                <el-option label="none (无)" value="none" />
                <el-option label="http (HTTP 报文伪装)" value="http" />
              </el-select>
            </el-form-item>
            <template v-if="tcpForm.header_type === 'http'">
              <el-form-item label="Request Host">
                <el-input v-model="tcpForm.request_host" placeholder="example.com" />
              </el-form-item>
              <el-form-item label="Request Path">
                <el-input v-model="tcpForm.request_path" placeholder="/" />
              </el-form-item>
            </template>
          </div>
        </template>

        <!-- WebSocket -->
        <template v-if="localNetwork === 'ws'">
          <div class="grid-2">
            <el-form-item label="Path 路径">
              <el-input v-model="wsForm.path" placeholder="/" />
            </el-form-item>
            <el-form-item label="Host 域名标头 (可选)">
              <el-input v-model="wsForm.host" placeholder="留空默认" />
            </el-form-item>
          </div>
        </template>

        <!-- xhttp -->
        <template v-if="localNetwork === 'xhttp'">
          <div class="grid-2">
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
          </div>
        </template>

        <!-- gRPC -->
        <template v-if="localNetwork === 'grpc'">
          <div class="grid-2">
            <el-form-item label="Service Name">
              <el-input v-model="grpcForm.service_name" placeholder="grpc" />
            </el-form-item>
            <el-form-item label="Authority（自定义 Host，可选）">
              <el-input v-model="grpcForm.authority" placeholder="留空使用默认" />
            </el-form-item>
            <el-form-item label="Multi Mode">
              <el-switch v-model="grpcForm.multi_mode" />
            </el-form-item>
          </div>
        </template>
      </div>

      <!-- 安全/加密配置 -->
      <div class="form-section">
        <div class="section-title">安全加密 ({{ localTlsType.toUpperCase() }})</div>

        <div style="margin-bottom: 8px">
          <el-form-item label="uTLS 指纹 (fingerprint)">
            <el-select v-model="fingerprint" style="width: 240px">
              <el-option label="chrome (推荐)" value="chrome" />
              <el-option label="firefox" value="firefox" />
              <el-option label="safari" value="safari" />
              <el-option label="edge" value="edge" />
              <el-option label="360" value="360" />
              <el-option label="qq" value="qq" />
              <el-option label="random" value="random" />
              <el-option label="randomized" value="randomized" />
            </el-select>
          </el-form-item>
        </div>

        <!-- REALITY -->
        <template v-if="localTlsType === 'reality'">
          <div class="grid-2">
            <el-form-item label="借壳目标 Dest">
              <el-input v-model="realityForm.dest" placeholder="www.apple.com:443" />
            </el-form-item>
            <el-form-item label="SNI (server_name)">
              <el-input v-model="realityForm.server_name" placeholder="www.apple.com" />
            </el-form-item>
            <el-form-item label="Short ID">
              <div style="display: flex; gap: 8px; width: 100%">
                <el-input v-model="realityForm.short_id" placeholder="abcdef0123456789" />
                <el-button @click="genShortId">
                  <el-icon><Refresh /></el-icon>&nbsp;随机
                </el-button>
              </div>
            </el-form-item>
            <el-form-item label="SpiderX 爬虫路径 (可选)">
              <el-input v-model="realityForm.spider_x" placeholder="/" />
            </el-form-item>
          </div>

          <div class="grid-2" style="margin-top: 8px">
            <el-form-item label="Private Key (服务端私钥)">
              <el-input v-model="realityForm.private_key" placeholder="x25519 私钥" />
            </el-form-item>
            <el-form-item label="Public Key (客户端公钥)">
              <el-input v-model="realityForm.public_key" placeholder="x25519 公钥" />
            </el-form-item>
          </div>
          <div style="margin-top: 8px; display: flex; gap: 10px; align-items: center">
            <el-button type="primary" plain size="small" :loading="genKeyLoading" @click="fetchRealityKeys">
              <el-icon><MagicStick /></el-icon>&nbsp;一键生成 x25519 密钥对
            </el-button>
            <el-button v-if="realityForm.private_key" size="small" text @click="copyText(realityForm.private_key, '私钥')">
              <el-icon><CopyDocument /></el-icon>&nbsp;复制私钥
            </el-button>
            <el-button v-if="realityForm.public_key" size="small" text @click="copyText(realityForm.public_key, '公钥')">
              <el-icon><CopyDocument /></el-icon>&nbsp;复制公钥
            </el-button>
          </div>
        </template>

        <!-- TLS -->
        <template v-if="localTlsType === 'tls'">
          <div class="grid-2">
            <el-form-item label="SNI (server_name)">
              <el-input v-model="tlsForm.server_name" placeholder="example.com" />
            </el-form-item>
            <el-form-item label="ALPN (逗号分割)">
              <el-input v-model="tlsAlpnText" placeholder="h2,http/1.1" />
            </el-form-item>
            <el-form-item label="Cert File (证书文件路径)">
              <el-input v-model="tlsForm.cert_file" placeholder="/etc/xray/cert.pem" />
            </el-form-item>
            <el-form-item label="Key File (私钥文件路径)">
              <el-input v-model="tlsForm.key_file" placeholder="/etc/xray/key.pem" />
            </el-form-item>
          </div>
        </template>

        <!-- none -->
        <template v-if="localTlsType === 'none'">
          <el-alert
            type="warning"
            :closable="false"
            show-icon
            title="未启用 TLS 加密"
            description="明文传输容易被 DPI 审查封锁，生产环境强烈建议选择 REALITY 或 TLS。"
          />
        </template>
      </div>

      <!-- 流量嗅探 Sniffing -->
      <div class="form-section">
        <div class="section-title">流量嗅探 (Sniffing)</div>
        <div class="grid-2">
          <el-form-item label="启用嗅探">
            <el-switch v-model="sniffingForm.enabled" />
          </el-form-item>
          <el-form-item label="DestOverride（覆盖目标类型）">
            <el-checkbox-group v-model="sniffingForm.destOverride" :disabled="!sniffingForm.enabled">
              <el-checkbox label="http" />
              <el-checkbox label="tls" />
              <el-checkbox label="quic" />
              <el-checkbox label="fakedns" />
            </el-checkbox-group>
          </el-form-item>
          <el-form-item label="MetadataOnly（仅元数据嗅探）">
            <el-switch v-model="sniffingForm.metadataOnly" :disabled="!sniffingForm.enabled" />
          </el-form-item>
          <el-form-item label="RouteOnly（仅用于路由，不改写目标）">
            <el-switch v-model="sniffingForm.routeOnly" :disabled="!sniffingForm.enabled" />
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 视图 2: 原始 JSON 编辑器 -->
    <div v-else class="json-container">
      <div class="json-head">
        <span>实时双向同步 JSON 源代码：</span>
        <el-tag v-if="!jsonError" type="success" size="small">
          <el-icon><Check /></el-icon>&nbsp;JSON 语法正确
        </el-tag>
        <el-tag v-else type="danger" size="small">
          <el-icon><Warning /></el-icon>&nbsp;语法错误
        </el-tag>
      </div>
      <el-alert v-if="jsonError" type="error" :closable="false" :title="jsonError" style="margin-bottom: 10px" />
      <el-input
        :model-value="rawJsonText"
        type="textarea"
        :rows="18"
        class="code-textarea"
        placeholder='{"reality": {"dest": "www.apple.com:443", ...}}'
        @input="onRawJsonInput"
      />
    </div>
  </div>
</template>

<style scoped lang="scss">
.inbound-config-editor {
  background: var(--x-bg-card, #ffffff);
  border-radius: 8px;
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: var(--x-bg-muted, #f8fafc);
  border-bottom: 1px solid var(--x-border, #e2e8f0);
  border-radius: 8px 8px 0 0;

  .header-title {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 600;
    font-size: 13.5px;
    color: var(--x-text-1, #1e293b);

    .title-icon {
      color: var(--x-primary, #4f46e5);
    }
  }
}

.form-container,
.json-container {
  padding: 14px;
}

.form-section {
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px dashed var(--x-border, #e2e8f0);

  &:last-child {
    border-bottom: none;
    margin-bottom: 0;
  }

  .section-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--x-primary, #4f46e5);
    margin-bottom: 12px;
  }
}

.grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.fallbacks-card {
  background: var(--x-bg-soft, #f1f5f9);
  border-radius: 6px;
  padding: 12px;
  margin-top: 10px;

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;

    .head-title {
      font-size: 12.5px;
      font-weight: 600;
      color: var(--x-text-2, #475569);
    }
  }

  .empty-tip {
    font-size: 12px;
    color: var(--x-text-3, #94a3b8);
    text-align: center;
    padding: 10px 0;
  }

  .fallback-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .fallback-item {
    background: #ffffff;
    border: 1px solid var(--x-border, #cbd5e1);
    border-radius: 6px;
    padding: 10px;
    position: relative;

    .fb-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
      gap: 0 10px;
    }

    .fb-actions {
      display: flex;
      justify-content: flex-end;
      gap: 6px;
      margin-top: 4px;
    }
  }
}

.json-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  font-size: 12.5px;
  color: var(--x-text-2, #475569);
}

.code-textarea {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.5;
}
</style>
