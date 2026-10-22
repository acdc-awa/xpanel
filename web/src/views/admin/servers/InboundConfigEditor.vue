<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import {
  Check,
  Warning,
  MagicStick,
  InfoFilled,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
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
  SockoptConfig,
  RealityConfig,
  VlessConfig,
  DokodemoConfig,
  HysteriaConfig,
} from './inbound/types'
import TcpTransport from './inbound/transports/TcpTransport.vue'
import XhttpTransport from './inbound/transports/XhttpTransport.vue'
import HysteriaFinalMask from './inbound/transports/HysteriaFinalMask.vue'
import SockoptSettings from './inbound/transports/SockoptSettings.vue'
import RealitySecurity from './inbound/securities/RealitySecurity.vue'
import TlsSecurity from './inbound/securities/TlsSecurity.vue'
import VlessSettings from './inbound/protocols/VlessSettings.vue'
import DokodemoSettings from './inbound/protocols/DokodemoSettings.vue'
import HysteriaSettings from './inbound/protocols/HysteriaSettings.vue'
import BusinessSettings from './inbound/common/BusinessSettings.vue'
import { validateHysteria, validateXhttp } from './inbound/validate'

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
  /** 表单级校验错误（如 xhttp 区间非法、xmux 互斥冲突）；父组件保存前须拦截。 */
  validationError?: string
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

const hysteriaForm = ref<HysteriaConfig>({
  udp_idle_timeout: 60,
  obfs_type: 'none',
  obfs_password: '',
  obfs_min_packet: 512,
  obfs_max_packet: 1200,
  bw_up: '',
  bw_down: '',
  port_hop_mode: 'intervalRemote',
  port_hop_ports: '',
  port_hop_interval: '',
  masquerade: {
    type: '404',
    dir: '',
    url: '',
    rewriteHost: false,
    xForwarded: false,
    insecure: false,
    content: '',
    statusCode: undefined,
  },
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
  xPaddingBytes: '',
  sessionIDPlacement: '',
  sessionIDKey: '',
  sessionIDLength: '',
  sessionIDTable: '',
  paddingObfsMode: false,
  paddingKey: '',
  paddingHeader: '',
  paddingPlacement: '',
  paddingMethod: '',
  noSSEHeader: false,
  scMaxBufferedPosts: undefined,
  scStreamUpServerSecs: '',
  serverMaxHeaderBytes: undefined,
  xmuxEnabled: false,
  xmuxMaxConnections: '3',
  xmuxMaxConcurrency: '',
  xmuxMaxRequestTimes: '600-900',
  xmuxMaxReusableSecs: '1800-3000',
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
  // 不再预填路径：未绑定托管证书时会写出节点上不存在的文件，导致 xray 加载失败。
  cert_file: '',
  key_file: '',
  cipher_suites: '',
  min_version: '1.2',
  max_version: '1.3',
  allow_insecure: false,
  reject_unknown_sni: false,
  legacy_verify_certs: [],
})

// 底层 Socket 优化状态
const sockoptForm = ref<SockoptConfig>({
  enabled: false,
  tcp_fast_open: false,
  tcp_fast_open_qlen: undefined,
  tcp_congestion: '',
  tcp_mptcp: false,
  tcp_keep_alive_idle: undefined,
  tcp_keep_alive_interval: undefined,
  tcp_user_timeout: undefined,
  accept_proxy_protocol: false,
  mark: undefined,
  tproxy: 'off',
})

// 嗅探状态
const sniffingForm = reactive({
  enabled: false,
  destOverride: ['http', 'tls', 'quic'],
  metadataOnly: false,
  routeOnly: false,
})
// sniffing 下表单未覆盖的键（如 domainsExcluded），保存时原样回写。
const sniffingExtra = ref<Record<string, any>>({})

// 表单未覆盖的 streamSettings 顶层键（如 VLESS finalmask、externalProxy 等）。
// 编辑保存时原样回写，避免手改配置被静默丢弃。
const streamExtra = ref<Record<string, any>>({})

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
  return ['tcp', 'xhttp']
})

const allowedSecurities = computed<InboundSecurity[]>(() => {
  if (localProtocol.value === 'hysteria') return ['tls']
  if (localProtocol.value === 'dokodemo-door') return ['none']
  return ['none', 'tls', 'reality']
})

