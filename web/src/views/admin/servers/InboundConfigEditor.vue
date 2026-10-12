<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import {
  Check,
  Warning,
  MagicStick,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getLayers,
  createLayer,
  deleteLayer,
  getXrayKeys,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import CodeEditor from '@/components/CodeEditor.vue'
import type { AccessLayer } from '@/api/types'

// 子组件与类型导入
import type {
  InboundProtocol,
  InboundNetwork,
  InboundSecurity,
  TcpConfig,
  XhttpConfig,
  TlsConfig,
  RealityConfig,
  VlessConfig,
  DokodemoConfig,
  SocksHttpConfig,
  HysteriaConfig,
} from './inbound/types'
import TcpTransport from './inbound/transports/TcpTransport.vue'
import XhttpTransport from './inbound/transports/XhttpTransport.vue'
import RealitySecurity from './inbound/securities/RealitySecurity.vue'
import TlsSecurity from './inbound/securities/TlsSecurity.vue'
import VlessSettings from './inbound/protocols/VlessSettings.vue'
import DokodemoSettings from './inbound/protocols/DokodemoSettings.vue'
import SocksHttpSettings from './inbound/protocols/SocksHttpSettings.vue'
import HysteriaSettings from './inbound/protocols/HysteriaSettings.vue'
import BusinessSettings from './inbound/common/BusinessSettings.vue'

export interface InboundEditorChangePayload {
  settingsJson: string
  streamSettings: string
  sniffing: string
  protocol: string
  port: number
  tag: string
  listen: string
  remark?: string
  traffic_reset?: string
  type?: string
  flow: string
  ratio: number
  total_gb: number
  expiry_time: string | null
  shareAddrStrategy: string
  shareAddr: string
  sharePort: number
  shareSecurity: string
  shareSni: string
  shareHost: string
  sharePath: string
  shareAllowInsecure: boolean
  layerId: number
  targetInboundId?: number
  targetAddress?: string
  targetPort?: number
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
    listen?: string
    remark?: string
    trafficReset?: string
    showBaseFields?: boolean
    inboundType?: string
    savedInboundType?: string
    permissionGroupId?: number
    internalUUID?: string
    inboundId?: number
    targetInboundId?: number
    targetAddress?: string
    targetPort?: number
    certId?: number
    serverId?: number
    layerId?: number
    shareSecurity?: string
    shareSni?: string
    shareHost?: string
    sharePath?: string
    shareAllowInsecure?: boolean
  }>(),
  {
    modelValue: '{}',
    protocol: 'vless',
    network: 'tcp',
    tlsType: 'reality',
    port: 443,
    tag: '',
    listen: '0.0.0.0',
    remark: '',
    trafficReset: 'never',
    showBaseFields: true,
    inboundType: 'user',
    savedInboundType: '',
    permissionGroupId: 0,
    internalUUID: '',
    inboundId: 0,
    targetInboundId: undefined,
    targetAddress: '',
    targetPort: 0,
    certId: 0,
    serverId: 0,
    layerId: 0,
    shareSecurity: 'auto',
    shareSni: '',
    shareHost: '',
    sharePath: '',
    shareAllowInsecure: false,
  },
)

const emit = defineEmits<InboundEditorEmits & {
  (e: 'update:inboundType', value: string): void
  (e: 'update:certId', value: number): void
  (e: 'internal-uuid-changed', value: string): void
}>()

// 视图 Tab
const activeTab = ref('basic')

// 基础物理/业务类型
const localInboundType = ref(props.inboundType || 'user')
const localInternalUUID = ref(props.internalUUID || '')

// 核心三维度：协议层、传输层、安全层
const localProtocol = ref<InboundProtocol>((props.protocol as InboundProtocol) || 'vless')
const localNetwork = ref<InboundNetwork>((props.network as InboundNetwork) || 'tcp')
const localTlsType = ref<InboundSecurity>((props.tlsType as InboundSecurity) || 'reality')

// 基础网络字段
const localPort = ref(props.port || 443)
const localTag = ref(props.tag || '')
const localListen = ref(props.listen || '0.0.0.0')
const localRemark = ref(props.remark || '')
const localTrafficReset = ref(props.trafficReset || 'never')

// 计费与配额
const localRatio = ref(1)
const localTotalGB = ref(0)
const localExpiryTime = ref<string | null>(null)

// 订阅分享与外部层
const localShareStrategy = ref('node')
const localShareAddr = ref('')
const localSharePort = ref(0)
const localShareSecurity = ref(props.shareSecurity || 'auto')
const localShareSni = ref(props.shareSni || '')
const localShareHost = ref(props.shareHost || '')
const localSharePath = ref(props.sharePath || '')
const localShareAllowInsecure = ref(props.shareAllowInsecure || false)
const localLayerId = ref(props.layerId || 0)

// 协议层状态对象（使用 ref 保证双向绑定更新）
const vlessForm = ref<VlessConfig>({
  flow: '',
  fallbacks: [],
  decryption: '',
})

const dokodemoForm = ref<DokodemoConfig>({
  target_mode: props.targetInboundId ? 'inbound' : (props.targetAddress || props.targetPort ? 'manual' : 'draft'),
  target_inbound_id: props.targetInboundId,
  target_address: props.targetAddress || '',
  target_port: props.targetPort || 0,
  network: 'tcp,udp',
  proxy_protocol: true,
})

const socksHttpForm = ref<SocksHttpConfig>({
  auth: 'password',
  accounts: [],
  udp: true,
})

const hysteriaForm = ref<HysteriaConfig>({
  udp_idle_timeout: 60,
})

// 传输层状态对象
const tcpForm = ref<TcpConfig>({
  header_type: 'none',
  request_host: '',
  request_path: '',
  acceptProxyProtocol: false,
})

const xhttpForm = ref<XhttpConfig>({
  mode: 'auto',
  path: '/xhttp-stream',
  host: '',
  acceptProxyProtocol: false,
})

// 安全层状态对象
const realityForm = ref<RealityConfig>({
  dest: 'www.cloudflare.com:443',
  server_name: 'www.cloudflare.com',
  short_id: '',
  private_key: '',
  public_key: '',
  min_client_ver: '',
  max_client_ver: '',
  max_time_diff: 0,
})
const realityFingerprint = ref('chrome')

