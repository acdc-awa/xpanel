<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, nextTick } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsCheckboxGroup,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsModal,
  CsSegmentedControl,
  CsSelect,
  CsTabs,
  CsToggle,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import type { CheckboxGroupOption, SegmentedOption, TabItem } from '@/components/cloudscape'
import {
  getLayers,
  createLayer,
  deleteLayer,
  getXrayKeys,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import CodeEditor from '@/components/CodeEditor.vue'
import TipIcon from '@/components/base/TipIcon.vue'
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
  TunnelConfig,
  HysteriaConfig,
} from './inbound/types'
import TcpTransport from './inbound/transports/TcpTransport.vue'
import XhttpTransport from './inbound/transports/XhttpTransport.vue'
import HysteriaFinalMask from './inbound/transports/HysteriaFinalMask.vue'
import SockoptSettings from './inbound/transports/SockoptSettings.vue'
import RealitySecurity from './inbound/securities/RealitySecurity.vue'
import TlsSecurity from './inbound/securities/TlsSecurity.vue'
import VlessSettings from './inbound/protocols/VlessSettings.vue'
import TunnelForward from './inbound/transports/TunnelForward.vue'
import HysteriaSettings from './inbound/protocols/HysteriaSettings.vue'
import BusinessSettings from './inbound/common/BusinessSettings.vue'
import { validateHysteriaFinalMask, validateHysteriaSettings, validateXhttp } from './inbound/validate'

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
  shareSni: string
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
    inboundType?: string
    inboundId?: number
    targetInboundId?: number
    targetAddress?: string
    targetPort?: number
    certId?: number
    serverId?: number
    layerId?: number
    shareSni?: string
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
    inboundType: 'user',
    inboundId: 0,
    targetInboundId: undefined,
    targetAddress: '',
    targetPort: 0,
    certId: 0,
    serverId: 0,
    layerId: 0,
    shareSni: '',
    shareAllowInsecure: false,
  },
)

const emit = defineEmits<InboundEditorEmits & {
  (e: 'update:inboundType', value: string): void
  (e: 'update:certId', value: number): void
}>()

// 视图 Tab（默认落在「基础设置」：新建入站的第一件事是填 listen/port/tag/protocol）
const activeTab = ref('settings')

// 基础物理/业务类型
const localInboundType = ref(props.inboundType || 'user')

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
const localShareSni = ref(props.shareSni || '')
const localShareAllowInsecure = ref(props.shareAllowInsecure || false)
const localLayerId = ref(props.layerId || 0)

// 协议层状态对象（使用 ref 保证双向绑定更新）
const vlessForm = ref<VlessConfig>({
  flow: '',
  fallbacks: [],
  decryption: '',
})