// Tab 顺序严格对应入站 JSON 的结构：
// 顶层四字段 → settings → streamSettings(传输 + finalmask + sockopt) → streamSettings(security)
// → sniffing → 面板业务字段（不属于 xray 配置）→ 原始 JSON。
const visibleTabs = computed(() => {
  return [
    { key: 'basic', label: '基础' },
    { key: 'settings', label: 'settings' },
    { key: 'transport', label: '传输' },
    { key: 'security', label: '安全' },
    { key: 'sniffing', label: '嗅探' },
    { key: 'business', label: '业务与订阅' },
    { key: 'advanced', label: '原始 JSON' },
  ]
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

function onProtocolChange(proto: any) {
  const p = String(proto) as InboundProtocol
  // 切协议后旧协议的流层附加键不再适用（例如 vless 的 finalmask 不属于 hy2 表单）。
  if (p !== localProtocol.value) streamExtra.value = {}
  localProtocol.value = p
  if (p === 'hysteria') {
    localNetwork.value = 'hysteria'
    localTlsType.value = 'tls'
    if (localInboundType.value === 'tunnel' || localInboundType.value === 'forward') {
      localInboundType.value = 'user'
    }
  } else if (p === 'dokodemo-door') {
    localInboundType.value = 'tunnel'
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
    // 目标由派生 freedom 出站的 redirect 承担（字段声明明确禁止 settings.address/port），
    // 入站 settings 只保留转发网络与 followRedirect。
    return JSON.stringify({
      network: dokodemoForm.value.network || 'tcp,udp',
      followRedirect: false,
    })
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

function buildTlsSettingsObject(defaultAlpn?: string[]): Record<string, any> {
  const alpnArr = tlsForm.value.alpn
    ? tlsForm.value.alpn.split(',').map((x: string) => x.trim()).filter(Boolean)
    : (defaultAlpn || [])

  const t: Record<string, any> = {
    serverName: tlsForm.value.server_name || undefined,
  }

  const certsList: any[] = []
  if (tlsForm.value.cert_file || tlsForm.value.key_file) {
    certsList.push({
      certificateFile: tlsForm.value.cert_file || undefined,
      keyFile: tlsForm.value.key_file || undefined,
      usage: 'encipherment',
    })
  }
  // 存量 usage=verify 项原样回写（客户端语义，服务端生成时由后端剥离）。
  certsList.push(...(tlsForm.value.legacy_verify_certs || []))
  if (certsList.length > 0) {
    t.certificates = certsList
  }

  if (alpnArr.length > 0) t.alpn = alpnArr
  if (tlsForm.value.min_version) t.minVersion = tlsForm.value.min_version
  if (tlsForm.value.max_version) t.maxVersion = tlsForm.value.max_version
  // xray 的 cipherSuites 是「: 分隔的字符串」；提交数组会让整份配置反序列化失败。
  if (tlsForm.value.cipher_suites) {
    const suites = tlsForm.value.cipher_suites.split(',').map((x: string) => x.trim()).filter(Boolean)
    if (suites.length > 0) t.cipherSuites = suites.join(':')
  }
  if (tlsForm.value.allow_insecure) t.allowInsecure = true
  if (tlsForm.value.reject_unknown_sni) t.rejectUnknownSni = true

  return t
}

function buildSockoptObject(): Record<string, any> | undefined {
  if (!sockoptForm.value.enabled) return undefined
  const so: Record<string, any> = {}
  if (sockoptForm.value.tcp_fast_open) {
    so.tcpFastOpen = sockoptForm.value.tcp_fast_open_qlen ? Number(sockoptForm.value.tcp_fast_open_qlen) : true
  }
  if (sockoptForm.value.tcp_congestion) {
    so.tcpCongestion = sockoptForm.value.tcp_congestion
  }
  if (sockoptForm.value.tcp_mptcp) {
    so.tcpMptcp = true
  }
  if (sockoptForm.value.tcp_keep_alive_idle !== undefined && sockoptForm.value.tcp_keep_alive_idle !== null) {
    so.tcpKeepAliveIdle = Number(sockoptForm.value.tcp_keep_alive_idle)
  }
  if (sockoptForm.value.tcp_keep_alive_interval !== undefined && sockoptForm.value.tcp_keep_alive_interval !== null) {
    so.tcpKeepAliveInterval = Number(sockoptForm.value.tcp_keep_alive_interval)
  }
  if (sockoptForm.value.tcp_user_timeout !== undefined && sockoptForm.value.tcp_user_timeout !== null) {
    so.tcpUserTimeout = Number(sockoptForm.value.tcp_user_timeout)
  }
  if (sockoptForm.value.accept_proxy_protocol) {
    so.acceptProxyProtocol = true
  }
  if (sockoptForm.value.mark !== undefined && sockoptForm.value.mark !== null) {
    so.mark = Number(sockoptForm.value.mark)
  }
  if (sockoptForm.value.tproxy && sockoptForm.value.tproxy !== 'off') {
    so.tproxy = sockoptForm.value.tproxy
  }
  return Object.keys(so).length > 0 ? so : undefined
}

// buildHysteriaMasquerade 生成 hysteriaSettings.masquerade。
// 404 是内核默认值，不写以保持配置干净；其余类型原样写入。
function buildHysteriaMasquerade(): Record<string, any> | undefined {
  const m = hysteriaForm.value.masquerade
  if (!m || m.type === '404') return undefined
  const out: Record<string, any> = { ...(m.extra || {}), type: m.type }
  switch (m.type) {
    case 'file':
      out.dir = m.dir.trim()
      break
    case 'proxy':
      out.url = m.url.trim()
      if (m.rewriteHost) out.rewriteHost = true
      if (m.xForwarded) out.xForwarded = true
      if (m.insecure) out.insecure = true
      break
    case 'string':
      out.content = m.content
      if (m.statusCode) out.statusCode = m.statusCode
      break
  }
  return out
}

// buildHysteriaFinalMask 由表单生成 streamSettings.finalmask。
// udphop 是纯客户端能力（xray 服务端 WrapPacketConnServer 直接报 "udphop: client only"），
// 这里仍写入 DB 作为订阅/机间出站的单源，生成服务端配置时由 sanitize 剥离。
function buildHysteriaFinalMask(): Record<string, any> | undefined {
  const fm: Record<string, any> = { ...(hysteriaForm.value.finalmaskExtra || {}) }
  const udp: any[] = []

  // 保留手改 JSON 里表单不认识的 udp 条目（rand/filter/自定义 mask 等）。
  const extraUdp = fm.udp
  if (Array.isArray(extraUdp)) {
    for (const item of extraUdp) {
      const type = String(item?.type || '').toLowerCase()
      if (type !== 'salamander' && type !== 'udphop') udp.push(item)
    }
  }
  delete fm.udp

  const obfs = hysteriaForm.value.obfs_type
  if (obfs !== 'none' && hysteriaForm.value.obfs_password) {
    const settings: Record<string, any> = { password: hysteriaForm.value.obfs_password }
    if (obfs === 'gecko') {
      // Int32Range 只接受 "min-max" 字符串或整数；对象会被 xray 拒绝加载。
      settings.packetSize = `${hysteriaForm.value.obfs_min_packet}-${hysteriaForm.value.obfs_max_packet}`
    }
    udp.push({ type: 'salamander', settings })
  }

  const ports = hysteriaForm.value.port_hop_ports.trim()
  if (ports) {
    const hop: Record<string, any> = {
      mode: hysteriaForm.value.port_hop_mode || 'intervalRemote',
      remotePorts: ports,
    }
    const interval = hysteriaForm.value.port_hop_interval.trim()
    if (hysteriaForm.value.port_hop_mode === 'intervalRemote' && interval) hop.interval = interval
    udp.push({ type: 'udphop', settings: hop })
  }

  if (udp.length > 0) fm.udp = udp

  const up = hysteriaForm.value.bw_up.trim()
  const down = hysteriaForm.value.bw_down.trim()
  if (up || down) {
    const qp: Record<string, any> = { ...((fm.quicParams as Record<string, any>) || {}) }
    if (up) qp.brutalUp = up
    else delete qp.brutalUp
    if (down) qp.brutalDown = down
    else delete qp.brutalDown
    fm.quicParams = qp
  } else if (fm.quicParams) {
    // 表单清空带宽时同步清掉这两个键，其余自定义 QUIC 参数保留。
    const qp = { ...(fm.quicParams as Record<string, any>) }
    delete qp.brutalUp
    delete qp.brutalDown
    if (Object.keys(qp).length > 0) fm.quicParams = qp
    else delete fm.quicParams
  }

  return Object.keys(fm).length > 0 ? fm : undefined
}

// buildXhttpSettings 由表单生成 xhttpSettings。表单没覆盖的键（headers/extra/downloadSettings/
// uplink*/seq* 等）从 rawExtra 原样回写，避免编辑保存后静默丢失手改配置。
function buildXhttpSettings(): Record<string, any> {
  const x: Record<string, any> = { ...(xhttpForm.value.rawExtra || {}) }
  x.mode = xhttpForm.value.mode || 'auto'
  x.path = xhttpForm.value.path || '/'
  if (xhttpForm.value.host) x.host = xhttpForm.value.host
  else delete x.host

  const setOrDelete = (key: string, val: string | number | boolean | undefined) => {
    if (val === undefined || val === '' || val === false) delete x[key]
    else x[key] = val
  }

  setOrDelete('xPaddingBytes', xhttpForm.value.xPaddingBytes.trim())
  setOrDelete('sessionIDPlacement', xhttpForm.value.sessionIDPlacement)
  setOrDelete('sessionIDKey', xhttpForm.value.sessionIDKey.trim())
  setOrDelete('sessionIDLength', xhttpForm.value.sessionIDLength.trim())
  setOrDelete('sessionIDTable', xhttpForm.value.sessionIDTable.trim())
  setOrDelete('xPaddingObfsMode', xhttpForm.value.paddingObfsMode)
  setOrDelete('xPaddingKey', xhttpForm.value.paddingKey.trim())
  setOrDelete('xPaddingHeader', xhttpForm.value.paddingHeader.trim())
  setOrDelete('xPaddingPlacement', xhttpForm.value.paddingPlacement)
  setOrDelete('xPaddingMethod', xhttpForm.value.paddingMethod)
  setOrDelete('noSSEHeader', xhttpForm.value.noSSEHeader)
  setOrDelete('scMaxBufferedPosts', xhttpForm.value.scMaxBufferedPosts)
  setOrDelete('scStreamUpServerSecs', xhttpForm.value.scStreamUpServerSecs.trim())
  setOrDelete('serverMaxHeaderBytes', xhttpForm.value.serverMaxHeaderBytes)

  if (xhttpForm.value.xmuxEnabled) {
    const xmux: Record<string, any> = {}
    const put = (key: string, raw: string) => {
      const v = raw.trim()
      if (v) xmux[key] = v
    }
    put('maxConnections', xhttpForm.value.xmuxMaxConnections)
    put('maxConcurrency', xhttpForm.value.xmuxMaxConcurrency)
    put('hMaxRequestTimes', xhttpForm.value.xmuxMaxRequestTimes)
    put('hMaxReusableSecs', xhttpForm.value.xmuxMaxReusableSecs)
    x.xmux = xmux
  } else {
    delete x.xmux
  }
  return x
}

function buildStreamSettingsJSON(): string {
  if (localProtocol.value === 'dokodemo-door') {
    const s: Record<string, any> = {
      ...(streamExtra.value || {}),
      security: 'none',
      // 面板私有键（xray 不识别）：直通派生 freedom 出站是否向目标发送 PPv2，
      // 由后端 TunnelSendProxyProtocol 消费；接收侧见 tcpSettings.acceptProxyProtocol。
      proxy_protocol: dokodemoForm.value.proxy_protocol !== false,
    }
    if (tcpForm.value.acceptProxyProtocol) {
      s.tcpSettings = { ...(s.tcpSettings as Record<string, any> || {}), acceptProxyProtocol: true }
    }
    const so = buildSockoptObject()
    if (so) s.sockopt = so
    return JSON.stringify(s)
  }

  if (localProtocol.value === 'hysteria') {
    const t = buildTlsSettingsObject(['h3'])
    // Hysteria 2 走 QUIC：服务端 ALPN 必须是 h3（留空内核才补 h3）。
    // 表单里的 ALPN 在 hy2 下被禁用，这里强制覆盖，顺带修复历史上写成 h2,http/1.1 的配置。
    t.alpn = ['h3']
    const s: Record<string, any> = {
      ...(streamExtra.value || {}),
      network: 'hysteria',
      security: 'tls',
      tlsSettings: t,
      hysteriaSettings: {
        ...(hysteriaForm.value.hysteriaSettingsExtra || {}),
        version: 2,
        udpIdleTimeout: hysteriaForm.value.udp_idle_timeout || 60,
      },
    }
    const masq = buildHysteriaMasquerade()
    if (masq) s.hysteriaSettings.masquerade = masq
    const fm = buildHysteriaFinalMask()
    if (fm) s.finalmask = fm
    const so = buildSockoptObject()
    if (so) s.sockopt = so
    return JSON.stringify(s)
  }

  const s: Record<string, any> = {
    ...(streamExtra.value || {}),
    network: localNetwork.value,
    security: localTlsType.value,
  }

  // PROXY Protocol 只对 tcp 入站生效（xhttp 无此字段）；接收侧统一写 tcpSettings /
  // sockopt，不再产出 xray 不认识的顶层 acceptProxyProtocol。

  if (localNetwork.value === 'xhttp') {
    s.xhttpSettings = buildXhttpSettings()
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
    if (tcpForm.value.acceptProxyProtocol || sockoptForm.value.accept_proxy_protocol) {
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
    s.tlsSettings = buildTlsSettingsObject()
  }

  const so = buildSockoptObject()
  if (so) s.sockopt = so

  return JSON.stringify(s)
}

function buildSniffingJSON(): string {
  if (!sniffingForm.enabled) return ''
  const known = new Set(['enabled', 'destOverride', 'metadataOnly', 'routeOnly'])
  const out: Record<string, any> = {}
  for (const [k, v] of Object.entries(sniffingExtra.value || {})) {
    if (!known.has(k)) out[k] = v
  }
  out.enabled = true
  out.destOverride = sniffingForm.destOverride
  out.metadataOnly = sniffingForm.metadataOnly
  out.routeOnly = sniffingForm.routeOnly
  return JSON.stringify(out)
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
    settings: JSON.parse(settingsJson),
    streamSettings: JSON.parse(streamJson),
    ...(sniffJson ? { sniffing: JSON.parse(sniffJson) } : {}),
  }, null, 2)
  jsonError.value = ''

  emit('update:modelValue', rawJsonText.value)
  emitChange(settingsJson, streamJson, sniffJson)
  isInternalUpdating.value = false
}

function formValidationError(): string {
  if (localProtocol.value === 'hysteria') {
    return validateHysteria(hysteriaForm.value).join('；')
  }
  if (localProtocol.value === 'vless' && localNetwork.value === 'xhttp') {
    return validateXhttp(xhttpForm.value).join('；')
  }
  return ''
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
    validationError: formValidationError(),
  })
}

// jsonRangeToString 把 xray 的 Int32Range 各种存量形态（数字/字符串/{from,to}）统一成
// 表单使用的 "a-b" 字符串；无法识别时返回空串。
function jsonRangeToString(v: any): string {
  if (typeof v === 'number') return String(v)
  if (typeof v === 'string') return v
  if (v && typeof v === 'object') {
    const from = v.from ?? v.From
    const to = v.to ?? v.To
    if (typeof from === 'number' && typeof to === 'number') {
      return from === to ? String(from) : `${from}-${to}`
    }
  }
  return ''
}

function packetSizeOf(v: any): { min: number; max: number } | null {
  if (v && typeof v === 'object') {
    const min = Number(v.from ?? v.From ?? 0)
    const max = Number(v.to ?? v.To ?? 0)
    if (min > 0 && max > 0) return { min, max }
    return null
  }
  const s = String(v ?? '').trim()
  if (!s) return null
  const parts = s.split('-').map((x) => Number(x.trim()))
  if (!Number.isFinite(parts[0])) return null
  const min = parts[0]
  const max = parts.length > 1 ? parts[1] : parts[0]
  return min > 0 && max > 0 ? { min, max } : null
}

function parseJsonToForm(str: string) {
  if (!str || !str.trim()) return
  try {
    const parsed = JSON.parse(str)
    jsonError.value = ''

    // 平台模型兼容两种形态：
    //   - 新形态（与 xray 一致）：settings / streamSettings / sniffing；
    //   - 旧形态（面板 DB 字段名）：settings_json / stream_settings。
    // 面板业务字段统一从 _panel 读取，兼容旧形态直接平铺在顶层。
    const panel: any = parsed._panel && typeof parsed._panel === 'object' ? parsed._panel : parsed

    let s: any = parsed
    const settingsRaw: any = parsed.settings ?? parsed.settings_json
    if (typeof settingsRaw === 'string') {
      try {
        s = JSON.parse(settingsRaw)
      } catch { /* ignore */ }
    } else if (settingsRaw && typeof settingsRaw === 'object') {
      s = settingsRaw
    }

    let streamObj: Record<string, any> = {}
    const ssRaw: any = parsed.streamSettings ?? parsed.stream_settings
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
    const panelNetwork = panel.network ?? parsed.network ?? streamObj.network
    if (typeof panelNetwork === 'string' && panelNetwork) {
      localNetwork.value = panelNetwork as InboundNetwork
    }
    const panelTlsType = panel.tls_type ?? parsed.tls_type
    if (typeof panelTlsType === 'string' && panelTlsType) {
      localTlsType.value = panelTlsType as InboundSecurity
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
    if (typeof panel.remark === 'string') localRemark.value = panel.remark
    if (typeof panel.traffic_reset === 'string') localTrafficReset.value = panel.traffic_reset

    if (typeof panel.flow === 'string') vlessForm.value.flow = panel.flow
    if (typeof panel.ratio === 'number') localRatio.value = panel.ratio
    if (typeof panel.total_gb === 'number') localTotalGB.value = panel.total_gb
    if (typeof panel.expiry_time === 'string' && panel.expiry_time) localExpiryTime.value = panel.expiry_time
    if (typeof panel.share_addr_strategy === 'string') localShareStrategy.value = panel.share_addr_strategy
    if (typeof panel.share_addr === 'string') localShareAddr.value = panel.share_addr
    if (typeof panel.share_port === 'number') localSharePort.value = panel.share_port
    if (typeof panel.share_security === 'string') localShareSecurity.value = panel.share_security
    if (typeof panel.share_sni === 'string') localShareSni.value = panel.share_sni
    if (typeof panel.share_host === 'string') localShareHost.value = panel.share_host
    if (typeof panel.share_path === 'string') localSharePath.value = panel.share_path
    if (typeof panel.share_allow_insecure === 'boolean') localShareAllowInsecure.value = panel.share_allow_insecure
    if (typeof panel.layer_id === 'number') localLayerId.value = panel.layer_id

    if (parsed.protocol === 'hysteria' || localProtocol.value === 'hysteria') {
      localProtocol.value = 'hysteria'
      localInboundType.value = 'user'
    } else if (panel.type === 'tunnel' || parsed.protocol === 'dokodemo-door' || localProtocol.value === 'dokodemo-door') {
      localInboundType.value = 'tunnel'
      localProtocol.value = 'dokodemo-door'
    } else {
      localInboundType.value = 'user'
      localProtocol.value = 'vless'
    }

    // streamSettings 里表单未覆盖的顶层键原样保留：
    // vless/dokodemo 的 finalmask、externalProxy 等目前没有表单，但手改后保存不能丢。
    const streamKnown = new Set([
      'network', 'security', 'sockopt', 'tlsSettings', 'realitySettings',
    ])
    if (localProtocol.value === 'vless') {
      for (const k of ['tcpSettings', 'xhttpSettings', 'fingerprint', 'acceptProxyProtocol']) streamKnown.add(k)
    } else if (localProtocol.value === 'dokodemo-door') {
      for (const k of ['tcpSettings', 'acceptProxyProtocol', 'proxy_protocol', 'proxyProtocol']) streamKnown.add(k)
    } else {
      for (const k of ['hysteriaSettings', 'finalmask']) streamKnown.add(k)
    }
    const streamExtraObj: Record<string, any> = {}
    for (const [k, v] of Object.entries(streamObj)) {
      if (!streamKnown.has(k)) streamExtraObj[k] = v
    }
    streamExtra.value = streamExtraObj

    // 协议层回显
    if (localProtocol.value === 'hysteria') {
      const hySettings = streamObj.hysteriaSettings
      const fm = streamObj.finalmask && typeof streamObj.finalmask === 'object' ? streamObj.finalmask : null
      const fmExtra: Record<string, any> = {}
      let obfsType: HysteriaConfig['obfs_type'] = 'none'
      let obfsPassword = ''
      let minPacket = 512
      let maxPacket = 1200
      let hopPorts = ''
      let hopInterval = ''
      let hopMode: HysteriaConfig['port_hop_mode'] = 'intervalRemote'
      if (fm) {
        for (const [k, v] of Object.entries(fm)) {
          if (k !== 'udp' && k !== 'quicParams') fmExtra[k] = v
        }
        const udp = Array.isArray(fm.udp) ? fm.udp : []
        const unknownUdp: any[] = []
        for (const item of udp) {
          const type = String(item?.type || '').toLowerCase()
          const settings = (item?.settings && typeof item.settings === 'object') ? item.settings : {}
          if (type === 'salamander') {
            obfsPassword = String(settings.password ?? '')
            const ps = packetSizeOf(settings.packetSize)
            if (ps) {
              obfsType = 'gecko'
              minPacket = ps.min
              maxPacket = ps.max
            } else {
              obfsType = 'salamander'
            }
          } else if (type === 'udphop') {
            const mode = String(settings.mode ?? '').toLowerCase()
            // 只有目标端口跳跃能下发订阅；intervalLocal 只跳源端口，保留在 extra 里不映射。
            if (mode.includes('intervalremote') || mode.includes('perconnremote')) {
              hopPorts = String(settings.remotePorts ?? '')
              hopInterval = jsonRangeToString(settings.interval)
              hopMode = mode.includes('perconnremote') ? 'perConnRemote' : 'intervalRemote'
            } else {
              unknownUdp.push(item)
            }
          } else {
            unknownUdp.push(item)
          }
        }
        if (unknownUdp.length > 0) fmExtra.udp = unknownUdp
        const qp = fm.quicParams && typeof fm.quicParams === 'object' ? { ...fm.quicParams } : null
        if (qp) {
          delete qp.brutalUp
          delete qp.brutalDown
          if (Object.keys(qp).length > 0) fmExtra.quicParams = qp
        }
      }
      const qp = fm?.quicParams && typeof fm.quicParams === 'object' ? fm.quicParams : {}

      // hysteriaSettings 下表单不认识的键（含旧配置）原样回写，避免保存丢配置。
      const hyExtra: Record<string, any> = {}
      const hyKnown = new Set(['version', 'udpIdleTimeout', 'auth', 'masquerade'])
      if (hySettings && typeof hySettings === 'object') {
        for (const [k, v] of Object.entries(hySettings)) {
          if (!hyKnown.has(k)) hyExtra[k] = v
        }
      }

      // 伪装站点回显
      const mq = hySettings?.masquerade && typeof hySettings.masquerade === 'object'
        ? hySettings.masquerade
        : null
      const mqKnown = new Set(['type', 'dir', 'url', 'rewriteHost', 'xForwarded', 'insecure', 'content', 'statusCode'])
      const mqExtra: Record<string, any> = {}
      if (mq) {
        for (const [k, v] of Object.entries(mq)) {
          if (!mqKnown.has(k)) mqExtra[k] = v
        }
      }
      const mqType = String(mq?.type ?? '404').toLowerCase()
      const masquerade: HysteriaConfig['masquerade'] = {
        type: (mqType === 'file' || mqType === 'proxy' || mqType === 'string' ? mqType : '404'),
        dir: String(mq?.dir ?? ''),
        url: String(mq?.url ?? ''),
        rewriteHost: !!mq?.rewriteHost,
        xForwarded: !!mq?.xForwarded,
        insecure: !!mq?.insecure,
        content: String(mq?.content ?? ''),
        statusCode: typeof mq?.statusCode === 'number' ? mq.statusCode : undefined,
        extra: Object.keys(mqExtra).length > 0 ? mqExtra : undefined,
      }

      hysteriaForm.value = {
        udp_idle_timeout: typeof hySettings?.udpIdleTimeout === 'number' ? hySettings.udpIdleTimeout : 60,
        obfs_type: obfsType,
        obfs_password: obfsPassword,
        obfs_min_packet: minPacket,
        obfs_max_packet: maxPacket,
        bw_up: String(qp.brutalUp ?? ''),
        bw_down: String(qp.brutalDown ?? ''),
        port_hop_mode: hopMode,
        port_hop_ports: hopPorts,
        port_hop_interval: hopInterval,
        finalmaskExtra: Object.keys(fmExtra).length > 0 ? fmExtra : undefined,
        masquerade,
        hysteriaSettingsExtra: Object.keys(hyExtra).length > 0 ? hyExtra : undefined,
      }
    } else if (localProtocol.value === 'dokodemo-door') {
      const hasTargetPanel = typeof panel.target_inbound_id === 'number' ||
        typeof panel.target_address === 'string' ||
        typeof panel.target_port === 'number'
      const targetInboundId = hasTargetPanel ? panel.target_inbound_id : dokodemoForm.value.target_inbound_id
      const targetAddress = hasTargetPanel ? (panel.target_address || '') : dokodemoForm.value.target_address
      const targetPort = hasTargetPanel ? (panel.target_port || 0) : dokodemoForm.value.target_port
      dokodemoForm.value = {
        target_mode: hasTargetPanel
          ? (targetInboundId ? 'inbound' : (s?.address || targetAddress || s?.port || targetPort ? 'manual' : 'draft'))
          : dokodemoForm.value.target_mode,
        target_inbound_id: targetInboundId,
        target_address: s?.address || targetAddress,
        target_port: s?.port || targetPort,
        network: s?.network || 'tcp,udp',
        proxy_protocol: streamObj.proxy_protocol !== false && streamObj.proxyProtocol !== false,
      }
    } else {
      vlessForm.value = {
        flow: typeof panel.flow === 'string' ? panel.flow : vlessForm.value.flow,
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

    // 传输层回显（xhttpSettings.acceptProxyProtocol 是历史误存字段，xray 不识别，不再回显）
    const ap = !!streamObj.acceptProxyProtocol || !!tcpSettings?.acceptProxyProtocol
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
      const known = new Set([
        'mode', 'path', 'host',
        'xPaddingBytes', 'sessionIDPlacement', 'sessionIDKey', 'sessionIDLength', 'sessionIDTable',
        'xPaddingObfsMode', 'xPaddingKey', 'xPaddingHeader', 'xPaddingPlacement', 'xPaddingMethod',
        'noSSEHeader', 'scMaxBufferedPosts', 'scStreamUpServerSecs', 'serverMaxHeaderBytes', 'xmux',
        // 旧键名由 sanitize 提升到新键，不要留在 rawExtra 里重复写回。
        'sessionPlacement', 'sessionKey',
      ])
      const rawExtra: Record<string, any> = {}
      for (const [k, v] of Object.entries(xhttpSettings)) {
        if (!known.has(k)) rawExtra[k] = v
      }
      const xmux = xhttpSettings.xmux && typeof xhttpSettings.xmux === 'object' ? xhttpSettings.xmux : null
      const xmuxEnabled = !!xmux && Object.keys(xmux).length > 0
      xhttpForm.value = {
        mode: xhttpSettings.mode || 'auto',
        path: xhttpSettings.path || '/',
        host: xhttpSettings.host || '',
        xPaddingBytes: jsonRangeToString(xhttpSettings.xPaddingBytes),
        // 旧键名 sessionPlacement / sessionKey（xray-core #6258 前）也要能回显。
        sessionIDPlacement: (xhttpSettings.sessionIDPlacement || xhttpSettings.sessionPlacement || '') as XhttpConfig['sessionIDPlacement'],
        sessionIDKey: xhttpSettings.sessionIDKey || xhttpSettings.sessionKey || '',
        sessionIDLength: jsonRangeToString(xhttpSettings.sessionIDLength),
        sessionIDTable: String(xhttpSettings.sessionIDTable ?? ''),
        paddingObfsMode: !!xhttpSettings.xPaddingObfsMode,
        paddingKey: String(xhttpSettings.xPaddingKey ?? ''),
        paddingHeader: String(xhttpSettings.xPaddingHeader ?? ''),
        paddingPlacement: (xhttpSettings.xPaddingPlacement || '') as XhttpConfig['paddingPlacement'],
        paddingMethod: (xhttpSettings.xPaddingMethod || '') as XhttpConfig['paddingMethod'],
        noSSEHeader: !!xhttpSettings.noSSEHeader,
        scMaxBufferedPosts: typeof xhttpSettings.scMaxBufferedPosts === 'number' ? xhttpSettings.scMaxBufferedPosts : undefined,
        scStreamUpServerSecs: jsonRangeToString(xhttpSettings.scStreamUpServerSecs),
        serverMaxHeaderBytes: typeof xhttpSettings.serverMaxHeaderBytes === 'number' ? xhttpSettings.serverMaxHeaderBytes : undefined,
        xmuxEnabled,
        xmuxMaxConnections: xmuxEnabled ? (jsonRangeToString(xmux.maxConnections) || '3') : '3',
        xmuxMaxConcurrency: xmuxEnabled ? jsonRangeToString(xmux.maxConcurrency) : '',
        xmuxMaxRequestTimes: xmuxEnabled ? (jsonRangeToString(xmux.hMaxRequestTimes) || '600-900') : '600-900',
        xmuxMaxReusableSecs: xmuxEnabled ? (jsonRangeToString(xmux.hMaxReusableSecs) || '1800-3000') : '1800-3000',
        rawExtra: Object.keys(rawExtra).length > 0 ? rawExtra : undefined,
      }
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
      let certFile = ''
      let keyFile = ''
      const legacyVerifyCerts: any[] = []
      if (Array.isArray(tlsSettings.certificates)) {
        for (const c of tlsSettings.certificates) {
          if (c && typeof c === 'object') {
            if (c.usage === 'verify') {
              // 服务端不消费，仅原样保留，避免存量配置丢字段。
              legacyVerifyCerts.push(c)
            } else {
              if (c.certificateFile) certFile = c.certificateFile
              if (c.keyFile) keyFile = c.keyFile
            }
          }
        }
      }
      tlsForm.value = {
        cert_id: typeof panel.cert_id === 'number' ? panel.cert_id : tlsForm.value.cert_id,
        server_name: tlsSettings.serverName || '',
        alpn: Array.isArray(tlsSettings.alpn) ? tlsSettings.alpn.join(',') : '',
        cert_file: certFile,
        key_file: keyFile,
        // 兼容历史脏数据：数组（旧前端）与字符串（xray 正确形态）两种存法。
        cipher_suites: Array.isArray(tlsSettings.cipherSuites)
          ? tlsSettings.cipherSuites.join(',')
          : (typeof tlsSettings.cipherSuites === 'string' ? tlsSettings.cipherSuites.split(':').join(',') : ''),
        min_version: tlsSettings.minVersion || '1.2',
        max_version: tlsSettings.maxVersion || '1.3',
        allow_insecure: !!tlsSettings.allowInsecure,
        reject_unknown_sni: !!tlsSettings.rejectUnknownSni,
        legacy_verify_certs: legacyVerifyCerts,
      }
    }

    // Sockopt 回显
    const sockoptSettings: any = streamObj.sockopt
    if (sockoptSettings && typeof sockoptSettings === 'object') {
      const isTfoNumber = typeof sockoptSettings.tcpFastOpen === 'number'
      sockoptForm.value = {
        enabled: true,
        tcp_fast_open: !!sockoptSettings.tcpFastOpen,
        tcp_fast_open_qlen: isTfoNumber ? sockoptSettings.tcpFastOpen : undefined,
        tcp_congestion: sockoptSettings.tcpCongestion || sockoptSettings.tcpcongestion || '',
        tcp_mptcp: !!sockoptSettings.tcpMptcp,
        tcp_keep_alive_idle: typeof sockoptSettings.tcpKeepAliveIdle === 'number' ? sockoptSettings.tcpKeepAliveIdle : undefined,
        tcp_keep_alive_interval: typeof sockoptSettings.tcpKeepAliveInterval === 'number' ? sockoptSettings.tcpKeepAliveInterval : undefined,
        tcp_user_timeout: typeof sockoptSettings.tcpUserTimeout === 'number' ? sockoptSettings.tcpUserTimeout : undefined,
        accept_proxy_protocol: !!sockoptSettings.acceptProxyProtocol,
        mark: typeof sockoptSettings.mark === 'number' ? sockoptSettings.mark : undefined,
        tproxy: sockoptSettings.tproxy || 'off',
      }
    } else {
      sockoptForm.value = {
        enabled: false,
        tcp_fast_open: false,
        tcp_fast_open_qlen: undefined,
        tcp_congestion: '',
        tcp_mptcp: false,
        tcp_keep_alive_idle: undefined,
        tcp_keep_alive_interval: undefined,
        tcp_user_timeout: undefined,
        accept_proxy_protocol: false,
        mark: undefined,
        tproxy: 'off',
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
      const sniffKnown = new Set(['enabled', 'destOverride', 'metadataOnly', 'routeOnly'])
      const sniffExtra: Record<string, any> = {}
      for (const [k, v] of Object.entries(sniffObj)) {
        if (!sniffKnown.has(k)) sniffExtra[k] = v
      }
      sniffingExtra.value = sniffExtra
    } else {
      sniffingExtra.value = {}
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
    hysteriaForm,
    tcpForm,
    xhttpForm,
    realityForm,
    realityFingerprint,
    tlsForm,
    sniffingForm,
    sockoptForm,
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
        <!-- ==================== 1. 基础 Tab：只有入站 JSON 的四个顶层字段 ==================== -->
        <div v-show="activeTab === 'basic'" class="tab-pane">
          <div class="x-micro-card">
            <div class="x-micro-card__head">
              <div class="head-title">
                <span class="w-2 h-2 rounded-full bg-blue-500"></span>
                <span>入站顶层字段</span>
              </div>
              <div class="head-meta">listen / port / tag / protocol</div>
            </div>
            <div class="x-micro-card__body">
              <div class="form-grid-2">
                <el-form-item label="监听地址 (listen)">
                  <el-input v-model="localListen" placeholder="默认 0.0.0.0，本地反代填 127.0.0.1" class="font-mono-num" />
                </el-form-item>

                <el-form-item label="端口 (port)">
                  <el-input-number v-model="localPort" :min="1" :max="65535" style="width: 100%" class="font-mono-num" />
                </el-form-item>

                <el-form-item label="唯一标识 (tag)">
                  <el-input v-model="localTag" placeholder="如 Tokyo-01-VLESS" />
                </el-form-item>

                <el-form-item label="协议 (protocol)">
                  <el-select v-model="localProtocol" style="width: 100%" @change="onProtocolChange">
                    <el-option label="VLESS" value="vless" />
                    <el-option label="Hysteria 2" value="hysteria" />
                    <el-option label="Dokodemo-door (四层直通/端口转发)" value="dokodemo-door" />
                  </el-select>
                </el-form-item>
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== 2. settings（协议参数） Tab ==================== -->
        <div v-show="activeTab === 'settings'" class="tab-pane">
          <div v-if="localProtocol !== 'dokodemo-door'" class="compact-tip gray">
            <el-icon><InfoFilled /></el-icon>
            <span>settings.users 由绑定的接入点动态注入，这里只维护协议级参数。</span>
          </div>
          <VlessSettings
            v-if="localProtocol === 'vless'"
            v-model="vlessForm"
            :inbound-type="localInboundType"
            :internal-u-u-i-d="localInternalUUID"
            :inbound-id="props.inboundId"
            :allow-vision="allowVisionFlow"
          />
          <HysteriaSettings
            v-else-if="localProtocol === 'hysteria'"
            v-model="hysteriaForm"
            :inbound-type="localInboundType"
            :internal-u-u-i-d="localInternalUUID"
            :inbound-id="props.inboundId"
          />
          <DokodemoSettings
            v-else-if="localProtocol === 'dokodemo-door'"
            v-model="dokodemoForm"
            :current-inbound-id="props.inboundId"
            :listen-port="localPort"
            :listen-address="localListen"
          />
        </div>

        <!-- ==================== 3. 传输层 Tab：streamSettings 非 security 部分 ==================== -->
        <div v-show="activeTab === 'transport'" class="tab-pane">
          <div class="x-micro-card">
            <div class="x-micro-card__head">
              <div class="head-title">
                <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
                <span>传输方式 (streamSettings.network)</span>
              </div>
              <div class="head-meta">网络流封装</div>
            </div>
            <div class="x-micro-card__body">
              <el-form-item v-if="localProtocol === 'vless'" label="传输协议类型" style="margin-bottom: 0">
                <el-radio-group v-model="localNetwork" size="small" @change="onNetworkChange">
                  <el-radio-button value="tcp">TCP (RAW 推荐)</el-radio-button>
                  <el-radio-button v-if="allowedNetworks.includes('xhttp')" value="xhttp">xhttp (SplitHTTP)</el-radio-button>
                </el-radio-group>
              </el-form-item>
              <div v-else class="compact-tip blue">
                <el-icon><InfoFilled /></el-icon>
                <span v-if="localProtocol === 'hysteria'">Hysteria 协议（protocol=hysteria，代理协议）与 hysteria 传输（network=hysteria，魔改 QUIC）是两层；这里固定使用后者，加密固定为 TLS。</span>
                <span v-else>Dokodemo-door 固定使用 raw (tcp) 传输，监听与目标由 settings 决定。</span>
              </div>
              <div v-if="localProtocol === 'vless'" class="compact-tip gray" style="margin-top: 10px">
                <el-icon><InfoFilled /></el-icon>
                <span>finalmask 等未提供表单的 streamSettings 键可在「原始 JSON」中编辑，本表单会原样保留。</span>
              </div>
            </div>
          </div>

          <div class="compact-tip gray">
            <el-icon><InfoFilled /></el-icon>
            <span>
              层级顺序（由内向外）：协议 settings → 传输 network → 安全 security →
              finalmask（加密完成后的外层伪装 / QUIC 调参）→ sockopt（最底层 socket 行为）。
            </span>
          </div>

          <TcpTransport v-if="localProtocol === 'vless' && localNetwork === 'tcp'" v-model="tcpForm" />
          <XhttpTransport
            v-else-if="localProtocol === 'vless' && localNetwork === 'xhttp'"
            v-model="xhttpForm"
          />
          <HysteriaFinalMask v-else-if="localProtocol === 'hysteria'" v-model="hysteriaForm" />

          <SockoptSettings v-model="sockoptForm" />
        </div>

        <!-- ==================== 4. 安全层 Tab：streamSettings.security ==================== -->
        <div v-show="activeTab === 'security'" class="tab-pane">
          <div class="x-micro-card">
            <div class="x-micro-card__head">
              <div class="head-title">
                <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
                <span>安全加密协议 (streamSettings.security)</span>
              </div>
              <div class="head-meta">TLS / 伪装认证</div>
            </div>
            <div class="x-micro-card__body">
              <el-form-item label="安全协议类型" style="margin-bottom: 0">
                <el-radio-group v-model="localTlsType" size="small" @change="onSecurityChange">
                  <el-radio-button v-if="allowedSecurities.includes('none')" value="none">无加密 (none)</el-radio-button>
                  <el-radio-button v-if="allowedSecurities.includes('tls')" value="tls">标准 TLS</el-radio-button>
                  <el-radio-button v-if="allowedSecurities.includes('reality')" value="reality">REALITY 伪装</el-radio-button>
                </el-radio-group>
              </el-form-item>
            </div>
          </div>

          <div v-if="localProtocol === 'hysteria'" class="compact-tip blue">
            <el-icon><InfoFilled /></el-icon>
            <span>Hysteria 2 强制启用 TLS/QUIC，不支持明文传输。</span>
          </div>

          <TlsSecurity
            v-if="localTlsType === 'tls'"
            v-model="tlsForm"
            :alpn-fixed="localProtocol === 'hysteria' ? 'h3' : ''"
            @update:cert-id="(id) => emit('update:certId', id)"
          />
          <RealitySecurity
            v-else-if="localTlsType === 'reality'"
            v-model="realityForm"
            v-model:fingerprint="realityFingerprint"
          />
          <div v-else-if="localTlsType === 'none'" class="x-micro-card">
            <div class="x-micro-card__body">
              <div class="compact-tip gray">
                <el-icon><InfoFilled /></el-icon>
                <span>未启用传输层加密 (none)，适用于前置反代或受信任局域网。</span>
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== 5. 嗅探 Tab：inbound.sniffing ==================== -->
        <div v-show="activeTab === 'sniffing'" class="tab-pane">
          <div class="x-micro-card">
            <div class="x-micro-card__head">
              <div class="head-title">
                <span class="w-2 h-2 rounded-full bg-violet-500"></span>
                <span>流量嗅探 (sniffing)</span>
              </div>
              <div class="head-meta">
                <el-switch
                  v-model="sniffingForm.enabled"
                  active-text="启用流量嗅探"
                  size="small"
                />
              </div>
            </div>
            <div v-if="sniffingForm.enabled" class="x-micro-card__body">
              <div class="form-grid-2">
                <el-form-item label="嗅探目标协议" style="grid-column: span 2">
                  <el-checkbox-group v-model="sniffingForm.destOverride">
                    <el-checkbox value="http" label="HTTP 域名" />
                    <el-checkbox value="tls" label="TLS SNI" />
                    <el-checkbox value="quic" label="QUIC" />
                  </el-checkbox-group>
                </el-form-item>

                <el-form-item label="仅提取元数据 (metadataOnly)">
                  <el-switch
                    v-model="sniffingForm.metadataOnly"
                    active-text="仅提取不覆盖连接"
                  />
                </el-form-item>

                <el-form-item label="仅用于路由 (routeOnly)">
                  <el-switch
                    v-model="sniffingForm.routeOnly"
                    active-text="仅供出站路由分流决策"
                  />
                </el-form-item>
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== 6. 业务与订阅 Tab（面板字段，不写入 xray 配置） ==================== -->
        <div v-show="activeTab === 'business'" class="tab-pane">
          <div class="x-micro-card">
            <div class="x-micro-card__head">
              <div class="head-title">
                <span class="w-2 h-2 rounded-full bg-amber-500"></span>
                <span>备注与计费策略</span>
              </div>
              <div class="head-meta">面板业务字段</div>
            </div>
            <div class="x-micro-card__body">
              <div class="form-grid-2">
                <el-form-item label="备注 (Remark)" style="grid-column: span 2">
                  <el-input v-model="localRemark" placeholder="如 深圳专线入口 / 办公内网" />
                </el-form-item>

                <el-form-item label="流量倍率">
                  <el-input-number v-model="localRatio" :min="0" :step="0.1" :precision="2" style="width: 100%" class="font-mono-num" />
                </el-form-item>

                <el-form-item label="流量配额 (GB)">
                  <el-input-number v-model="localTotalGB" :min="0" :step="10" :precision="0" style="width: 100%" class="font-mono-num" />
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
                    placeholder="留空为永久有效"
                    style="width: 100%"
                    value-format="YYYY-MM-DDTHH:mm:ssZ"
                  />
                </el-form-item>
              </div>
            </div>
          </div>

          <BusinessSettings
            v-if="localInboundType === 'user'"
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

        <!-- ==================== 7. 原始 JSON Tab ==================== -->
        <div v-show="activeTab === 'advanced'" class="tab-pane">
          <div class="x-micro-card">
            <div class="x-micro-card__head">
              <div class="head-title">
                <span class="w-2 h-2 rounded-full bg-amber-500"></span>
                <span>入站底层配置 (Config JSON)</span>
              </div>
              <div class="head-meta" style="display: flex; align-items: center; gap: 8px">
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
            <div class="x-micro-card__body">
              <div class="compact-tip gray" style="margin-bottom: 10px">
                <el-icon><InfoFilled /></el-icon>
                <span>
                  这是面板存储的投影源（服务端与订阅共用）。生成服务端配置时会剥离
                  fingerprint、realitySettings.publicKey、tlsSettings.allowInsecure、
                  xhttpSettings.xmux / downloadSettings / noGRPCHeader 等客户端字段；
                  订阅侧仍使用它们。直通的 streamSettings.proxy_protocol 是面板私有键
                  （供派生 freedom 出站发送 PPv2），不会写进节点配置。
                </span>
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
          <el-input-number v-model="layerQuickForm.port" :min="1" :max="65535" style="width: 100%" class="font-mono-num" />
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
  gap: 12px;
}

.tab-pane {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.json-editor-wrap {
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  overflow: hidden;
}

.compact-tip {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--x-font-xs);
  padding: 8px 12px;
  border-radius: var(--x-radius);
  line-height: var(--x-leading-tight);

  .el-icon {
    font-size: 14px;
    flex: none;
  }

  &.blue {
    background: var(--x-primary-soft, rgba(59, 130, 246, 0.08));
    color: var(--x-primary, #3b82f6);
    border: 1px solid rgba(59, 130, 246, 0.2);
  }

  &.orange {
    background: rgba(245, 158, 11, 0.08);
    color: #d97706;
    border: 1px solid rgba(245, 158, 11, 0.25);
  }

  &.gray {
    background: var(--x-fill-2, rgba(0, 0, 0, 0.03));
    color: var(--x-text-3, #9ca3af);
    border: 1px solid var(--x-border-light, #e5e7eb);
  }
}
</style>