const tlsForm = ref<TlsConfig>({
  cert_id: props.certId || 0,
  server_name: '',
  alpn: 'h2,http/1.1',
  cert_file: '/etc/xray/cert.pem',
  key_file: '/etc/xray/key.pem',
  cipher_suites: '',
  min_version: '1.2',
  max_version: '1.3',
  allow_insecure: false,
})

// 嗅探状态
const sniffingForm = reactive({
  enabled: false,
  destOverride: ['http', 'tls', 'quic'],
  metadataOnly: false,
  routeOnly: false,
})

// 对外接入层列表
const layers = ref<AccessLayer[]>([])
const layerDeleting = ref(false)
const layerQuickOpen = ref(false)
const layerQuickSaving = ref(false)
const layerQuickForm = reactive({ name: '', host: '', port: 443, security: 'tls' })

// JSON 编辑状态
const rawJsonText = ref('')
const jsonError = ref('')
const isInternalUpdating = ref(false)

// -----------------------------------------------------------------------------
// 能力矩阵联动规则 (Capability Matrix)
// -----------------------------------------------------------------------------
const allowedNetworks = computed<InboundNetwork[]>(() => {
  if (localProtocol.value === 'hysteria') return ['hysteria']
  if (localProtocol.value === 'dokodemo-door') return ['tcp']
  if (localProtocol.value === 'mixed' || localProtocol.value === 'socks' || localProtocol.value === 'http') return ['tcp']
  return ['tcp', 'xhttp']
})

const allowedSecurities = computed<InboundSecurity[]>(() => {
  if (localProtocol.value === 'hysteria') return ['tls']
  if (localProtocol.value === 'dokodemo-door') return ['none']
  if (localProtocol.value === 'mixed' || localProtocol.value === 'socks' || localProtocol.value === 'http') return ['none', 'tls']
  return ['none', 'tls', 'reality']
})

const visibleTabs = computed(() => {
  const tabs = [
    { key: 'basic', label: '基础配置' },
    { key: 'protocol', label: '协议参数' },
  ]
  if (localProtocol.value === 'vless') {
    tabs.push({ key: 'transport', label: '传输层' })
  }
  tabs.push(
    { key: 'security', label: '安全层' },
    { key: 'sniffing', label: '流量嗅探' },
  )
  if (localInboundType.value === 'user') {
    tabs.push({ key: 'share', label: '订阅与分享' })
  }
  tabs.push({ key: 'advanced', label: '高级配置 (JSON)' })
  return tabs
})

const allowVisionFlow = computed(() => {
  return localProtocol.value === 'vless' &&
    localNetwork.value === 'tcp' &&
    (localTlsType.value === 'reality' || localTlsType.value === 'tls' || Boolean(vlessForm.value.decryption && vlessForm.value.decryption !== 'none'))
})

function enforceCapabilityMatrix() {
  if (!allowedNetworks.value.includes(localNetwork.value)) {
    localNetwork.value = allowedNetworks.value[0]
  }
  if (!allowedSecurities.value.includes(localTlsType.value)) {
    localTlsType.value = allowedSecurities.value[0]
  }
  if (!allowVisionFlow.value && vlessForm.value.flow === 'xtls-rprx-vision') {
    vlessForm.value.flow = ''
  }
  // 保证当前激活的 Tab 必须在可见列表中
  if (!visibleTabs.value.some((t) => t.key === activeTab.value)) {
    activeTab.value = 'basic'
  }
}

// -----------------------------------------------------------------------------
// 协议 / 模式切换响应
// -----------------------------------------------------------------------------
function onTypeChange(type: any) {
  const t = String(type)
  localInboundType.value = t
  emit('update:inboundType', t)
  if (t === 'tunnel') {
    localProtocol.value = 'dokodemo-door'
  } else if (t === 'relay') {
    if (localProtocol.value !== 'hysteria') {
      localProtocol.value = 'vless'
      localTlsType.value = 'none'
    }
  } else if (t === 'user') {
    if (localProtocol.value !== 'hysteria') {
      localProtocol.value = 'vless'
    }
  }
  enforceCapabilityMatrix()
  syncFormToJson()
}

function onProtocolChange(proto: any) {
  const p = String(proto) as InboundProtocol
  localProtocol.value = p
  if (p === 'hysteria') {
    localNetwork.value = 'hysteria'
    localTlsType.value = 'tls'
    if (localInboundType.value === 'tunnel' || localInboundType.value === 'forward') {
      localInboundType.value = 'user'
    }
  } else if (p === 'dokodemo-door') {
    localInboundType.value = 'tunnel'
  } else if (p === 'mixed' || p === 'socks' || p === 'http') {
    localInboundType.value = 'forward'
  } else if (p === 'vless' && (localInboundType.value === 'tunnel' || localInboundType.value === 'forward')) {
    localInboundType.value = 'user'
  }
  enforceCapabilityMatrix()
  syncFormToJson()
}

function onNetworkChange(net: any) {
  localNetwork.value = String(net || 'tcp') as InboundNetwork
  enforceCapabilityMatrix()
  syncFormToJson()
}

function onSecurityChange(sec: any) {
  localTlsType.value = String(sec || 'none') as InboundSecurity
  if (localTlsType.value === 'reality' && !realityForm.value.private_key) {
    autoGenRealityKey()
  }
  enforceCapabilityMatrix()
  syncFormToJson()
}

async function autoGenRealityKey() {
  try {
    const { data } = await getXrayKeys()
    if (data.code === 0 && data.data) {
      realityForm.value.private_key = data.data.private_key
      realityForm.value.public_key = data.data.public_key
    }
  } catch {
    /* ignore */
  }
}