const tunnelForm = ref<TunnelConfig>({
  // 草稿 = 引用入站但未选目标（target_mode 不入库，打开时按目标字段反推）。
  target_mode: props.targetInboundId ? 'inbound' : (props.targetAddress || props.targetPort ? 'manual' : 'inbound'),
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
  congestion: '',
  bbr_profile: '',
  max_idle_timeout: undefined,
  max_incoming_streams: undefined,
  init_stream_receive_window: undefined,
  max_stream_receive_window: undefined,
  init_connection_receive_window: undefined,
  max_connection_receive_window: undefined,
  disable_path_mtu_discovery: false,
  disable_gso: false,
  disable_stateless_reset: false,
  brutal_disable_loss_compensation: false,
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
  reject_unknown_sni: false,
  legacy_verify_certs: [],
  legacy_inline_certs: [],
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

// 入站 JSON 顶层未建模的键（如 allocate）。手写进「原始 JSON」后，
// 表单改动触发的重投影不能把它们丢掉。
const topExtra = ref<Record<string, any>>({})

// settings 里未建模的协议键（clients 由后端注入，不参与保留）。
const settingsExtra = ref<Record<string, any>>({})

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
// 「原始 JSON」是用户正在编辑的真源：解析完由父组件接收的提示，不能让随后的
// 深监听把整份文档按表单重投影（否则粘贴进来的未建模键会当场消失）。
let skipSyncAfterRawInput = false

// -----------------------------------------------------------------------------
// 能力矩阵联动规则 (Capability Matrix)
// -----------------------------------------------------------------------------
const allowedNetworks = computed<InboundNetwork[]>(() => {
  if (localProtocol.value === 'hysteria') return ['hysteria']
  if (localProtocol.value === 'tunnel') return ['tcp']
  return ['tcp', 'xhttp']
})

const allowedSecurities = computed<InboundSecurity[]>(() => {
  if (localProtocol.value === 'hysteria') return ['tls']
  if (localProtocol.value === 'tunnel') return ['none']
  return ['none', 'tls', 'reality']
})

// Tab 顺序严格对应入站 JSON 的结构：
// 顶层四字段 + 面板计费字段（基础设置）→ settings 里各协议独有的键（协议专属字段）
// → streamSettings(传输 + finalmask + sockopt) → streamSettings(security)
// → sniffing → 面板订阅字段（不属于 xray 配置）→ 原始 JSON。
const TAB_LABELS: Record<string, string> = {
  settings: '基础设置',
  protocol: '协议专属字段',
  transport: '传输设置',
  security: '安全',
  sniffing: '嗅探',
  business: '业务与订阅',
  advanced: '原始 JSON',
}

// 直通没有安全层、没有业务订阅页（tunnel 恒为 type=tunnel），
// 目标/转发网络/Proxy Protocol 全部并入传输页，页签只留四页。
const visibleTabIds = computed<string[]>(() => {
  if (localProtocol.value === 'tunnel') {
    return ['settings', 'transport', 'sniffing', 'advanced']
  }
  return ['settings', 'protocol', 'transport', 'security', 'sniffing', 'business', 'advanced']
})

const visibleTabs = computed<TabItem[]>(() => {
  const flagged = new Set(validationIssues.value.map((issue) => issue.tab))
  return visibleTabIds.value.map((id) => ({
    id,
    // 保存门禁的报错要能定位到具体页签，CsTabs 只支持 disabled，这里用标签后缀标记。
    label: flagged.has(id) ? `${TAB_LABELS[id]} ⚠` : TAB_LABELS[id],
  }))
})

// 保存前自检：只覆盖「后端一定会拒绝、或节点一定跑不起来」的硬条件。
// 每一项都带页签，父组件用拼接后的 validationError 拦截保存。
const validationIssues = computed<{ tab: string; message: string }[]>(() => {
  const issues: { tab: string; message: string }[] = []
  const push = (tab: string, messages: string[]) => {
    for (const message of messages) {
      if (message) issues.push({ tab, message })
    }
  }

  // 标签 / 端口为空由父组件保存前统一提示（两个入口共用同一句文案），这里只管配置级硬错误。
  if (localTlsType.value === 'reality') {
    push('security', [
      realityForm.value.dest.trim() ? '' : 'REALITY 必须填写借壳目标 (dest)',
      realityForm.value.server_name.trim() ? '' : 'REALITY 必须填写伪装 SNI (serverNames)',
      realityForm.value.private_key.trim() ? '' : 'REALITY 必须填写服务端私钥 (privateKey)',
    ])
  }
  if (localTlsType.value === 'tls' && !tlsForm.value.cert_id) {
    const pairedFiles = Boolean(tlsForm.value.cert_file.trim() && tlsForm.value.key_file.trim())
    const inlineCerts = (tlsForm.value.legacy_inline_certs || []).length > 0
    // Hysteria 2 强制 TLS，没有明文回退；vless+xhttp+none 的反代卸载场景不会走到这里。
    push('security', [pairedFiles || inlineCerts ? '' : 'TLS 入站必须绑定托管证书，或填写成对的证书/私钥文件路径'])
  }

  if (localProtocol.value === 'hysteria') {
    push('protocol', validateHysteriaSettings(hysteriaForm.value))
    push('transport', validateHysteriaFinalMask(hysteriaForm.value))
  } else if (localProtocol.value === 'vless' && localNetwork.value === 'xhttp') {
    push('transport', validateXhttp(xhttpForm.value))
  }

  if (localProtocol.value === 'tunnel') {
    const target = tunnelForm.value
    // 引用入站允许留空（草稿），画布连线后自动补全；自定义目标必须填完整。
    if (target.target_mode === 'manual') {
      push('transport', [
        target.target_address?.trim() ? '' : '请填写转发目标地址',
        target.target_port ? '' : '请填写转发目标端口',
      ])
    }
  }

  return issues
})

const protocolOptions = [
  { label: 'VLESS', value: 'vless' },
  { label: 'Hysteria 2', value: 'hysteria' },
  { label: 'Tunnel (四层直通/端口转发)', value: 'tunnel' },
]

const trafficResetOptions = [
  { label: '不重置 (never)', value: 'never' },
  { label: '每日重置 (daily)', value: 'daily' },
  { label: '每周重置 (weekly)', value: 'weekly' },
  { label: '每月重置 (monthly)', value: 'monthly' },
]

const layerSecurityOptions = [
  { label: 'TLS (默认)', value: 'tls' },
  { label: '明文 (none)', value: 'none' },
]

const sniffingTargetOptions: CheckboxGroupOption[] = [
  { value: 'http', label: 'HTTP 域名' },
  { value: 'tls', label: 'TLS SNI' },
  { value: 'quic', label: 'QUIC' },
]

// 传输层与安全层的分段控件选项：只列出当前协议允许的取值。
const networkOptions = computed<SegmentedOption[]>(() =>
  allowedNetworks.value.map((network) => ({
    id: network,
    text: network === 'tcp' ? 'TCP (RAW 推荐)' : 'xhttp (SplitHTTP)',
  }))
)

const securityOptions = computed<SegmentedOption[]>(() =>
  allowedSecurities.value.map((security) => ({
    id: security,
    text:
      security === 'none'
        ? '无加密 (none)'
        : security === 'tls'
          ? '标准 TLS'
          : 'REALITY 伪装',
  }))
)

// XTLS 只在 TCP+TLS/REALITY 下能直接对拷加密后的数据；VLESS Encryption 下只能「穿透
// Encryption」，所以 vlessenc 不参与 vision 门控（后端同样不给 vlessenc 产出 flow）。
const allowVisionFlow = computed(() => {
  return localProtocol.value === 'vless' &&
    localNetwork.value === 'tcp' &&
    (localTlsType.value === 'reality' || localTlsType.value === 'tls')
})

// 只暴露 xray 真正需要的两态：留空 / xtls-rprx-vision。
// 留空不等于「关闭」：TCP + REALITY 下后端会自动补 vision（resolveVLESSFlow），
// 显式 none 才是关闭——该值只为存量行保留，界面不再产出。
const flowOptions = [
  { label: '默认（留空）', value: '' },
  { label: 'xtls-rprx-vision', value: 'xtls-rprx-vision' },
]

// xhttp 不支持 Vision：切走时暂存用户的选择并清空，切回 TCP 时原样还原。
let flowStashed: string | null = null

function enforceCapabilityMatrix() {
  if (!allowedNetworks.value.includes(localNetwork.value)) {
    localNetwork.value = allowedNetworks.value[0]
  }
  if (!allowedSecurities.value.includes(localTlsType.value)) {
    localTlsType.value = allowedSecurities.value[0]
  }
  if (localProtocol.value === 'vless') {
    if (localNetwork.value !== 'tcp') {
      if (vlessForm.value.flow !== '') {
        flowStashed = vlessForm.value.flow
        vlessForm.value.flow = ''
      }
    } else if (flowStashed !== null) {
      vlessForm.value.flow = flowStashed
      flowStashed = null
    }
  }
  if (!allowVisionFlow.value && vlessForm.value.flow === 'xtls-rprx-vision') {
    vlessForm.value.flow = ''
  }
  // 保证当前激活的 Tab 必须在可见列表中
  if (!visibleTabs.value.some((t) => t.id === activeTab.value)) {
    activeTab.value = 'settings'
  }
}

// -----------------------------------------------------------------------------
// 协议 / 模式切换响应
// -----------------------------------------------------------------------------

function onProtocolChange(proto: any) {
  const p = String(proto) as InboundProtocol
  if (p === localProtocol.value) return
  // 切协议后旧协议的流层 / settings 附加键不再适用
  // （例如 vless 的 finalmask 不属于 hy2 表单，带过去会被写进节点配置）。
  streamExtra.value = {}
  settingsExtra.value = {}
  localProtocol.value = p
  if (p === 'hysteria') {
    localNetwork.value = 'hysteria'
    localTlsType.value = 'tls'
    if (localInboundType.value === 'tunnel' || localInboundType.value === 'forward') {
      localInboundType.value = 'user'
    }
  } else if (p === 'tunnel') {
    localInboundType.value = 'tunnel'
  } else if (p === 'vless' && (localInboundType.value === 'tunnel' || localInboundType.value === 'forward')) {
    localInboundType.value = 'user'
  }
  enforceCapabilityMatrix()
  syncFormToJson()
}

function onNetworkChange(detail: { selectedId: string | number }) {
  localNetwork.value = String(detail?.selectedId || 'tcp') as InboundNetwork
  enforceCapabilityMatrix()
  syncFormToJson()
}

function onSecurityChange(detail: { selectedId: string | number }) {
  localTlsType.value = String(detail?.selectedId || 'none') as InboundSecurity
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
  // 手改 JSON 里表单不认识的协议级键原样回写；clients 由后端按接入点注入，永不保留。
  const extra: Record<string, any> = { ...(settingsExtra.value || {}) }
  delete extra.clients

  if (localProtocol.value === 'tunnel') {
    // 目标由派生 freedom 出站的 redirect 承担（字段声明明确禁止 settings.address/port），
    // 入站 settings 只保留转发网络与 followRedirect。
    return JSON.stringify({
      ...extra,
      network: tunnelForm.value.network || 'tcp,udp',
      followRedirect: false,
    })
  }

  if (localProtocol.value === 'hysteria') {
    return JSON.stringify({
      ...extra,
      version: 2,
    })
  }

  // vless
  const s: Record<string, any> = {
    ...extra,
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
  // 内联 PEM 证书表单不编辑，原样回写（否则保存即丢失）。
  certsList.push(...(tlsForm.value.legacy_inline_certs || []))
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
  if (tlsForm.value.reject_unknown_sni) t.rejectUnknownSni = true

  return t
}

function buildSockoptObject(): Record<string, any> | undefined {
  // Hysteria 走 UDP/QUIC：sockopt 没有有效消费方（TCP 项全部无意义），
  // 表单已隐藏，生成侧也不写，避免把上一次 VLESS 的 TCP 调优带进配置。
  if (localProtocol.value === 'hysteria') return undefined
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

  // quicParams 一次性重建：先保留手改 JSON 里的未知键，再覆盖表单认识的字段。
  // 留空/auto 的字段必须显式删除，否则切换回默认值后旧值会残留。
  const qp: Record<string, any> = { ...((fm.quicParams as Record<string, any>) || {}) }
  const setQpString = (key: string, raw: string) => {
    const v = (raw || '').trim()
    if (v) qp[key] = v
    else delete qp[key]
  }
  const setQpNumber = (key: string, v: number | undefined) => {
    if (typeof v === 'number' && Number.isFinite(v)) qp[key] = v
    else delete qp[key]
  }
  const setQpBool = (key: string, on: boolean) => {
    if (on) qp[key] = true
    else delete qp[key]
  }

  setQpString('brutalUp', hysteriaForm.value.bw_up)
  setQpString('brutalDown', hysteriaForm.value.bw_down)
  setQpString('congestion', hysteriaForm.value.congestion)
  setQpString('bbrProfile', hysteriaForm.value.bbr_profile)
  setQpNumber('maxIdleTimeout', hysteriaForm.value.max_idle_timeout)
  setQpNumber('maxIncomingStreams', hysteriaForm.value.max_incoming_streams)
  setQpNumber('initStreamReceiveWindow', hysteriaForm.value.init_stream_receive_window)
  setQpNumber('maxStreamReceiveWindow', hysteriaForm.value.max_stream_receive_window)
  setQpNumber('initConnectionReceiveWindow', hysteriaForm.value.init_connection_receive_window)
  setQpNumber('maxConnectionReceiveWindow', hysteriaForm.value.max_connection_receive_window)
  setQpBool('disablePathMTUDiscovery', hysteriaForm.value.disable_path_mtu_discovery)
  setQpBool('disableGSO', hysteriaForm.value.disable_gso)
  setQpBool('disableStatelessReset', hysteriaForm.value.disable_stateless_reset)
  setQpBool('brutalDisableLossCompensation', hysteriaForm.value.brutal_disable_loss_compensation)

  if (Object.keys(qp).length > 0) fm.quicParams = qp
  else delete fm.quicParams

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
  if (localProtocol.value === 'tunnel') {
    const s: Record<string, any> = {
      ...(streamExtra.value || {}),
      security: 'none',
      // 面板私有键（xray 不识别）：直通派生 freedom 出站是否向目标发送 PPv2，
      // 由后端 TunnelSendProxyProtocol 消费；接收侧见 tcpSettings.acceptProxyProtocol。
      proxy_protocol: tunnelForm.value.proxy_protocol !== false,
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
    // QUIC 只跑 TLS 1.3：版本范围与加密套件对握手不生效；
    // maxVersion 写成 1.2 还会直接握手失败，所以生成侧一律不写这两个键。
    delete t.minVersion
    delete t.maxVersion
    delete t.cipherSuites
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

  // PROXY Protocol 只在传输层维护一个开关：tcp 入站写 tcpSettings.acceptProxyProtocol
  // （xray 运行时会与 sockopt.acceptProxyProtocol 取 OR，所以不再往 sockopt 重复写）。
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
    ...(topExtra.value || {}),
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
  return validationIssues.value.map((issue) => issue.message).join('；')
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
    // Vision 只属于 VLESS；切到 Hysteria/直通后不能把上一次选的值带进库。
    flow: localProtocol.value === 'vless' ? vlessForm.value.flow : '',
    ratio: localRatio.value,
    total_gb: localTotalGB.value,
    expiry_time: localExpiryTime.value,
    shareAddrStrategy: localShareStrategy.value,
    shareAddr: localShareAddr.value,
    sharePort: localSharePort.value,
    shareSni: localShareSni.value,
    shareAllowInsecure: localShareAllowInsecure.value,
    layerId: localLayerId.value,
    targetInboundId: tunnelForm.value.target_inbound_id,
    targetAddress: tunnelForm.value.target_address,
    targetPort: tunnelForm.value.target_port,
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
    if (typeof panel.share_sni === 'string') localShareSni.value = panel.share_sni
    if (typeof panel.share_allow_insecure === 'boolean') {
      localShareAllowInsecure.value = panel.share_allow_insecure
    } else if (tlsSettings?.allowInsecure === true) {
      // 旧配置把 skip-cert-verify 存在 tlsSettings.allowInsecure（服务端生成时本就剥离），
      // 统一归到订阅字段，避免同一语义两个来源。
      localShareAllowInsecure.value = true
    }
    if (typeof panel.layer_id === 'number') localLayerId.value = panel.layer_id

    if (parsed.protocol === 'hysteria' || localProtocol.value === 'hysteria') {
      localProtocol.value = 'hysteria'
      localInboundType.value = 'user'
    } else if (panel.type === 'tunnel' || parsed.protocol === 'tunnel' || localProtocol.value === 'tunnel') {
      localInboundType.value = 'tunnel'
      localProtocol.value = 'tunnel'
    } else {
      localInboundType.value = 'user'
      localProtocol.value = 'vless'
    }

    // streamSettings 里表单未覆盖的顶层键原样保留：
    // vless/tunnel 的 finalmask、externalProxy 等目前没有表单，但手改后保存不能丢。
    const streamKnown = new Set([
      'network', 'security', 'sockopt', 'tlsSettings', 'realitySettings',
    ])
    if (localProtocol.value === 'vless') {
      for (const k of ['tcpSettings', 'xhttpSettings', 'fingerprint', 'acceptProxyProtocol']) streamKnown.add(k)
    } else if (localProtocol.value === 'tunnel') {
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
        // 表单认识的 quicParams 键单独回显，其余（自定义调参）原样保留。
        const qp = fm.quicParams && typeof fm.quicParams === 'object' ? { ...fm.quicParams } : null
        if (qp) {
          for (const k of [
            'brutalUp', 'brutalDown', 'congestion', 'bbrProfile',
            'maxIdleTimeout', 'maxIncomingStreams',
            'initStreamReceiveWindow', 'maxStreamReceiveWindow',
            'initConnectionReceiveWindow', 'maxConnectionReceiveWindow',
            'disablePathMTUDiscovery', 'disableGSO', 'disableStatelessReset',
            'brutalDisableLossCompensation',
          ]) {
            delete qp[k]
          }
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

      const rawCongestion = String(qp.congestion ?? '').toLowerCase()
      const congestion = (['bbr', 'reno', 'brutal', 'force-brutal'].includes(rawCongestion)
        ? rawCongestion
        : '') as HysteriaConfig['congestion']
      const rawBbrProfile = String(qp.bbrProfile ?? '').toLowerCase()
      const bbrProfile = (['conservative', 'standard', 'aggressive'].includes(rawBbrProfile)
        ? rawBbrProfile
        : '') as HysteriaConfig['bbr_profile']
      const numOf = (v: any) => (typeof v === 'number' ? v : undefined)

      hysteriaForm.value = {
        udp_idle_timeout: typeof hySettings?.udpIdleTimeout === 'number' ? hySettings.udpIdleTimeout : 60,
        obfs_type: obfsType,
        obfs_password: obfsPassword,
        obfs_min_packet: minPacket,
        obfs_max_packet: maxPacket,
        bw_up: String(qp.brutalUp ?? ''),
        bw_down: String(qp.brutalDown ?? ''),
        congestion,
        bbr_profile: bbrProfile,
        max_idle_timeout: numOf(qp.maxIdleTimeout),
        max_incoming_streams: numOf(qp.maxIncomingStreams),
        init_stream_receive_window: numOf(qp.initStreamReceiveWindow),
        max_stream_receive_window: numOf(qp.maxStreamReceiveWindow),
        init_connection_receive_window: numOf(qp.initConnectionReceiveWindow),
        max_connection_receive_window: numOf(qp.maxConnectionReceiveWindow),
        disable_path_mtu_discovery: qp.disablePathMTUDiscovery === true,
        disable_gso: qp.disableGSO === true,
        disable_stateless_reset: qp.disableStatelessReset === true,
        brutal_disable_loss_compensation: qp.brutalDisableLossCompensation === true,
        port_hop_mode: hopMode,
        port_hop_ports: hopPorts,
        port_hop_interval: hopInterval,
        finalmaskExtra: Object.keys(fmExtra).length > 0 ? fmExtra : undefined,
        masquerade,
        hysteriaSettingsExtra: Object.keys(hyExtra).length > 0 ? hyExtra : undefined,
      }
    } else if (localProtocol.value === 'tunnel') {
      const hasTargetPanel = typeof panel.target_inbound_id === 'number' ||
        typeof panel.target_address === 'string' ||
        typeof panel.target_port === 'number'
      const targetInboundId = hasTargetPanel ? panel.target_inbound_id : tunnelForm.value.target_inbound_id
      const targetAddress = hasTargetPanel ? (panel.target_address || '') : tunnelForm.value.target_address
      const targetPort = hasTargetPanel ? (panel.target_port || 0) : tunnelForm.value.target_port
      tunnelForm.value = {
        target_mode: hasTargetPanel
          ? (targetInboundId ? 'inbound' : (s?.address || targetAddress || s?.port || targetPort ? 'manual' : 'inbound'))
          : tunnelForm.value.target_mode,
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
    // sockopt.acceptProxyProtocol 是旧版的第二个入口，仅作为回显来源兼容，保存时统一写 tcpSettings。
    const ap = !!streamObj.acceptProxyProtocol || !!tcpSettings?.acceptProxyProtocol ||
      !!streamObj.sockopt?.acceptProxyProtocol
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
      const legacyInlineCerts: any[] = []
      if (Array.isArray(tlsSettings.certificates)) {
        for (const c of tlsSettings.certificates) {
          if (c && typeof c === 'object') {
            if (c.usage === 'verify') {
              // 服务端不消费，仅原样保留，避免存量配置丢字段。
              legacyVerifyCerts.push(c)
            } else if (c.certificateFile || c.keyFile) {
              if (c.certificateFile) certFile = c.certificateFile
              if (c.keyFile) keyFile = c.keyFile
            } else if (Array.isArray(c.certificate) || Array.isArray(c.key)) {
              // 内联 PEM（手写配置）：表单无控件，原样回写。
              legacyInlineCerts.push(c)
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
        reject_unknown_sni: !!tlsSettings.rejectUnknownSni,
        legacy_verify_certs: legacyVerifyCerts,
        legacy_inline_certs: legacyInlineCerts,
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

    // 顶层与 settings 的未建模键原样保留：表单改动触发的重投影不能把它们丢掉。
    const panelKeys = new Set([
      '_panel', 'tag', 'port', 'listen', 'protocol',
      'settings', 'settings_json', 'streamSettings', 'stream_settings', 'sniffing',
      // 旧形态平铺在顶层的面板字段，不参与回写（share_path 已废弃：path 只取 xhttp 设置）
      'network', 'tls_type', 'remark', 'traffic_reset', 'flow', 'ratio', 'total_gb',
      'expiry_time', 'share_addr_strategy', 'share_addr', 'share_port', 'share_security',
      'share_sni', 'share_host', 'share_path', 'share_allow_insecure', 'layer_id', 'cert_id',
      'type', 'target_inbound_id', 'target_address', 'target_port',
    ])
    const topExtraObj: Record<string, any> = {}
    for (const [k, v] of Object.entries(parsed)) {
      if (!panelKeys.has(k)) topExtraObj[k] = v
    }
    topExtra.value = topExtraObj

    const settingsKnown = localProtocol.value === 'vless'
      ? new Set(['decryption', 'fallbacks'])
      : localProtocol.value === 'hysteria'
        ? new Set(['version'])
        : new Set(['network', 'followRedirect'])
    const settingsExtraObj: Record<string, any> = {}
    let settingsObj: any = null
    if (typeof settingsRaw === 'string') {
      try { settingsObj = JSON.parse(settingsRaw) } catch { settingsObj = null }
    } else if (settingsRaw && typeof settingsRaw === 'object') {
      settingsObj = settingsRaw
    }
    if (settingsObj && typeof settingsObj === 'object') {
      for (const [k, v] of Object.entries(settingsObj)) {
        // clients 由后端按接入点注入，保留它只会把旧快照写回库。
        if (!settingsKnown.has(k) && k !== 'clients') settingsExtraObj[k] = v
      }
    }
    settingsExtra.value = settingsExtraObj

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
    // 深监听在当前微任务末尾的 flush 里跑；复位必须排在 flush 之后，
    // 否则标记会先被清掉、watcher 又把整份文档按表单重投影。
    skipSyncAfterRawInput = true
    isInternalUpdating.value = true
    parseJsonToForm(val)
    isInternalUpdating.value = false
    nextTick(() => { skipSyncAfterRawInput = false })
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

// 前置反代卸载只对 vless + xhttp + 明文监听成立；REALITY/TCP/Hysteria 下这段 Caddyfile 是错的。
const caddyOffloadActive = computed(
  () => localProtocol.value === 'vless' && localNetwork.value === 'xhttp' && localTlsType.value === 'none',
)

const caddySnippet = computed(() => {
  if (!caddyOffloadActive.value) return ''
  const domain = activeLayer.value ? activeLayer.value.host : localShareSni.value || localShareAddr.value || 'node.example.com'
  // 反代匹配路径与入站 xhttp 路径必须同值（xray 按 HasPrefix 校验），只有这一个来源。
  const path = xhttpForm.value.path || '/xhttp-stream'
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
  // 对外 TLS 只能由接入层声明：不挂层时订阅会跟随入站传输层（这里是 none），必错。
  if (!props.serverId) {
    ElMessage.warning('请先选择服务器：前置反代需要一个接入层来声明对外 TLS')
    return
  }
  localNetwork.value = 'xhttp'
  localTlsType.value = 'none'
  localListen.value = '127.0.0.1'
  if (localPort.value === 443) {
    localPort.value = 10086
  }
  if (!xhttpForm.value.path) {
    xhttpForm.value.path = '/xhttp-stream'
  }
  xhttpForm.value.mode = 'auto'
  tcpForm.value.acceptProxyProtocol = false

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
    localShareSni,
    localShareAllowInsecure,
    localLayerId,
    vlessForm,
    tunnelForm,
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
    if (skipSyncAfterRawInput) {
      skipSyncAfterRawInput = false
      return
    }
    syncFormToJson()
  },
  { deep: true },
)

onMounted(() => {
  if (props.modelValue && props.modelValue !== '{}') {
    parseJsonToForm(props.modelValue)
  }
  // 新建 REALITY 入站时先补一对密钥，避免「打开就报错、还要手动点生成」。
  if (localTlsType.value === 'reality' && !realityForm.value.private_key) {
    autoGenRealityKey()
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
    <CsTabs v-model="activeTab" :tabs="visibleTabs" aria-label="入站配置分类">
      <div class="form-body">
        <!-- 保存门禁：把所有硬错误汇总到一处，并标注所属页签 -->
        <CsAlert v-if="validationIssues.length" type="error" class="validation-summary">
          <div class="validation-title">保存前需修正</div>
          <div v-for="(issue, idx) in validationIssues" :key="idx">
            {{ TAB_LABELS[issue.tab] }}：{{ issue.message }}
          </div>
        </CsAlert>
        <!-- ==================== 1. 基础设置：入站 JSON 顶层四字段 + 面板计费字段 ==================== -->
        <div v-show="activeTab === 'settings'" class="tab-pane">
          <CsContainer header="入站顶层字段">
            <div class="form-grid-2">
              <CsFormItem label="监听地址 (listen)">
                <CsInput v-model="localListen" placeholder="默认 0.0.0.0，本地反代填 127.0.0.1" class="mono" />
              </CsFormItem>

              <CsFormItem label="端口 (port)">
                <CsInputNumber v-model="localPort" :min="1" :max="65535" style="width: 100%" class="mono" />
              </CsFormItem>

              <CsFormItem label="唯一标识 (tag)">
                <CsInput v-model="localTag" placeholder="如 Tokyo-01-VLESS" />
              </CsFormItem>

              <CsFormItem label="协议 (protocol)">
                <CsSelect
                  :model-value="localProtocol"
                  :options="protocolOptions"
                  style="width: 100%"
                  @change="onProtocolChange"
                />
              </CsFormItem>
            </div>
          </CsContainer>

          <CsContainer header="备注与计费策略">
            <div class="form-grid-2">
              <CsFormItem label="备注 (Remark)" style="grid-column: span 2">
                <CsInput v-model="localRemark" placeholder="如 深圳专线入口 / 办公内网" />
              </CsFormItem>

              <CsFormItem label="流量倍率">
                <CsInputNumber v-model="localRatio" :min="0" :step="0.1" :precision="2" style="width: 100%" class="mono" />
              </CsFormItem>

              <CsFormItem label="流量配额 (GB)">
                <CsInputNumber v-model="localTotalGB" :min="0" :step="10" :precision="0" style="width: 100%" class="mono" />
              </CsFormItem>

              <CsFormItem label="流量重置周期">
                <CsSelect v-model="localTrafficReset" :options="trafficResetOptions" style="width: 100%" />
              </CsFormItem>

              <CsFormItem label="到期时间">
                <CsDatePicker
                  v-model="localExpiryTime"
                  type="datetime"
                  placeholder="留空为永久有效"
                  style="width: 100%"
                  value-format="YYYY-MM-DDTHH:mm:ssZ"
                />
              </CsFormItem>
            </div>
          </CsContainer>
        </div>

        <!-- ==================== 2. 协议专属字段：settings 里各协议独有的键 ==================== -->
        <div v-show="activeTab === 'protocol'" class="tab-pane">
          <VlessSettings v-if="localProtocol === 'vless'" v-model="vlessForm" />
          <HysteriaSettings v-else-if="localProtocol === 'hysteria'" v-model="hysteriaForm" />
          <div class="card-note">
            认证用户由绑定的接入点注入（settings.users），本页只维护协议级参数。
          </div>
        </div>

        <!-- ==================== 3. 传输层 Tab：streamSettings 非 security 部分 ==================== -->
        <div v-show="activeTab === 'transport'" class="tab-pane">
          <template v-if="localProtocol !== 'tunnel'">
            <CsContainer header="传输方式 (streamSettings.network)">
              <template #info>
                <TipIcon
                  :content="
                    localProtocol === 'hysteria'
                      ? '层级顺序：协议 settings → hysteria 传输 → TLS → finalmask（QUIC 调参）。'
                      : '层级顺序（由内向外）：协议 settings → 传输 network → 安全 security → finalmask（加密完成后的外层伪装 / QUIC 调参）→ sockopt（最底层 socket 行为）。finalmask 等未提供表单的键可在「原始 JSON」中编辑，本表单会原样保留。'
                  "
                />
              </template>
              <CsFormItem v-if="localProtocol === 'vless'" label="传输协议类型">
                <CsSegmentedControl
                  v-model="localNetwork"
                  :options="networkOptions"
                  @change="onNetworkChange"
                />
              </CsFormItem>
              <div v-else class="card-note">
                固定使用 hysteria 传输（network=hysteria，魔改 QUIC），加密固定为 TLS。
              </div>
            </CsContainer>

            <TcpTransport v-if="localProtocol === 'vless' && localNetwork === 'tcp'" v-model="tcpForm" />
            <XhttpTransport
              v-else-if="localProtocol === 'vless' && localNetwork === 'xhttp'"
              v-model="xhttpForm"
            />
            <HysteriaFinalMask v-else-if="localProtocol === 'hysteria'" v-model="hysteriaForm" />

            <CsContainer
              v-if="localProtocol === 'vless' && localNetwork === 'tcp'"
              header="流控 (settings.flow)"
            >
              <template #info>
                <TipIcon
                  content="Vision 属于协议 settings，不属于传输层：它让 TCP 在 TLS 解密后零拷贝转发。留空时只有 TCP + REALITY 会由后端自动补 vision；TCP + TLS 需要手动选择。VLESS Encryption 与 XTLS 不搭配（没有可直拷的底层 TLS，只能穿透 Encryption），因此不提供该选项。"
                />
              </template>
              <CsFormItem
                label="流控算法 (Flow)"
                :constraint-text="allowVisionFlow ? '' : '当前安全层不支持 Vision：仅 TCP + TLS / REALITY 可用（VLESS Encryption 下只能穿透 Encryption）。'"
              >
                <CsSelect
                  v-model="vlessForm.flow"
                  :options="flowOptions"
                  :disabled="!allowVisionFlow"
                  style="width: 100%"
                />
              </CsFormItem>
            </CsContainer>
          </template>

          <!-- 直通：目标 / 转发网络 / Proxy Protocol 都属于「这条管道的传输语义」，
               与 sockopt 同页；streamSettings.network 恒为 raw，不再单独展示。 -->
          <TunnelForward
            v-if="localProtocol === 'tunnel'"
            v-model="tunnelForm"
            :current-inbound-id="props.inboundId"
          />

          <!-- Hysteria 走 UDP/QUIC，sockopt 的 TCP 项全部无效、UDP 项也无实际收益，整页隐藏。 -->
          <SockoptSettings v-if="localProtocol !== 'hysteria'" v-model="sockoptForm" />
        </div>

        <!-- ==================== 4. 安全层 Tab：streamSettings.security ==================== -->
        <div v-show="activeTab === 'security'" class="tab-pane">
          <CsContainer header="安全加密协议 (streamSettings.security)">
            <CsFormItem
              label="安全协议类型"
              :constraint-text="
                localProtocol === 'hysteria'
                  ? 'Hysteria 2 强制启用 TLS/QUIC，不支持明文传输。'
                  : localTlsType === 'none'
                    ? '未启用传输层加密 (none)，仅适用于前置反代或受信任局域网。'
                    : ''
              "
            >
              <CsSegmentedControl
                v-model="localTlsType"
                :options="securityOptions"
                @change="onSecurityChange"
              />
            </CsFormItem>
          </CsContainer>

          <TlsSecurity
            v-if="localTlsType === 'tls'"
            v-model="tlsForm"
            :alpn-fixed="localProtocol === 'hysteria' ? 'h3' : ''"
            :quic="localProtocol === 'hysteria'"
            :allow-insecure="localShareAllowInsecure"
            @update:cert-id="(id) => emit('update:certId', id)"
            @update:allow-insecure="(v) => localShareAllowInsecure = v"
          />
          <RealitySecurity
            v-else-if="localTlsType === 'reality'"
            v-model="realityForm"
            v-model:fingerprint="realityFingerprint"
          />
        </div>

        <!-- ==================== 5. 嗅探 Tab：inbound.sniffing ==================== -->
        <div v-show="activeTab === 'sniffing'" class="tab-pane">
          <CsContainer header="流量嗅探 (sniffing)">
            <template #actions>
              <CsToggle v-model="sniffingForm.enabled" label="启用流量嗅探" />
            </template>
            <div v-if="sniffingForm.enabled" class="form-grid-2">
              <CsFormItem label="嗅探目标协议" style="grid-column: span 2" class="sniff-targets">
                <CsCheckboxGroup
                  v-model="sniffingForm.destOverride"
                  :options="sniffingTargetOptions"
                />
              </CsFormItem>

              <CsFormItem label="仅提取元数据 (metadataOnly)">
                <div class="control-cell">
                  <CsToggle v-model="sniffingForm.metadataOnly" label="仅提取不覆盖连接" />
                </div>
              </CsFormItem>

              <CsFormItem label="仅用于路由 (routeOnly)">
                <div class="control-cell">
                  <CsToggle v-model="sniffingForm.routeOnly" label="仅供出站路由分流决策" />
                </div>
              </CsFormItem>
            </div>
          </CsContainer>
        </div>

        <!-- ==================== 6. 业务与订阅 Tab（面板字段，不写入 xray 配置） ==================== -->
        <div v-show="activeTab === 'business'" class="tab-pane">
          <BusinessSettings
            v-if="localInboundType === 'user'"
            :can-offload="localProtocol === 'vless'"
            :layer-id="localLayerId"
            :layers="layers"
            :server-id="props.serverId"
            :share-strategy="localShareStrategy"
            :share-addr="localShareAddr"
            :share-port="localSharePort"
            :share-sni="localShareSni"
            :transport-security="localTlsType"
            :caddy-snippet="caddySnippet"
            :layer-deleting="layerDeleting"
            @update:layer-id="(v) => localLayerId = v"
            @update:share-strategy="(v) => localShareStrategy = v"
            @update:share-addr="(v) => localShareAddr = v"
            @update:share-port="(v) => localSharePort = v"
            @update:share-sni="(v) => localShareSni = v"
            @open-layer-quick="openLayerQuick"
            @delete-layer="deleteLayerFromEditor"
            @apply-caddy-preset="applyCaddyOffloadPreset"
          />
        </div>

        <!-- ==================== 7. 原始 JSON Tab ==================== -->
        <div v-show="activeTab === 'advanced'" class="tab-pane">
          <CsContainer header="入站底层配置 (Config JSON)">
            <template #info>
              <TipIcon
                content="这是面板存储的投影源（服务端与订阅共用）。生成服务端配置时会剥离 fingerprint、realitySettings.publicKey、tlsSettings.allowInsecure、xhttpSettings.xmux / downloadSettings / noGRPCHeader 等客户端字段；订阅侧仍使用它们。直通的 streamSettings.proxy_protocol 是面板私有键（供派生 freedom 出站发送 PPv2），不会写进节点配置。"
              />
            </template>
            <template #actions>
              <div class="json-status">
                <CsBadge v-if="!jsonError" color="green">
                  <CsIcon name="check" /> 语法正确
                </CsBadge>
                <CsBadge v-else color="red">
                  <CsIcon name="status-warning" /> {{ jsonError }}
                </CsBadge>
                <CsButton icon-name="gen-ai" @click="formatRawJson">格式化</CsButton>
              </div>
            </template>
            <div class="json-editor-wrap">
              <CodeEditor
                :model-value="rawJsonText"
                language="json"
                height="480px"
                @update:model-value="onRawJsonInput"
              />
            </div>
          </CsContainer>
        </div>
      </div>
    </CsTabs>
  </div>

  <!-- 快捷新建对外接入层弹窗 -->
  <CsModal
    v-model:visible="layerQuickOpen"
    header="新建对外接入层"
    size="medium"
    :close-on-click-overlay="false"
  >
    <CsForm label-position="top">
      <div class="card-note layer-form-alert">
        对外接入层定义订阅对外 Host / 端口 / 安全层，创建后将绑定至当前入站。
      </div>
      <CsFormItem label="层名称" required>
        <CsInput v-model="layerQuickForm.name" placeholder="如 HK 443 反代层" />
      </CsFormItem>
      <div class="layer-host-grid">
        <CsFormItem label="对外 Host" required>
          <CsInput v-model="layerQuickForm.host" placeholder="域名或 IP，如 hk.edge.example.com" />
        </CsFormItem>
        <CsFormItem label="对外端口">
          <CsInputNumber v-model="layerQuickForm.port" :min="1" :max="65535" style="width: 100%" class="mono" />
        </CsFormItem>
      </div>
      <CsFormItem label="对外安全层">
        <CsSelect v-model="layerQuickForm.security" :options="layerSecurityOptions" style="width: 100%" />
      </CsFormItem>
    </CsForm>
    <template #footer>
      <CsButton @click="layerQuickOpen = false">取消</CsButton>
      <CsButton variant="primary" :loading="layerQuickSaving" @click="saveLayerQuick">创建并绑定</CsButton>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.inbound-editor {
  display: flex;
  flex-direction: column;
}

.form-body {
  display: flex;
  flex-direction: column;
}

.tab-pane {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* 等宽数字：只改字形，不缩字号（cs-text-mono 会把字号降到 12px） */
.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}

.validation-summary {
  margin-bottom: 12px;
}

/* 卡片级说明：与字段级 constraint-text 同一字号/颜色，但不带图标也不占色块 */
.card-note {
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-form-secondary-wt0md2, #656871);
}

/* 嗅探目标是三个短选项：横向铺满整行，避免左侧堆成一列、右半边全空 */
.sniff-targets :deep([role='group']) {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 24px;
}

.validation-title {
  font-weight: 600;
  margin-bottom: 2px;
}

.json-status {
  display: flex;
  align-items: center;
  gap: 8px;
}

.json-editor-wrap {
  margin-top: 12px;
}

.layer-form-alert {
  margin-bottom: 16px;
}

.layer-host-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 0 16px;
}

@media (max-width: $bp-tablet-max) {
  .layer-host-grid {
    grid-template-columns: 1fr;
  }
}
</style>