// -----------------------------------------------------------------------------
// 序列化与反序列化
// -----------------------------------------------------------------------------
function buildSettingsJSON(): string {
  if (localProtocol.value === 'dokodemo-door') {
    return JSON.stringify({
      address: dokodemoForm.value.target_address || '',
      port: dokodemoForm.value.target_port || 0,
      network: dokodemoForm.value.network || 'tcp,udp',
      followRedirect: false,
    })
  }

  if (localProtocol.value === 'mixed' || localProtocol.value === 'socks') {
    const s: Record<string, any> = {
      auth: 'password',
      udp: socksHttpForm.value.udp,
    }
    return JSON.stringify(s)
  }

  if (localProtocol.value === 'http') {
    const s: Record<string, any> = {
      auth: 'password',
      allowTransparent: false,
    }
    return JSON.stringify(s)
  }

  if (localProtocol.value === 'hysteria') {
    return JSON.stringify({
      version: 2,
    })
  }

  // vless
  const s: Record<string, any> = {
    decryption: vlessForm.value.decryption && vlessForm.value.decryption !== 'none' ? vlessForm.value.decryption : 'none',
  }
  if (vlessForm.value.fallbacks.length > 0) {
    s.fallbacks = vlessForm.value.fallbacks.map((f) => ({
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
  if (localProtocol.value === 'dokodemo-door') {
    const s: Record<string, any> = {
      network: dokodemoForm.value.network || 'tcp,udp',
      security: 'none',
      proxy_protocol: dokodemoForm.value.proxy_protocol !== false,
    }
    if (tcpForm.value.acceptProxyProtocol) {
      s.acceptProxyProtocol = true
      s.tcpSettings = { acceptProxyProtocol: true }
    }
    return JSON.stringify(s)
  }

  if (localProtocol.value === 'hysteria') {
    const alpnArr = tlsForm.value.alpn ? tlsForm.value.alpn.split(',').map((x: string) => x.trim()).filter(Boolean) : ['h3']
    const t: Record<string, any> = {
      serverName: tlsForm.value.server_name || undefined,
      certificates: [{ certificateFile: tlsForm.value.cert_file, keyFile: tlsForm.value.key_file }],
    }
    if (alpnArr.length > 0) t.alpn = alpnArr
    if (tlsForm.value.min_version) t.minVersion = tlsForm.value.min_version
    if (tlsForm.value.max_version) t.maxVersion = tlsForm.value.max_version
    if (tlsForm.value.cipher_suites) t.cipherSuites = tlsForm.value.cipher_suites.split(',').map((x: string) => x.trim()).filter(Boolean)
    if (tlsForm.value.allow_insecure) t.allowInsecure = true

    const s: Record<string, any> = {
      network: 'hysteria',
      security: 'tls',
      tlsSettings: t,
      hysteriaSettings: {
        version: 2,
        udpIdleTimeout: hysteriaForm.value.udp_idle_timeout || 60,
      },
    }
    return JSON.stringify(s)
  }

  const s: Record<string, any> = {
    network: localNetwork.value,
    security: localTlsType.value,
  }

  if (tcpForm.value.acceptProxyProtocol || xhttpForm.value.acceptProxyProtocol) {
    s.acceptProxyProtocol = true
  }

  if (localNetwork.value === 'xhttp') {
    s.xhttpSettings = { mode: xhttpForm.value.mode || 'auto', path: xhttpForm.value.path || '/' }
    if (xhttpForm.value.host) s.xhttpSettings.host = xhttpForm.value.host
    if (xhttpForm.value.acceptProxyProtocol) s.xhttpSettings.acceptProxyProtocol = true
  } else if (localNetwork.value === 'tcp') {
    const tcpSettings: Record<string, any> = {}
    if (tcpForm.value.header_type !== 'none') {
      const hdr: Record<string, any> = { type: tcpForm.value.header_type }
      if (tcpForm.value.header_type === 'http') {
        hdr.request = {}
        if (tcpForm.value.request_host) hdr.request.headers = { Host: tcpForm.value.request_host.split(',').map((x: string) => x.trim()) }
        if (tcpForm.value.request_path) hdr.request.path = tcpForm.value.request_path.split(',').map((x: string) => x.trim())
      }
      tcpSettings.header = hdr
    }
    if (tcpForm.value.acceptProxyProtocol) {
      tcpSettings.acceptProxyProtocol = true
    }
    if (Object.keys(tcpSettings).length > 0) {
      s.tcpSettings = tcpSettings
    }
  }

  if (localTlsType.value === 'reality') {
    s.fingerprint = realityFingerprint.value || 'chrome'
    s.realitySettings = {
      show: false,
      dest: realityForm.value.dest,
      serverNames: realityForm.value.server_name ? realityForm.value.server_name.split(',').map((x: string) => x.trim()).filter(Boolean) : [],
      privateKey: realityForm.value.private_key,
      shortIds: realityForm.value.short_id ? realityForm.value.short_id.split(',').map((x: string) => x.trim()).filter(Boolean) : [],
    }
    if (realityForm.value.public_key) s.realitySettings.publicKey = realityForm.value.public_key
    if (realityForm.value.min_client_ver) s.realitySettings.minClientVer = realityForm.value.min_client_ver
    if (realityForm.value.max_client_ver) s.realitySettings.maxClientVer = realityForm.value.max_client_ver
    if (realityForm.value.max_time_diff > 0) s.realitySettings.maxTimeDiff = realityForm.value.max_time_diff
  } else if (localTlsType.value === 'tls') {
    const alpnArr = tlsForm.value.alpn ? tlsForm.value.alpn.split(',').map((x: string) => x.trim()).filter(Boolean) : []
    const t: Record<string, any> = {
      serverName: tlsForm.value.server_name || undefined,
      certificates: [{ certificateFile: tlsForm.value.cert_file, keyFile: tlsForm.value.key_file }],
    }
    if (alpnArr.length > 0) t.alpn = alpnArr
    if (tlsForm.value.min_version) t.minVersion = tlsForm.value.min_version
    if (tlsForm.value.max_version) t.maxVersion = tlsForm.value.max_version
    if (tlsForm.value.cipher_suites) t.cipherSuites = tlsForm.value.cipher_suites.split(',').map((x: string) => x.trim()).filter(Boolean)
    if (tlsForm.value.allow_insecure) t.allowInsecure = true
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
    tag: localTag.value,
    port: localPort.value,
    listen: localListen.value || '0.0.0.0',
    protocol: localProtocol.value,
    settings_json: JSON.parse(settingsJson),
    stream_settings: JSON.parse(streamJson),
    sniffing: sniffJson ? JSON.parse(sniffJson) : null,
  }, null, 2)
  jsonError.value = ''

  emit('update:modelValue', rawJsonText.value)
  emitChange(settingsJson, streamJson, sniffJson)
  isInternalUpdating.value = false
}

function emitChange(settingsJson?: string, streamJson?: string, sniffJson?: string) {
  emit('change', {
    settingsJson: typeof settingsJson === 'string' ? settingsJson : buildSettingsJSON(),
    streamSettings: typeof streamJson === 'string' ? streamJson : buildStreamSettingsJSON(),
    sniffing: typeof sniffJson === 'string' ? sniffJson : buildSniffingJSON(),
    protocol: localProtocol.value,
    port: localPort.value,
    tag: localTag.value,
    listen: localListen.value || '0.0.0.0',
    remark: localRemark.value,
    traffic_reset: localTrafficReset.value,
    type: localInboundType.value,
    flow: vlessForm.value.flow,
    ratio: localRatio.value,
    total_gb: localTotalGB.value,
    expiry_time: localExpiryTime.value,
    shareAddrStrategy: localShareStrategy.value,
    shareAddr: localShareAddr.value,
    sharePort: localSharePort.value,
    shareSecurity: localShareSecurity.value,
    shareSni: localShareSni.value,
    shareHost: localShareHost.value,
    sharePath: localSharePath.value,
    shareAllowInsecure: localShareAllowInsecure.value,
    layerId: localLayerId.value,
    targetInboundId: dokodemoForm.value.target_inbound_id,
    targetAddress: dokodemoForm.value.target_address,
    targetPort: dokodemoForm.value.target_port,
  })
}

function parseJsonToForm(str: string) {
  if (!str || !str.trim()) return
  try {
    const parsed = JSON.parse(str)
    jsonError.value = ''

    let s: any = parsed
    if (parsed.settings_json && typeof parsed.settings_json === 'string') {
      try {
        s = JSON.parse(parsed.settings_json)
      } catch { /* ignore */ }
    } else if (parsed.settings_json && typeof parsed.settings_json === 'object') {
      s = parsed.settings_json
    } else if (parsed.settings && typeof parsed.settings === 'object') {
      s = parsed.settings
    }

    let streamObj: Record<string, any> = {}
    const ssRaw: any = parsed.stream_settings ?? parsed.streamSettings
    if (typeof ssRaw === 'string') {
      try {
        streamObj = JSON.parse(ssRaw)
      } catch {
        streamObj = {}
      }
    } else if (ssRaw && typeof ssRaw === 'object') {
      streamObj = ssRaw
    }
    const realitySettings: any = streamObj.realitySettings
    const tlsSettings: any = streamObj.tlsSettings
    const xhttpSettings: any = streamObj.xhttpSettings
    const tcpSettings: any = streamObj.tcpSettings

    if (parsed.protocol && typeof parsed.protocol === 'string') {
      localProtocol.value = parsed.protocol as InboundProtocol
    } else if (props.protocol) {
      localProtocol.value = props.protocol as InboundProtocol
    }
    if (parsed.network && typeof parsed.network === 'string') {
      localNetwork.value = parsed.network as InboundNetwork
    }
    if (parsed.tls_type && typeof parsed.tls_type === 'string') {
      localTlsType.value = parsed.tls_type as InboundSecurity
    } else if (streamObj.security) {
      localTlsType.value = streamObj.security as InboundSecurity
    }

    // vlessenc 回显判断（迁移至协议层 decryption）
    const decRaw = (s && typeof s === 'object' ? s.decryption : '') as string
    const parsedDec = (typeof decRaw === 'string' && decRaw !== 'none') ? decRaw : ''
    vlessForm.value.decryption = parsedDec
    if ((localTlsType.value as string) === 'vlessenc') {
      localTlsType.value = 'none'
    }

    if (typeof parsed.port === 'number') localPort.value = parsed.port
    if (parsed.tag && typeof parsed.tag === 'string') localTag.value = parsed.tag
    if (typeof parsed.listen === 'string') localListen.value = parsed.listen
    if (typeof parsed.remark === 'string') localRemark.value = parsed.remark
    if (typeof parsed.traffic_reset === 'string') localTrafficReset.value = parsed.traffic_reset

    if (typeof parsed.flow === 'string') vlessForm.value.flow = parsed.flow
    if (typeof parsed.ratio === 'number') localRatio.value = parsed.ratio
    if (typeof parsed.total_gb === 'number') localTotalGB.value = parsed.total_gb
    if (typeof parsed.expiry_time === 'string' && parsed.expiry_time) localExpiryTime.value = parsed.expiry_time
    if (typeof parsed.share_addr_strategy === 'string') localShareStrategy.value = parsed.share_addr_strategy
    if (typeof parsed.share_addr === 'string') localShareAddr.value = parsed.share_addr
    if (typeof parsed.share_port === 'number') localSharePort.value = parsed.share_port
    if (typeof parsed.share_security === 'string') localShareSecurity.value = parsed.share_security
    if (typeof parsed.share_sni === 'string') localShareSni.value = parsed.share_sni
    if (typeof parsed.share_host === 'string') localShareHost.value = parsed.share_host
    if (typeof parsed.share_path === 'string') localSharePath.value = parsed.share_path
    if (typeof parsed.share_allow_insecure === 'boolean') localShareAllowInsecure.value = parsed.share_allow_insecure
    if (typeof parsed.layer_id === 'number') localLayerId.value = parsed.layer_id

    if (parsed.protocol === 'hysteria' || localProtocol.value === 'hysteria') {
      localProtocol.value = 'hysteria'
      if (parsed.type === 'relay') {
        localInboundType.value = 'relay'
      } else {
        localInboundType.value = 'user'
      }
    } else if (parsed.type === 'tunnel' || parsed.protocol === 'dokodemo-door' || localProtocol.value === 'dokodemo-door') {
      localInboundType.value = 'tunnel'
      localProtocol.value = 'dokodemo-door'
    } else if (parsed.type === 'forward' || parsed.protocol === 'mixed' || parsed.protocol === 'socks' || parsed.protocol === 'http' || localProtocol.value === 'mixed' || localProtocol.value === 'socks' || localProtocol.value === 'http') {
      localInboundType.value = 'forward'
      if (parsed.protocol === 'mixed' || localProtocol.value === 'mixed') {
        localProtocol.value = 'mixed'
      } else if (localProtocol.value !== 'socks' && localProtocol.value !== 'http') {
        localProtocol.value = (parsed.protocol === 'socks' || parsed.protocol === 'http') ? (parsed.protocol as InboundProtocol) : 'mixed'
      }
    } else if (parsed.type === 'relay') {
      localInboundType.value = 'relay'
      localProtocol.value = 'vless'
    } else if (parsed.type === 'user') {
      localInboundType.value = 'user'
      localProtocol.value = 'vless'
    }

    // 协议层回显
    if (localProtocol.value === 'hysteria') {
      const hySettings = streamObj.hysteriaSettings
      hysteriaForm.value = {
        udp_idle_timeout: typeof hySettings?.udpIdleTimeout === 'number' ? hySettings.udpIdleTimeout : 60,
      }
    } else if (localProtocol.value === 'dokodemo-door') {
      dokodemoForm.value = {
        target_mode: parsed.target_inbound_id ? 'inbound' : (s?.address || parsed.target_address || s?.port || parsed.target_port ? 'manual' : 'draft'),
        target_inbound_id: parsed.target_inbound_id,
        target_address: s?.address || parsed.target_address || '',
        target_port: s?.port || parsed.target_port || 0,
        network: s?.network || 'tcp,udp',
        proxy_protocol: streamObj.proxy_protocol !== false && streamObj.proxyProtocol !== false,
      }
    } else if (localProtocol.value === 'mixed' || localProtocol.value === 'socks' || localProtocol.value === 'http') {
      socksHttpForm.value = {
        auth: 'password',
        udp: s?.udp !== false,
        accounts: Array.isArray(s?.accounts) ? s.accounts : [],
      }
    } else {
      vlessForm.value = {
        flow: parsed.flow || '',
        decryption: parsedDec,
        fallbacks: Array.isArray(s?.fallbacks)
          ? s.fallbacks.map((f: any) => ({
              name: f.name || '',
              alpn: f.alpn || '',
              path: f.path || '',
              dest: String(f.dest || '80'),
              xver: f.xver || 0,
            }))
          : [],
      }
    }

    // 传输层回显
    const ap = !!streamObj.acceptProxyProtocol || !!tcpSettings?.acceptProxyProtocol || !!xhttpSettings?.acceptProxyProtocol
    if (tcpSettings && typeof tcpSettings === 'object') {
      const req = tcpSettings.header?.request
      tcpForm.value = {
        header_type: tcpSettings.header?.type || 'none',
        request_host: Array.isArray(req?.headers?.Host) ? req.headers.Host.join(',') : '',
        request_path: Array.isArray(req?.path) ? req.path.join(',') : '/',
        acceptProxyProtocol: ap,
      }
    } else {
      tcpForm.value.acceptProxyProtocol = ap
    }
    if (xhttpSettings && typeof xhttpSettings === 'object') {
      xhttpForm.value = {
        mode: xhttpSettings.mode || 'auto',
        path: xhttpSettings.path || '/',
        host: xhttpSettings.host || '',
        acceptProxyProtocol: ap,
      }
    } else {
      xhttpForm.value.acceptProxyProtocol = ap
    }

    // 安全层回显
    if (realitySettings && typeof realitySettings === 'object') {
      realityForm.value = {
        dest: realitySettings.dest || '',
        server_name: Array.isArray(realitySettings.serverNames) ? realitySettings.serverNames[0] || '' : realitySettings.serverName || '',
        private_key: realitySettings.privateKey || '',
        public_key: realitySettings.publicKey || '',
        short_id: Array.isArray(realitySettings.shortIds) ? realitySettings.shortIds[0] || '' : realitySettings.shortId || '',
        min_client_ver: realitySettings.minClientVer || '',
        max_client_ver: realitySettings.maxClientVer || '',
        max_time_diff: realitySettings.maxTimeDiff || 0,
      }
      realityFingerprint.value = streamObj.fingerprint || 'chrome'
    }

    if (tlsSettings && typeof tlsSettings === 'object') {
      const cert = Array.isArray(tlsSettings.certificates) ? tlsSettings.certificates[0] : undefined
      tlsForm.value = {
        cert_id: parsed.cert_id || 0,
        server_name: tlsSettings.serverName || '',
        alpn: Array.isArray(tlsSettings.alpn) ? tlsSettings.alpn.join(',') : '',
        cert_file: cert?.certificateFile || '',
        key_file: cert?.keyFile || '',
        cipher_suites: Array.isArray(tlsSettings.cipherSuites) ? tlsSettings.cipherSuites.join(',') : '',
        min_version: tlsSettings.minVersion || '1.2',
        max_version: tlsSettings.maxVersion || '1.3',
        allow_insecure: !!tlsSettings.allowInsecure,
      }
    }

    // 嗅探回显
    const sniffRaw: any = parsed.sniffing ?? (s && typeof s === 'object' ? s.sniffing : undefined)
    let sniffObj: any = null
    if (typeof sniffRaw === 'string') {
      try { sniffObj = JSON.parse(sniffRaw) } catch { sniffObj = null }
    } else if (sniffRaw && typeof sniffRaw === 'object') {
      sniffObj = sniffRaw
    }
    if (sniffObj) {
      sniffingForm.enabled = !!sniffObj.enabled
      sniffingForm.destOverride = Array.isArray(sniffObj.destOverride) ? sniffObj.destOverride : ['http', 'tls', 'quic']
      sniffingForm.metadataOnly = !!sniffObj.metadataOnly
      sniffingForm.routeOnly = !!sniffObj.routeOnly
    }

    enforceCapabilityMatrix()
  } catch (e: any) {
    jsonError.value = e?.message || 'JSON 语法错误'
  }
}

function onRawJsonInput(val: string) {
  rawJsonText.value = val
  try {
    JSON.parse(val)
    jsonError.value = ''
    isInternalUpdating.value = true
    parseJsonToForm(val)
    isInternalUpdating.value = false
    emit('update:modelValue', val)
    emitChange()
  } catch (e: any) {
    jsonError.value = e?.message || 'JSON 语法错误'
  }
}

function formatRawJson() {
  try {
    const obj = JSON.parse(rawJsonText.value)
    rawJsonText.value = JSON.stringify(obj, null, 2)
    jsonError.value = ''
    ElMessage.success('已格式化 JSON')
  } catch (e: any) {
    jsonError.value = e?.message || 'JSON 语法错误'
  }
}

// -----------------------------------------------------------------------------
// 对外层与 Caddy 反代
// -----------------------------------------------------------------------------
const activeLayer = computed(() => layers.value.find((l) => l.id === localLayerId.value) || null)

const caddySnippet = computed(() => {
  const domain = activeLayer.value ? activeLayer.value.host : localShareSni.value || localShareAddr.value || 'node.example.com'
  const path = localSharePath.value || xhttpForm.value.path || '/xhttp-stream'
  const port = localPort.value || 10086
  return `${domain} {\n    # 前置 TLS 解密并反代到本地明文 Xray xhttp 入站\n    reverse_proxy ${path} 127.0.0.1:${port}\n}`
})

async function loadLayers() {
  if (!props.serverId) {
    layers.value = []
    return
  }
  try {
    const { data } = await getLayers(props.serverId)
    if (data.code === 0) layers.value = data.data.items
  } catch {
    layers.value = []
  }
}

watch(() => props.serverId, loadLayers, { immediate: true })

async function deleteLayerFromEditor() {
  const l = activeLayer.value
  if (!l || !props.serverId) return
  const count = l.inbound_count ?? 1
  try {
    await ElMessageBox.confirm(
      `删除对外接入层「${l.name}」？该层当前挂载 ${count} 个入站，删除后均回退为直连端点。`,
      '删除对外接入层',
      { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  layerDeleting.value = true
  try {
    const { data } = await deleteLayer(props.serverId, l.id)
    if (data.code === 0) {
      layers.value = layers.value.filter((x) => x.id !== l.id)
      localLayerId.value = 0
      ElMessage.success('对外层已删除')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  } finally {
    layerDeleting.value = false
  }
}

function openLayerQuick() {
  layerQuickForm.name = ''
  layerQuickForm.host = ''
  layerQuickForm.port = 443
  layerQuickForm.security = 'tls'
  layerQuickOpen.value = true
}

async function saveLayerQuick() {
  if (!props.serverId) return
  if (!layerQuickForm.name.trim() || !layerQuickForm.host.trim()) {
    ElMessage.warning('请填写层名称与对外 Host')
    return
  }
  layerQuickSaving.value = true
  try {
    const { data } = await createLayer(props.serverId, {
      name: layerQuickForm.name.trim(),
      host: layerQuickForm.host.trim(),
      port: layerQuickForm.port || 443,
      security: layerQuickForm.security,
    })
    if (data.code === 0) {
      layers.value = [...layers.value, data.data.layer]
      localLayerId.value = data.data.layer.id
      layerQuickOpen.value = false
      ElMessage.success('对外层已创建并绑定')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建失败'))
  } finally {
    layerQuickSaving.value = false
  }
}

function applyCaddyOffloadPreset() {
  localNetwork.value = 'xhttp'
  localTlsType.value = 'none'
  localListen.value = '127.0.0.1'
  if (localPort.value === 443) {
    localPort.value = 10086
  }
  if (!localSharePath.value) {
    localSharePath.value = '/xhttp-stream'
    xhttpForm.value.path = '/xhttp-stream'
  }
  xhttpForm.value.mode = 'auto'
  tcpForm.value.acceptProxyProtocol = false
  xhttpForm.value.acceptProxyProtocol = false

  if (props.serverId) {
    const domain = localShareSni.value || localShareAddr.value || 'node.example.com'
    createLayer(props.serverId, { name: '反代层', host: domain, port: 443, security: 'tls' })
      .then(({ data }) => {
        if (data.code === 0) {
          layers.value = [...layers.value, data.data.layer]
          localLayerId.value = data.data.layer.id
          ElMessage.success('已创建对外层「反代层」并绑定（TLS 卸载 + xhttp 传输）')
        } else {
          ElMessage.error(data.message)
        }
      })
      .catch((e) => ElMessage.error(errMsg(e, '创建对外层失败')))
    return
  }
  localShareStrategy.value = 'custom'
  localSharePort.value = 443
  localShareSecurity.value = 'tls'
  if (!localShareSni.value && !localShareAddr.value) {
    localShareAddr.value = 'node.example.com'
  }
  ElMessage.success('已应用反代预设')
}

// -----------------------------------------------------------------------------
// 深度监听所有表单变动并同步
// -----------------------------------------------------------------------------
watch(
  [
    localProtocol,
    localNetwork,
    localTlsType,
    localPort,
    localTag,
    localListen,
    localRemark,
    localTrafficReset,
    localInboundType,
    localRatio,
    localTotalGB,
    localExpiryTime,
    localShareStrategy,
    localShareAddr,
    localSharePort,
    localShareSecurity,
    localShareSni,
    localShareHost,
    localSharePath,
    localShareAllowInsecure,
    localLayerId,
    vlessForm,
    dokodemoForm,
    socksHttpForm,
    hysteriaForm,
    tcpForm,
    xhttpForm,
    realityForm,
    realityFingerprint,
    tlsForm,
    sniffingForm,
  ],
  () => {
    syncFormToJson()
  },
  { deep: true },
)

onMounted(() => {
  if (props.modelValue && props.modelValue !== '{}') {
    parseJsonToForm(props.modelValue)
  }
  syncFormToJson()
})

watch(
  () => props.modelValue,
  (newVal) => {
    if (newVal && newVal !== '{}' && newVal !== rawJsonText.value && !isInternalUpdating.value) {
      parseJsonToForm(newVal)
      syncFormToJson()
    }
  },
)

watch(
  () => props.protocol,
  (proto) => {
    if (proto && proto !== localProtocol.value) {
      onProtocolChange(proto)
    }
  },
)
</script>

<template>
  <div class="inbound-editor">
    <!-- 顶部导航：各层配置 Tab 与 高级配置 (JSON) -->
    <div class="editor-top-nav">
      <el-radio-group v-model="activeTab" size="small" class="category-tabs">
        <el-radio-button v-for="t in visibleTabs" :key="t.key" :value="t.key">
          {{ t.label }}
        </el-radio-button>
      </el-radio-group>
    </div>

    <!-- 表单视图容器 -->
    <div class="form-body">
      <el-form label-position="top">
        <!-- ==================== 1. 基础配置 Tab ==================== -->
        <div v-show="activeTab === 'basic'" class="tab-pane">
          <div class="form-card">
            <div class="card-title">网络监听与基础参数</div>
            <div class="form-grid">
              <el-form-item label="入站标签 (Tag)">
                <el-input v-model="localTag" placeholder="如 Tokyo-01-VLESS" />
              </el-form-item>

              <el-form-item label="备注 (Remark)">
                <el-input v-model="localRemark" placeholder="如 深圳专线入口 / 办公内网" />
              </el-form-item>

              <el-form-item label="代理协议">
                <el-select v-model="localProtocol" style="width: 100%" @change="onProtocolChange">
                  <el-option label="VLESS" value="vless" />
                  <el-option label="Hysteria 2" value="hysteria" />
                  <el-option label="Dokodemo-door (四层直通/端口转发)" value="dokodemo-door" />
                  <el-option label="Mixed (SOCKS5 / HTTP 混合代理)" value="mixed" />
                </el-select>
              </el-form-item>

              <el-form-item v-if="localProtocol === 'vless' || localProtocol === 'hysteria'" label="入站用途">
                <el-radio-group v-model="localInboundType" class="type-segmented-group" @change="onTypeChange">
                  <el-radio-button value="user">用户接入点</el-radio-button>
                  <el-radio-button value="relay">内部中继链路</el-radio-button>
                </el-radio-group>
              </el-form-item>

              <el-form-item v-else-if="localProtocol === 'dokodemo-door'" label="入站用途">
                <el-input value="四层直通 (tunnel)" disabled />
              </el-form-item>

              <el-form-item v-else-if="localProtocol === 'mixed' || localProtocol === 'socks' || localProtocol === 'http'" label="入站用途">
                <el-input value="转发代理 (forward)" disabled />
              </el-form-item>

              <el-form-item label="监听端口">
                <el-input-number v-model="localPort" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>

              <el-form-item label="监听地址">
                <el-input v-model="localListen" placeholder="默认 0.0.0.0，本地反代填 127.0.0.1" />
              </el-form-item>

              <el-form-item label="流量倍率">
                <el-input-number v-model="localRatio" :min="0" :step="0.1" :precision="2" style="width: 100%" />
              </el-form-item>

              <el-form-item label="流量配额 (GB)">
                <el-input-number v-model="localTotalGB" :min="0" :step="10" :precision="0" style="width: 100%" />
              </el-form-item>

              <el-form-item label="流量重置周期">
                <el-select v-model="localTrafficReset" style="width: 100%">
                  <el-option label="不重置 (never)" value="never" />
                  <el-option label="每日重置 (daily)" value="daily" />
                  <el-option label="每周重置 (weekly)" value="weekly" />
                  <el-option label="每月重置 (monthly)" value="monthly" />
                </el-select>
              </el-form-item>

              <el-form-item label="到期时间">
                <el-date-picker
                  v-model="localExpiryTime"
                  type="datetime"
                  placeholder="留空为永久"
                  style="width: 100%"
                  value-format="YYYY-MM-DDTHH:mm:ssZ"
                />
              </el-form-item>
            </div>
          </div>
        </div>

        <!-- ==================== 2. 协议参数 Tab ==================== -->
        <div v-show="activeTab === 'protocol'" class="tab-pane">
          <div class="form-card">
            <div class="card-title">{{ localProtocol === 'hysteria' ? 'HYSTERIA 2' : localProtocol.toUpperCase() }} 协议参数</div>
            <VlessSettings
              v-if="localProtocol === 'vless'"
              v-model="vlessForm"
              :inbound-type="localInboundType"
              :internal-u-u-i-d="localInternalUUID"
              :inbound-id="props.inboundId"
              :allow-vision="allowVisionFlow"
              @internal-uuid-changed="(v) => emit('internal-uuid-changed', v)"
            />
            <HysteriaSettings
              v-else-if="localProtocol === 'hysteria'"
              v-model="hysteriaForm"
              :inbound-type="localInboundType"
              :internal-u-u-i-d="localInternalUUID"
              :inbound-id="props.inboundId"
              @internal-uuid-changed="(v) => emit('internal-uuid-changed', v)"
            />
            <DokodemoSettings
              v-else-if="localProtocol === 'dokodemo-door'"
              v-model="dokodemoForm"
              :current-inbound-id="props.inboundId"
              :listen-port="localPort"
              :listen-address="localListen"
            />
            <SocksHttpSettings
              v-else-if="localProtocol === 'mixed' || localProtocol === 'socks' || localProtocol === 'http'"
              v-model="socksHttpForm"
              :protocol="localProtocol"
            />
          </div>
        </div>

        <!-- ==================== 3. 传输层 Tab (仅限 VLESS) ==================== -->
        <div v-if="localProtocol === 'vless'" v-show="activeTab === 'transport'" class="tab-pane">
          <div class="form-card">
            <div class="card-title">底层传输协议 (Transport)</div>
            <el-form-item label="传输协议类型">
              <el-radio-group v-model="localNetwork" size="default" @change="onNetworkChange">
                <el-radio-button value="tcp">TCP (RAW)</el-radio-button>
                <el-radio-button v-if="allowedNetworks.includes('xhttp')" value="xhttp">xhttp (SplitHTTP)</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <div style="margin-top: 16px">
              <TcpTransport v-if="localNetwork === 'tcp'" v-model="tcpForm" />
              <XhttpTransport v-else-if="localNetwork === 'xhttp'" v-model="xhttpForm" />
            </div>
          </div>
        </div>

        <!-- ==================== 4. 安全层 Tab ==================== -->
        <div v-show="activeTab === 'security'" class="tab-pane">
          <div class="form-card">
            <div class="card-title">安全加密层 (Security)</div>
            <el-form-item label="安全协议类型">
              <el-radio-group v-model="localTlsType" size="default" @change="onSecurityChange">
                <el-radio-button v-if="allowedSecurities.includes('none')" value="none">无 (none)</el-radio-button>
                <el-radio-button v-if="allowedSecurities.includes('tls')" value="tls">TLS</el-radio-button>
                <el-radio-button v-if="allowedSecurities.includes('reality')" value="reality">Reality</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <div v-if="localProtocol === 'hysteria'" style="margin-bottom: 12px">
              <el-alert
                type="info"
                :closable="false"
                show-icon
                title="Hysteria 2 强制启用 TLS/QUIC 加密，不支持明文传输 (none)。"
              />
            </div>
            <div v-else-if="(localProtocol === 'mixed' || localProtocol === 'socks' || localProtocol === 'http') && localTlsType === 'tls'" style="margin-bottom: 12px">
              <el-alert
                type="warning"
                :closable="false"
                show-icon
                title="Mixed / SOCKS 套用 TLS 说明"
                description="Mixed 套用 TLS 后，客户端必须支持 HTTPS 代理（HTTP over TLS）或 SOCKS over TLS（如在 Clash / SwitchyOmega 中配置 HTTPS 代理）。普通仅支持明文 TCP 的 SOCKS5 客户端将无法建立连接。"
              />
            </div>

            <div style="margin-top: 16px">
              <TlsSecurity
                v-if="localTlsType === 'tls'"
                v-model="tlsForm"
                @update:cert-id="(id) => emit('update:certId', id)"
              />
              <RealitySecurity
                v-else-if="localTlsType === 'reality'"
                v-model="realityForm"
                v-model:fingerprint="realityFingerprint"
              />
              <div v-else>
                <el-alert
                  type="info"
                  :closable="false"
                  show-icon
                  title="明文传输 (none)"
                  description="当前未启用传输层加密保护。适用于前置反代（Caddy/Nginx TLS 卸载）或受信任局域网环境。"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== 5. 嗅探 Tab ==================== -->
        <div v-show="activeTab === 'sniffing'" class="tab-pane">
          <div class="form-card">
            <div class="card-title">流量嗅探 (Sniffing)</div>
            <div class="form-grid">
              <el-form-item label="启用嗅探">
                <el-switch
                  v-model="sniffingForm.enabled"
                  active-text="开启流量嗅探"
                />
              </el-form-item>

              <el-form-item label="嗅探目标协议">
                <el-checkbox-group
                  v-model="sniffingForm.destOverride"
                  :disabled="!sniffingForm.enabled"
                >
                  <el-checkbox value="http" label="HTTP" />
                  <el-checkbox value="tls" label="TLS" />
                  <el-checkbox value="quic" label="QUIC" />
                </el-checkbox-group>
              </el-form-item>

              <el-form-item label="仅提取元数据 (MetadataOnly)">
                <el-switch
                  v-model="sniffingForm.metadataOnly"
                  :disabled="!sniffingForm.enabled"
                  active-text="开启"
                />
              </el-form-item>

              <el-form-item label="仅用于路由 (RouteOnly)">
                <el-switch
                  v-model="sniffingForm.routeOnly"
                  :disabled="!sniffingForm.enabled"
                  active-text="开启"
                />
              </el-form-item>
            </div>
          </div>
        </div>

        <!-- ==================== 6. 订阅与分享 Tab (仅用户入站) ==================== -->
        <div v-if="localInboundType === 'user'" v-show="activeTab === 'share'" class="tab-pane">
          <BusinessSettings
            :inbound-type="localInboundType"
            :layer-id="localLayerId"
            :layers="layers"
            :server-id="props.serverId"
            :share-strategy="localShareStrategy"
            :share-addr="localShareAddr"
            :share-port="localSharePort"
            :share-security="localShareSecurity"
            :share-sni="localShareSni"
            :share-host="localShareHost"
            :share-path="localSharePath"
            :share-allow-insecure="localShareAllowInsecure"
            :caddy-snippet="caddySnippet"
            :layer-deleting="layerDeleting"
            @update:layer-id="(v) => localLayerId = v"
            @update:share-strategy="(v) => localShareStrategy = v"
            @update:share-addr="(v) => localShareAddr = v"
            @update:share-port="(v) => localSharePort = v"
            @update:share-security="(v) => localShareSecurity = v"
            @update:share-sni="(v) => localShareSni = v"
            @update:share-host="(v) => localShareHost = v"
            @update:share-path="(v) => localSharePath = v"
            @update:share-allow-insecure="(v) => localShareAllowInsecure = v"
            @open-layer-quick="openLayerQuick"
            @delete-layer="deleteLayerFromEditor"
            @apply-caddy-preset="applyCaddyOffloadPreset"
          />
        </div>

        <!-- ==================== 7. 高级配置 Tab (原始 Config JSON 编辑器) ==================== -->
        <div v-show="activeTab === 'advanced'" class="tab-pane">
          <div class="form-card">
            <div class="card-head-flex" style="margin-bottom: 12px">
              <div class="card-title" style="margin-bottom: 0">
                <span>入站底层配置 (Config JSON)</span>
              </div>
              <div style="display: flex; align-items: center; gap: 10px">
                <el-tag v-if="!jsonError" type="success" size="small">
                  <el-icon><Check /></el-icon>&nbsp;语法正确
                </el-tag>
                <el-tag v-else type="danger" size="small">
                  <el-icon><Warning /></el-icon>&nbsp;{{ jsonError }}
                </el-tag>
                <el-button size="small" plain @click="formatRawJson">
                  <el-icon><MagicStick /></el-icon>&nbsp;格式化
                </el-button>
              </div>
            </div>
            <div class="json-editor-wrap">
              <CodeEditor
                :model-value="rawJsonText"
                language="json"
                height="480px"
                @update:model-value="onRawJsonInput"
              />
            </div>
          </div>
        </div>
      </el-form>
    </div>
  </div>

  <!-- 快捷新建对外接入层弹窗 -->
  <el-dialog v-model="layerQuickOpen" title="新建对外接入层" width="480px" append-to-body>
    <el-form label-position="top">
      <el-alert
        title="对外接入层定义订阅对外 Host / 端口 / 安全层，创建后将绑定至当前入站。"
        type="info"
        :closable="false"
        style="margin-bottom: 16px"
      />
      <el-form-item label="层名称" required>
        <el-input v-model="layerQuickForm.name" placeholder="如 HK 443 反代层" />
      </el-form-item>
      <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0 16px">
        <el-form-item label="对外 Host" required>
          <el-input v-model="layerQuickForm.host" placeholder="域名或 IP，如 hk.edge.example.com" />
        </el-form-item>
        <el-form-item label="对外端口">
          <el-input-number v-model="layerQuickForm.port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
      </div>
      <el-form-item label="对外安全层" style="margin-bottom: 0">
        <el-select v-model="layerQuickForm.security" style="width: 100%">
          <el-option label="TLS (默认)" value="tls" />
          <el-option label="明文 (none)" value="none" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="layerQuickOpen = false">取消</el-button>
      <el-button type="primary" :loading="layerQuickSaving" @click="saveLayerQuick">创建并绑定</el-button>
    </template>
  </el-dialog>
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
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.type-segmented-group {
  width: 100%;
  display: flex !important;

  :deep(.el-radio-button) {
    flex: 1;
    display: flex;

    .el-radio-button__inner {
      width: 100%;
      padding: 8px 4px;
      font-size: 13px;
      text-align: center;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
}

.json-editor-wrap {
  border: 1px solid var(--x-border);
  border-radius: 6px;
  overflow: hidden;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
