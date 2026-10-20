<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, CopyDocument, Download, Share } from '@element-plus/icons-vue'
import { ElMessage } from '@/components/cloudscape'
import CodeEditor from '@/components/CodeEditor.vue'
import { parseNodeLink, formatNodeLink, type ParsedNodeLink } from '@/utils/nodeLink'
import {
  createServerOutbound,
  updateServerOutbound,
  getXrayKeys,
  getServers,
  getInbounds,
  type ServerOutbound,
  type ServerItem,
} from '@/api/admin'
import type { InboundItem } from '@/api/types'
import { errMsg } from '@/api/http'

const props = defineProps<{
  serverId: number
  outbound?: ServerOutbound | null
}>()

const emit = defineEmits<{
  (e: 'saved'): void
  (e: 'close'): void
}>()

const visible = ref(false)
const saving = ref(false)

// 4 种业务场景模式：
// 1. basic: 基础出口 (freedom, blackhole)
// 2. inbound_ref: 引用落地入站 (拓扑连线中转)
// 3. proxy: 外部代理节点 (vless, hysteria, socks, http)
// 4. custom: 原生自定义 JSON (透传 Xray outbound 对象)
type SceneMode = 'basic' | 'inbound_ref' | 'proxy' | 'custom'
const sceneMode = ref<SceneMode>('basic')

const isSystemReserved = computed(() => props.outbound?.tag === 'direct' || props.outbound?.tag === 'blocked')

// ===== 引用落地入站 (InboundRef) =====
const refServerId = ref<number>(0)
const refInboundId = ref<number>(0)
const servers = ref<ServerItem[]>([])
const allInbounds = ref<InboundItem[]>([])

// 选中服务器的可引用入站 (允许所有非 freedom 协议，如 vless, hysteria, mixed)
const refInboundOptions = computed(() => {
  return allInbounds.value.filter((i) => i.server_id === refServerId.value && i.enabled && i.protocol !== 'freedom')
})

function inboundLabel(i: InboundItem) {
  let net = '—'
  try {
    const s = JSON.parse(i.stream_settings || '{}')
    net = `${s.network || 'tcp'}/${s.security || 'none'}`
  } catch { /* ignore */ }
  return `${i.tag} :${i.port}（${i.protocol}+${net}）`
}

function onRefInboundChange(inbId: number) {
  const inb = allInbounds.value.find((i) => i.id === inbId)
  if (inb) {
    form.protocol = inb.protocol
  }
}

async function loadRefTargets() {
  try {
    const [s, i] = await Promise.all([getServers(), getInbounds()])
    if (s.data.code === 0) servers.value = s.data.data.items
    if (i.data.code === 0) allInbounds.value = i.data.data.items
  } catch { /* 引用选择器加载失败不阻塞 */ }
}

// ===== 基础表单状态 =====
const form = reactive({
  tag: '',
  protocol: 'freedom',
  enabled: true,
  priority: 0,
  remark: '',
  send_through: '',
  tls_verify_mode: '',
  // freedom 子字段
  domain_strategy: 'AsIs',
  block_private: true,
  block_cn: false,
  redirect: '',
  // blackhole 子字段
  response_type: 'none',
  // vless 子字段
  vless_address: '',
  vless_port: 443,
  vless_uuid: '',
  vless_flow: '',
  vless_encryption: 'none',
  vless_network: 'tcp',
  vless_security: 'none',
  vless_fingerprint: 'chrome',
  // vless 传输层
  ws_path: '/',
  ws_host: '',
  xhttp_mode: 'auto',
  xhttp_path: '/',
  xhttp_host: '',
  grpc_service_name: 'grpc',
  grpc_authority: '',
  grpc_multi_mode: false,
  // vless 安全层
  tls_server_name: '',
  tls_allow_insecure: false,
  tls_pinned_peer_cert_sha256: '',
  tls_disable_system_root: false,
  tls_ca_cert_file: '',
  reality_server_name: '',
  reality_public_key: '',
  reality_short_id: '',
  reality_fingerprint: 'chrome',
  reality_spider_x: '/',

  // Hysteria 2 子字段
  hy2_address: '',
  hy2_port: 8443,
  hy2_password: '',
  hy2_sni: '',
  hy2_allow_insecure: false,
  hy2_fingerprint: 'chrome',
  hy2_pinned_peer_cert_sha256: '',
  hy2_ca_cert_file: '',

  // SOCKS 子字段
  socks_address: '',
  socks_port: 1080,
  socks_user: '',
  socks_password: '',
  socks_tls: false,
  socks_sni: '',

  // HTTP 子字段
  http_address: '',
  http_port: 8080,
  http_user: '',
  http_password: '',
  http_tls: false,
  http_sni: '',

  // 原生自定义 JSON
  outbound_json: '',
})

// 选项常量
const BASIC_PROTOCOL_OPTIONS = [
  { label: 'freedom (自主直连出口)', value: 'freedom' },
  { label: 'blackhole (黑洞阻断出口)', value: 'blackhole' },
]

const PROXY_PROTOCOL_OPTIONS = [
  { label: 'vless (VLESS 代理链)', value: 'vless' },
  { label: 'hysteria (Hysteria 2 客户端)', value: 'hysteria' },
  { label: 'socks (SOCKS5 代理客户端)', value: 'socks' },
  { label: 'http (HTTP/HTTPS 代理客户端)', value: 'http' },
]

const FLOW_OPTIONS = [
  { label: '无', value: '' },
  { label: 'xtls-rprx-vision', value: 'xtls-rprx-vision' },
]

const NETWORK_OPTIONS = [
  { label: 'tcp', value: 'tcp' },
  { label: 'ws (WebSocket)', value: 'ws' },
  { label: 'xhttp', value: 'xhttp' },
  { label: 'grpc', value: 'grpc' },
]

const SECURITY_OPTIONS = [
  { label: 'none (明文)', value: 'none' },
  { label: 'tls (TLS)', value: 'tls' },
  { label: 'reality (REALITY)', value: 'reality' },
]

const FINGERPRINT_OPTIONS = [
  'chrome', 'firefox', 'safari', 'edge', '360', 'qq', 'random', 'randomized',
]

const DOMAIN_STRATEGY_OPTIONS = [
  { label: 'AsIs (保持原样)', value: 'AsIs' },
  { label: 'UseIP (解析后连接)', value: 'UseIP' },
  { label: 'UseIPv4', value: 'UseIPv4' },
  { label: 'UseIPv6', value: 'UseIPv6' },
]

const OUTBOUND_JSON_PLACEHOLDER = `{\n  "tag": "my-warp",\n  "protocol": "wireguard",\n  "settings": {\n    "secretKey": "...",\n    "address": ["172.16.0.2/32"],\n    "peers": [\n      { "publicKey": "...", "endpoint": "162.159.192.1:2408" }\n    ]\n  }\n}`

function onSceneChange(mode: any) {
  if (mode === 'basic') {
    if (form.protocol !== 'freedom' && form.protocol !== 'blackhole') {
      form.protocol = 'freedom'
    }
  } else if (mode === 'inbound_ref') {
    // 保持当前或根据所选入站更新
  } else if (mode === 'proxy') {
    if (!['vless', 'hysteria', 'socks', 'http'].includes(form.protocol)) {
      form.protocol = 'vless'
    }
  } else if (mode === 'custom') {
    form.protocol = 'custom'
  }
}

// ===== REALITY key gen =====
const genKeyLoading = ref(false)

async function fetchRealityKeys() {
  genKeyLoading.value = true
  try {
    const { data } = await getXrayKeys()
    if (data.code === 0) {
      form.reality_public_key = data.data.public_key
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

function genShortId() {
  const chars = '0123456789abcdef'
  let result = ''
  for (let i = 0; i < 16; i++) result += chars.charAt(Math.floor(Math.random() * chars.length))
  form.reality_short_id = result
}

// ===== 导入链接弹窗 =====
const importDialogVisible = ref(false)
const importLinkText = ref('')

function openImportDialog() {
  importLinkText.value = ''
  importDialogVisible.value = true
}

function doImportLink() {
  try {
    const parsed = parseNodeLink(importLinkText.value)
    sceneMode.value = 'proxy'
    form.protocol = parsed.protocol
    if (parsed.tag && !form.tag) {
      form.tag = parsed.tag
    }

    if (parsed.protocol === 'vless') {
      form.vless_address = parsed.address
      form.vless_port = parsed.port
      form.vless_uuid = parsed.uuid || ''
      form.vless_flow = parsed.flow || ''
      form.vless_encryption = parsed.encryption || 'none'
      form.vless_network = parsed.network || 'tcp'
      form.vless_security = parsed.security || 'none'
      form.vless_fingerprint = parsed.fingerprint || 'chrome'
      form.tls_server_name = parsed.sni || ''
      form.tls_allow_insecure = !!parsed.allowInsecure
      form.reality_server_name = parsed.sni || ''
      form.reality_public_key = parsed.realityPublicKey || ''
      form.reality_short_id = parsed.realityShortId || ''
      form.ws_path = parsed.wsPath || '/'
      form.ws_host = parsed.wsHost || ''
      form.xhttp_mode = parsed.xhttpMode || 'auto'
      form.xhttp_path = parsed.xhttpPath || '/'
      form.xhttp_host = parsed.xhttpHost || ''
      form.grpc_service_name = parsed.grpcServiceName || 'grpc'
    } else if (parsed.protocol === 'hysteria') {
      form.hy2_address = parsed.address
      form.hy2_port = parsed.port
      form.hy2_password = parsed.password || ''
      form.hy2_sni = parsed.sni || ''
      form.hy2_allow_insecure = !!parsed.allowInsecure
      form.hy2_fingerprint = parsed.fingerprint || 'chrome'
    } else if (parsed.protocol === 'socks') {
      form.socks_address = parsed.address
      form.socks_port = parsed.port
      form.socks_user = parsed.username || ''
      form.socks_password = parsed.password || ''
      form.socks_tls = parsed.security === 'tls'
      form.socks_sni = parsed.sni || ''
    } else if (parsed.protocol === 'http') {
      form.http_address = parsed.address
      form.http_port = parsed.port
      form.http_user = parsed.username || ''
      form.http_password = parsed.password || ''
      form.http_tls = parsed.security === 'tls'
      form.http_sni = parsed.sni || ''
    }

    ElMessage.success(`已成功解析并导入 ${parsed.protocol.toUpperCase()} 节点配置`)
    importDialogVisible.value = false
  } catch (err: any) {
    ElMessage.error(err.message || '导入链接解析失败')
  }
}

// 导出分享链接
async function exportNodeLink() {
  try {
    let dto: ParsedNodeLink
    const tag = form.tag || 'export-node'
    if (form.protocol === 'vless') {
      if (!form.vless_address || !form.vless_uuid) {
        ElMessage.warning('请先填写 VLESS 远端地址与 UUID')
        return
      }
      dto = {
        protocol: 'vless',
        tag,
        address: form.vless_address,
        port: form.vless_port,
        uuid: form.vless_uuid,
        flow: form.vless_flow,
        encryption: form.vless_encryption,
        network: form.vless_network,
        security: form.vless_security as any,
        fingerprint: form.vless_fingerprint,
        sni: form.vless_security === 'reality' ? form.reality_server_name : form.tls_server_name,
        realityPublicKey: form.reality_public_key,
        realityShortId: form.reality_short_id,
        allowInsecure: form.tls_allow_insecure,
        wsPath: form.ws_path,
        wsHost: form.ws_host,
        xhttpMode: form.xhttp_mode,
        xhttpPath: form.xhttp_path,
        xhttpHost: form.xhttp_host,
        grpcServiceName: form.grpc_service_name,
      }
    } else if (form.protocol === 'hysteria') {
      if (!form.hy2_address || !form.hy2_password) {
        ElMessage.warning('请先填写 Hysteria 2 远端地址与认证密码')
        return
      }
      dto = {
        protocol: 'hysteria',
        tag,
        address: form.hy2_address,
        port: form.hy2_port,
        password: form.hy2_password,
        sni: form.hy2_sni,
        allowInsecure: form.hy2_allow_insecure,
        fingerprint: form.hy2_fingerprint,
      }
    } else if (form.protocol === 'socks') {
      if (!form.socks_address) {
        ElMessage.warning('请先填写 SOCKS5 远端地址')
        return
      }
      dto = {
        protocol: 'socks',
        tag,
        address: form.socks_address,
        port: form.socks_port,
        username: form.socks_user,
        password: form.socks_password,
        security: form.socks_tls ? 'tls' : 'none',
        sni: form.socks_sni,
      }
    } else if (form.protocol === 'http') {
      if (!form.http_address) {
        ElMessage.warning('请先填写 HTTP 代理远端地址')
        return
      }
      dto = {
        protocol: 'http',
        tag,
        address: form.http_address,
        port: form.http_port,
        username: form.http_user,
        password: form.http_password,
        security: form.http_tls ? 'tls' : 'none',
        sni: form.http_sni,
      }
    } else {
      ElMessage.warning('当前协议不支持生成标准分享链接')
      return
    }

    const link = formatNodeLink(dto)
    await navigator.clipboard.writeText(link)
    ElMessage.success('已复制分享链接到剪贴板')
  } catch (err: any) {
    ElMessage.error(err.message || '生成链接失败')
  }
}

// ===== JSON 构建 =====
function buildSettingsJSON(): string {
  switch (form.protocol) {
    case 'freedom': {
      const s: Record<string, any> = { domainStrategy: form.domain_strategy }
      if (form.redirect) s.redirect = form.redirect
      if (form.block_cn) s.block_cn = true
      const rules: Record<string, any>[] = []
      if (form.block_private) {
        rules.push({ action: 'block', ip: ['geoip:private'], blockDelay: '0' })
      }
      rules.push({ action: 'allow' })
      s.finalRules = rules
      return JSON.stringify(s)
    }
    case 'blackhole':
      return JSON.stringify({ response: { type: form.response_type } })
    case 'vless': {
      const s: Record<string, any> = {
        vnext: [{
          address: form.vless_address,
          port: form.vless_port,
          users: [{
            id: form.vless_uuid,
            flow: form.vless_flow || undefined,
            encryption: form.vless_encryption || 'none',
          }],
        }],
      }
      return JSON.stringify(s)
    }
    case 'hysteria': {
      return JSON.stringify({
        version: 2,
        address: form.hy2_address,
        port: form.hy2_port,
      })
    }
    case 'socks': {
      const s: Record<string, any> = {
        servers: [{
          address: form.socks_address,
          port: form.socks_port,
          users: form.socks_user ? [{
            user: form.socks_user,
            pass: form.socks_password,
            level: 0,
          }] : [],
        }],
      }
      return JSON.stringify(s)
    }
    case 'http': {
      const s: Record<string, any> = {
        servers: [{
          address: form.http_address,
          port: form.http_port,
          users: form.http_user ? [{
            user: form.http_user,
            pass: form.http_password,
          }] : [],
        }],
      }
      return JSON.stringify(s)
    }
    default:
      return '{}'
  }
}

function buildStreamJSON(): string {
  if (form.protocol === 'vless') {
    const s: Record<string, any> = {
      network: form.vless_network,
      security: form.vless_security,
    }
    if (form.vless_network === 'ws') {
      const ws: Record<string, any> = { path: form.ws_path || '/' }
      if (form.ws_host) ws.host = form.ws_host
      s.wsSettings = ws
    } else if (form.vless_network === 'xhttp') {
      s.xhttpSettings = { mode: form.xhttp_mode || 'auto', path: form.xhttp_path || '/' }
      if (form.xhttp_host) s.xhttpSettings.host = form.xhttp_host
    } else if (form.vless_network === 'grpc') {
      const g: Record<string, any> = { serviceName: form.grpc_service_name || 'grpc' }
      if (form.grpc_authority) g.authority = form.grpc_authority
      if (form.grpc_multi_mode) g.multiMode = true
      s.grpcSettings = g
    }

    if (form.vless_security === 'tls') {
      const t: Record<string, any> = { serverName: form.tls_server_name || undefined }
      if (form.tls_allow_insecure) t.allowInsecure = true
      if (form.vless_fingerprint) t.fingerprint = form.vless_fingerprint
      if (form.tls_pinned_peer_cert_sha256) t.pinnedPeerCertSha256 = form.tls_pinned_peer_cert_sha256
      if (form.tls_disable_system_root) t.disableSystemRoot = true
      if (form.tls_ca_cert_file) t.certificates = [{ certificateFile: form.tls_ca_cert_file, usage: 'verify' }]
      s.tlsSettings = t
    } else if (form.vless_security === 'reality') {
      s.realitySettings = {
        serverName: form.reality_server_name || undefined,
        password: form.reality_public_key || undefined,
        shortId: form.reality_short_id || undefined,
        fingerprint: form.reality_fingerprint || 'chrome',
        spiderX: form.reality_spider_x || '/',
      }
    }
    return JSON.stringify(s)
  }

  if (form.protocol === 'hysteria') {
    const t: Record<string, any> = {
      serverName: form.hy2_sni || undefined,
      allowInsecure: form.hy2_allow_insecure || false,
    }
    if (form.hy2_fingerprint) t.fingerprint = form.hy2_fingerprint
    if (form.hy2_pinned_peer_cert_sha256) t.pinnedPeerCertSha256 = form.hy2_pinned_peer_cert_sha256
    if (form.hy2_ca_cert_file) t.certificates = [{ certificateFile: form.hy2_ca_cert_file, usage: 'verify' }]
    const s: Record<string, any> = {
      network: 'hysteria',
      security: 'tls',
      tlsSettings: t,
      hysteriaSettings: {
        auth: form.hy2_password,
      },
    }
    return JSON.stringify(s)
  }

  if (form.protocol === 'socks') {
    if (form.socks_tls) {
      return JSON.stringify({
        network: 'tcp',
        security: 'tls',
        tlsSettings: {
          serverName: form.socks_sni || undefined,
        },
      })
    }
    return JSON.stringify({ network: 'tcp', security: 'none' })
  }

  if (form.protocol === 'http') {
    if (form.http_tls) {
      return JSON.stringify({
        network: 'tcp',
        security: 'tls',
        tlsSettings: {
          serverName: form.http_sni || undefined,
        },
      })
    }
    return JSON.stringify({ network: 'tcp', security: 'none' })
  }

  return ''
}

// ===== 从已有 outbound 解析回表单 =====
function parseExisting() {
  if (!props.outbound) return
  form.tag = props.outbound.tag
  form.protocol = props.outbound.protocol
  form.enabled = props.outbound.enabled
  form.priority = props.outbound.priority
  form.remark = props.outbound.remark
  form.send_through = props.outbound.send_through || ''

  const ob = props.outbound

  // 1. 原生自定义 JSON
  if (ob.outbound_json) {
    sceneMode.value = 'custom'
    form.protocol = 'custom'
    form.outbound_json = ob.outbound_json
    return
  }

  // 2. 引用落地入站
  if (ob.inbound_ref) {
    sceneMode.value = 'inbound_ref'
    refInboundId.value = ob.inbound_ref
    form.tls_verify_mode = ob.tls_verify_mode || ''
    const target = allInbounds.value.find((i) => i.id === ob.inbound_ref)
    if (target) {
      refServerId.value = target.server_id
      form.protocol = target.protocol
    }
    return
  }

  // 3. 外部代理节点
  if (['vless', 'hysteria', 'socks', 'http'].includes(ob.protocol)) {
    sceneMode.value = 'proxy'
  } else {
    sceneMode.value = 'basic'
  }

  try {
    const settings = ob.settings_json ? JSON.parse(ob.settings_json) : {}
    if (form.protocol === 'freedom') {
      form.domain_strategy = settings.domainStrategy || 'AsIs'
      form.block_private = Array.isArray(settings.finalRules) && settings.finalRules.some((r: any) => r.action === 'block' && r.ip?.includes?.('geoip:private'))
      form.block_cn = !!settings.block_cn
      form.redirect = settings.redirect || ''
    } else if (form.protocol === 'blackhole') {
      form.response_type = settings.response?.type || 'none'
    } else if (form.protocol === 'vless') {
      const vnext = settings.vnext?.[0]
      if (vnext) {
        form.vless_address = vnext.address || ''
        form.vless_port = vnext.port || 443
        const user = vnext.users?.[0]
        if (user) {
          form.vless_uuid = user.id || ''
          form.vless_flow = user.flow || ''
          form.vless_encryption = user.encryption || 'none'
        }
      }
    } else if (form.protocol === 'hysteria') {
      form.hy2_address = settings.address || ''
      form.hy2_port = settings.port || 8443
    } else if (form.protocol === 'socks') {
      const srv = settings.servers?.[0]
      if (srv) {
        form.socks_address = srv.address || ''
        form.socks_port = srv.port || 1080
        const u = srv.users?.[0]
        if (u) {
          form.socks_user = u.user || ''
          form.socks_password = u.pass || ''
        }
      }
    } else if (form.protocol === 'http') {
      const srv = settings.servers?.[0]
      if (srv) {
        form.http_address = srv.address || ''
        form.http_port = srv.port || 8080
        const u = srv.users?.[0]
        if (u) {
          form.http_user = u.user || ''
          form.http_password = u.pass || ''
        }
      }
    }
  } catch { /* ignore parse errors */ }

  try {
    const stream = ob.stream_settings_json ? JSON.parse(ob.stream_settings_json) : {}
    if (form.protocol === 'vless') {
      form.vless_network = stream.network || 'tcp'
      form.vless_security = stream.security || 'none'
      form.vless_fingerprint = stream.fingerprint || 'chrome'

      if (stream.wsSettings) { form.ws_path = stream.wsSettings.path || '/'; form.ws_host = stream.wsSettings.host || stream.wsSettings.headers?.Host || '' }
      if (stream.xhttpSettings) { form.xhttp_mode = stream.xhttpSettings.mode || 'auto'; form.xhttp_path = stream.xhttpSettings.path || '/'; form.xhttp_host = stream.xhttpSettings.host || '' }
      if (stream.grpcSettings) { form.grpc_service_name = stream.grpcSettings.serviceName || 'grpc'; form.grpc_authority = stream.grpcSettings.authority || ''; form.grpc_multi_mode = !!stream.grpcSettings.multiMode }

      if (stream.tlsSettings) {
        form.tls_server_name = stream.tlsSettings.serverName || ''
        form.tls_allow_insecure = !!stream.tlsSettings.allowInsecure
        form.tls_pinned_peer_cert_sha256 = stream.tlsSettings.pinnedPeerCertSha256 || ''
        form.tls_disable_system_root = !!stream.tlsSettings.disableSystemRoot
        if (Array.isArray(stream.tlsSettings.certificates)) {
          const caCert = stream.tlsSettings.certificates.find((c: any) => c?.usage === 'verify')
          form.tls_ca_cert_file = caCert?.certificateFile || ''
        }
      }
      if (stream.realitySettings) {
        form.reality_server_name = stream.realitySettings.serverName || ''
        form.reality_public_key = stream.realitySettings.password || stream.realitySettings.publicKey || ''
        form.reality_short_id = stream.realitySettings.shortId || ''
        form.reality_fingerprint = stream.realitySettings.fingerprint || 'chrome'
        form.reality_spider_x = stream.realitySettings.spiderX || '/'
      }
    } else if (form.protocol === 'hysteria') {
      if (stream.hysteriaSettings) {
        form.hy2_password = stream.hysteriaSettings.auth || ''
      }
      if (stream.tlsSettings) {
        form.hy2_sni = stream.tlsSettings.serverName || ''
        form.hy2_allow_insecure = !!stream.tlsSettings.allowInsecure
        form.hy2_pinned_peer_cert_sha256 = stream.tlsSettings.pinnedPeerCertSha256 || ''
        if (Array.isArray(stream.tlsSettings.certificates)) {
          const caCert = stream.tlsSettings.certificates.find((c: any) => c?.usage === 'verify')
          form.hy2_ca_cert_file = caCert?.certificateFile || ''
        }
      }
    } else if (form.protocol === 'socks') {
      form.socks_tls = stream.security === 'tls'
      if (stream.tlsSettings) {
        form.socks_sni = stream.tlsSettings.serverName || ''
      }
    } else if (form.protocol === 'http') {
      form.http_tls = stream.security === 'tls'
      if (stream.tlsSettings) {
        form.http_sni = stream.tlsSettings.serverName || ''
      }
    }
  } catch { /* ignore */ }
}

onMounted(() => {
  loadRefTargets().then(() => {
    parseExisting()
    visible.value = true
  })
})

async function save() {
  if (!form.tag?.trim()) { ElMessage.warning('请填写出站标签'); return }
  const lowerTag = form.tag?.trim().toLowerCase()
  if (!props.outbound && (lowerTag === 'direct' || lowerTag === 'blocked')) {
    ElMessage.warning('direct 与 blocked 为系统内置预留出站，请使用其他 Tag')
    return
  }

  // 场景与字段校验
  if (sceneMode.value === 'custom') {
    if (!form.outbound_json?.trim()) {
      ElMessage.warning('请填写原生 Outbound JSON')
      return
    }
    try {
      const obj = JSON.parse(form.outbound_json)
      if (typeof obj !== 'object' || Array.isArray(obj)) {
        ElMessage.warning('Outbound JSON 必须为合法的 JSON 对象 {...}')
        return
      }
    } catch (e: any) {
      ElMessage.error(`JSON 语法错误: ${e.message}`)
      return
    }
  } else if (sceneMode.value === 'inbound_ref') {
    // refInboundId 可选（可留空稍后在画布中连线）
  } else if (sceneMode.value === 'proxy') {
    if (form.protocol === 'vless' && (!form.vless_address || !form.vless_uuid)) {
      ElMessage.warning('VLESS 需填写远端地址与 UUID')
      return
    }
    if (form.protocol === 'hysteria' && (!form.hy2_address || !form.hy2_password)) {
      ElMessage.warning('Hysteria 2 需填写远端地址与认证密码')
      return
    }
    if (form.protocol === 'socks' && !form.socks_address) {
      ElMessage.warning('SOCKS5 需填写远端地址')
      return
    }
    if (form.protocol === 'http' && !form.http_address) {
      ElMessage.warning('HTTP 代理需填写远端地址')
      return
    }
  }

  saving.value = true
  try {
    const payload: any = {
      tag: form.tag.trim(),
      protocol: form.protocol,
      send_through: form.send_through?.trim() || '',
      enabled: form.enabled,
      priority: form.priority,
      remark: form.remark || '',
    }

    if (sceneMode.value === 'custom') {
      payload.protocol = 'custom'
      payload.outbound_json = form.outbound_json.trim()
      payload.settings_json = ''
      payload.stream_settings_json = ''
      payload.inbound_ref = undefined
    } else if (sceneMode.value === 'inbound_ref') {
      payload.inbound_ref = refInboundId.value || undefined
      payload.tls_verify_mode = form.tls_verify_mode || ''
      payload.settings_json = ''
      payload.stream_settings_json = ''
      payload.outbound_json = ''
    } else {
      payload.inbound_ref = undefined
      payload.outbound_json = ''
      payload.settings_json = buildSettingsJSON()
      payload.stream_settings_json = buildStreamJSON()
    }

    const { data } = props.outbound
      ? await updateServerOutbound(props.serverId, props.outbound.id, payload)
      : await createServerOutbound(props.serverId, payload)

    if (data.code === 0) {
      ElMessage.success(props.outbound ? '出站已更新' : '出站已创建')
      emit('saved')
      visible.value = false
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

function onClosed() { emit('close') }

async function copyText(text: string, label: string) {
  try { await navigator.clipboard.writeText(text); ElMessage.success(`${label}已复制`) }
  catch { ElMessage.warning('复制失败') }
}

function generateUUID() {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    form.vless_uuid = crypto.randomUUID()
    ElMessage.success('已生成新 UUID')
  } else {
    form.vless_uuid = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = (Math.random() * 16) | 0
      const v = c === 'x' ? r : (r & 0x3) | 0x8
      return v.toString(16)
    })
    ElMessage.success('已生成新 UUID')
  }
}
</script>
<template>
  <el-dialog
    :model-value="visible"
    :title="outbound ? `编辑出站规则 · ${outbound.tag}` : '新增出站规则'"
    :width="'min(960px, 92vw)'"
    @closed="onClosed"
  >
    <el-alert
      v-if="isSystemReserved"
      type="info"
      :closable="false"
      show-icon
      title="系统内置核心出站（Tag 与协议已锁定，可调整内部策略与备注）"
      style="margin-bottom: 14px"
    />

    <el-form label-position="top">
      <!-- 场景模式分段选择卡片 -->
      <div class="scene-selector-container">
        <div class="scene-selector-label">出站业务场景模式 (Scene Mode)</div>
        <div class="scene-card-grid">
          <div
            class="scene-option-card"
            :class="{ active: sceneMode === 'basic', disabled: isSystemReserved }"
            @click="!isSystemReserved && (sceneMode = 'basic', onSceneChange('basic'))"
          >
            <div class="scene-card-head">
              <span class="x-dot emerald"></span>
              <span class="scene-title">基础出口 (Basic)</span>
            </div>
            <div class="scene-desc">Freedom 直连 / Blackhole 阻断</div>
            <div class="scene-badge font-mono-num">direct / blocked</div>
          </div>

          <div
            class="scene-option-card"
            :class="{ active: sceneMode === 'inbound_ref', disabled: isSystemReserved }"
            @click="!isSystemReserved && (sceneMode = 'inbound_ref', onSceneChange('inbound_ref'))"
          >
            <div class="scene-card-head">
              <span class="x-dot purple"></span>
              <span class="scene-title">引用落地入站 (Ref)</span>
            </div>
            <div class="scene-desc">跨机中继拓扑连线 · 自动对账</div>
            <div class="scene-badge font-mono-num">InboundRef</div>
          </div>

          <div
            class="scene-option-card"
            :class="{ active: sceneMode === 'proxy', disabled: isSystemReserved }"
            @click="!isSystemReserved && (sceneMode = 'proxy', onSceneChange('proxy'))"
          >
            <div class="scene-card-head">
              <span class="x-dot blue"></span>
              <span class="scene-title">外部代理节点 (Proxy)</span>
            </div>
            <div class="scene-desc">VLESS / Hy2 / Mixed / Socks</div>
            <div class="scene-badge font-mono-num">订阅导出 + 链接互转</div>
          </div>

          <div
            class="scene-option-card"
            :class="{ active: sceneMode === 'custom', disabled: isSystemReserved }"
            @click="!isSystemReserved && (sceneMode = 'custom', onSceneChange('custom'))"
          >
            <div class="scene-card-head">
              <span class="x-dot amber"></span>
              <span class="scene-title">原生 JSON (Custom)</span>
            </div>
            <div class="scene-desc">WireGuard / 自定义 Outbound</div>
            <div class="scene-badge font-mono-num">Raw Xray Object</div>
          </div>
        </div>
      </div>

      <!-- 1. 公共元数据微卡片 -->
      <div class="x-micro-card" style="margin-bottom: 14px">
        <div class="x-micro-card-head">
          <div class="x-micro-card-title">
            <span class="x-dot blue"></span>
            <span>公共元数据与网络属性</span>
            <span class="x-sub-label">出站 Tag 与网络出口标识</span>
          </div>
          <div class="card-head-action">
            <span class="action-label">启用此出站规则</span>
            <el-switch v-model="form.enabled" />
          </div>
        </div>
        <div class="form-grid-2">
          <el-form-item>
            <template #label>
              <div class="field-label">
                <span>出站标签 Tag</span>
                <span class="required-star">*</span>
                <el-tooltip content="该出站规则的唯一标识，如 direct、proxy-out、via-japan 等。" placement="top">
                  <span class="help-icon">?</span>
                </el-tooltip>
              </div>
            </template>
            <el-input v-model="form.tag" :disabled="isSystemReserved" class="font-mono-num" placeholder="如 proxy-out / via-japan" />
          </el-form-item>

          <el-form-item>
            <template #label>
              <div class="field-label">
                <span>发送 IP (sendThrough)</span>
                <el-tooltip content="多 IP 服务器可指定此出站从哪个网卡出口 IP 发送流量，留空为默认出口。" placement="top">
                  <span class="help-icon">?</span>
                </el-tooltip>
              </div>
            </template>
            <el-input v-model="form.send_through" class="font-mono-num" placeholder="留空使用系统默认 IP" />
          </el-form-item>

          <el-form-item>
            <template #label>
              <div class="field-label">
                <span>优先级 Priority</span>
                <el-tooltip content="数值越小在 Xray outbounds 数组中排位越靠前，影响默认出口排序。" placement="top">
                  <span class="help-icon">?</span>
                </el-tooltip>
              </div>
            </template>
            <el-input-number v-model="form.priority" :min="0" class="font-mono-num" style="width: 100%" />
          </el-form-item>

          <el-form-item label="备注说明">
            <el-input v-model="form.remark" placeholder="选填，如 默认直连出口 / 日本落地中转" />
          </el-form-item>
        </div>
      </div>

      <!-- ===== 场景 1: 基础出口 (Freedom / Blackhole) ===== -->
      <template v-if="sceneMode === 'basic'">
        <div class="x-micro-card" style="margin-bottom: 14px">
          <div class="x-micro-card-head">
            <div class="x-micro-card-title">
              <span class="x-dot emerald"></span>
              <span>基础出口协议与策略</span>
            </div>
            <span class="x-chip emerald font-mono-num">{{ form.protocol.toUpperCase() }}</span>
          </div>
          <div class="form-grid-2">
            <el-form-item label="出口协议" :class="{ 'col-span-2': form.protocol === 'blackhole' }">
              <el-select v-model="form.protocol" :disabled="isSystemReserved" style="width: 100%">
                <el-option v-for="opt in BASIC_PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>

            <template v-if="form.protocol === 'freedom'">
              <el-form-item>
                <template #label>
                  <div class="field-label">
                    <span>域名解析策略 domainStrategy</span>
                    <el-tooltip content="该 freedom 出站在连接阶段对目标域名的解析方式。AsIs=按原域名连接；UseIP=解析为 IP 后连接；UseIPv4/UseIPv6=仅解析对应族。" placement="top">
                      <span class="help-icon">?</span>
                    </el-tooltip>
                  </div>
                </template>
                <el-select v-model="form.domain_strategy" style="width: 100%">
                  <el-option v-for="opt in DOMAIN_STRATEGY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
              </el-form-item>

              <el-form-item label="透明代理重定向 (redirect)" class="col-span-2">
                <el-input v-model="form.redirect" class="font-mono-num" placeholder="如 127.0.0.1:3366（选填）" />
              </el-form-item>
            </template>

            <template v-if="form.protocol === 'blackhole'">
              <el-form-item label="阻断响应类型" class="col-span-2">
                <el-select v-model="form.response_type" style="width: 100%">
                  <el-option label="none（直接断开 TCP 连接，静默丢弃）" value="none" />
                  <el-option label="http（返回 HTTP 403 Forbidden）" value="http" />
                </el-select>
              </el-form-item>
            </template>
          </div>

          <!-- Freedom 紧凑两项安全开关 -->
          <div v-if="form.protocol === 'freedom'" class="compact-switches-grid">
            <div class="compact-switch-card">
              <div class="switch-card-info">
                <div class="switch-card-title">屏蔽内网私有 IP (block_private)</div>
                <div class="switch-card-sub">拦截访问 10.x / 172.16.x / 192.168.x 等局域网私网</div>
              </div>
              <el-switch v-model="form.block_private" />
            </div>

            <div class="compact-switch-card">
              <div class="switch-card-info">
                <div class="switch-card-title">阻断回国流量 (block_cn)</div>
                <div class="switch-card-sub">阻断走本出站的大陆 IP 段 (geoip:cn)，防止海外节点被滥用代理回国</div>
              </div>
              <el-switch v-model="form.block_cn" />
            </div>
          </div>
        </div>
      </template>

      <!-- ===== 场景 2: 引用落地入站 (InboundRef 拓扑连线) ===== -->
      <template v-if="sceneMode === 'inbound_ref'">
        <div class="x-micro-card" style="margin-bottom: 14px">
          <div class="x-micro-card-head">
            <div class="x-micro-card-title">
              <span class="x-dot purple"></span>
              <span>引用目标落地入站 (拓扑级联)</span>
            </div>
            <span class="x-chip purple font-mono-num">AUTO RELAY PEER</span>
          </div>
          <div class="form-grid-2">
            <el-form-item label="目标落地服务器（选填）">
              <el-select v-model="refServerId" style="width: 100%" clearable placeholder="留空可在画布中拖线连接" @change="refInboundId = 0">
                <el-option :value="0" label="（稍后在拓扑画布中拖线连接）" />
                <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="目标落地入站（选填）">
              <el-select v-model="refInboundId" style="width: 100%" clearable placeholder="留空可在画布中拖线连接" @change="onRefInboundChange">
                <el-option :value="0" label="（稍后在拓扑画布中拖线连接）" />
                <el-option v-for="i in refInboundOptions" :key="i.id" :label="inboundLabel(i)" :value="i.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="机间 TLS 验证方式" class="col-span-2">
              <el-radio-group v-model="form.tls_verify_mode" class="tls-verify-group">
                <el-radio-button value="">自动（目标绑定 CA 时优先 CA 验签，否则指纹 Pin）</el-radio-button>
                <el-radio-button value="ca">托管 CA 验签（内联 CA 根公钥，校验证书链）</el-radio-button>
                <el-radio-button value="pin">证书指纹 Pin（固定目标证书 SHA-256）</el-radio-button>
              </el-radio-group>
              <div class="field-hint" style="margin-top: 6px">
                若目标使用托管 CA 签发的证书，选择「托管 CA 验签」将自动内联 CA 根公钥并禁用系统根证书（严格防劫持）；若目标为独立自签证书，将使用证书指纹 Pin 验证。
              </div>
            </el-form-item>
          </div>
          <div class="compact-tip" style="margin-top: 12px">
            <b>拓扑中继说明：</b>系统已支持跨服务器引用 <b>VLESS、Hysteria 2、Mixed (SOCKS/HTTP)</b> 落地入站！
            在此处绑定目标落地入站（或保存后在拓扑画布中拖拽端点连线），配置生成器将自动打通对称机间凭据并防双计，无需手动维护密钥。
          </div>
        </div>
      </template>

      <!-- ===== 场景 3: 外部代理节点 (Proxy Node) ===== -->
      <template v-if="sceneMode === 'proxy'">
        <!-- 3.1 代理协议与订阅流转微卡片 -->
        <div class="x-micro-card" style="margin-bottom: 14px">
          <div class="x-micro-card-head">
            <div class="x-micro-card-title">
              <span class="x-dot indigo"></span>
              <span>代理协议与订阅流转</span>
            </div>
            <div style="display: flex; gap: 8px">
              <el-button size="small" type="primary" plain @click="openImportDialog">
                <el-icon><Download /></el-icon>&nbsp;从代理链接导入
              </el-button>
              <el-button size="small" text @click="exportNodeLink">
                <el-icon><Share /></el-icon>&nbsp;生成分享链接
              </el-button>
            </div>
          </div>
          <div class="form-grid-2">
            <el-form-item label="客户端代理协议" :class="{ 'col-span-2': form.protocol !== 'vless' }">
              <el-select v-model="form.protocol" style="width: 100%">
                <el-option v-for="opt in PROXY_PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>

            <el-form-item v-if="form.protocol === 'vless'" label="Flow（流控算法）">
              <el-select v-model="form.vless_flow" style="width: 100%" clearable placeholder="无流控">
                <el-option v-for="opt in FLOW_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>
          </div>
        </div>

        <!-- 3.2 远端服务器端点与凭据微卡片 -->
        <div class="x-micro-card" style="margin-bottom: 14px">
          <div class="x-micro-card-head">
            <div class="x-micro-card-title">
              <span class="x-dot blue"></span>
              <span>远端服务器端点与鉴权凭据</span>
            </div>
            <span class="x-chip blue font-mono-num">{{ form.protocol.toUpperCase() }} CREDENTIALS</span>
          </div>

          <!-- VLESS 端点凭据 -->
          <template v-if="form.protocol === 'vless'">
            <div class="form-grid-2">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.vless_address" class="font-mono-num" placeholder="如 proxy.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.vless_port" :min="1" :max="65535" class="font-mono-num" style="width: 100%" />
              </el-form-item>
              <el-form-item label="UUID（用户凭据）" class="col-span-2">
                <div style="display: flex; gap: 8px">
                  <el-input v-model="form.vless_uuid" class="font-mono-num" placeholder="xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" />
                  <el-button @click="generateUUID">换一个</el-button>
                  <el-button @click="copyText(form.vless_uuid, 'UUID')">
                    <el-icon><CopyDocument /></el-icon>&nbsp;复制
                  </el-button>
                </div>
              </el-form-item>
            </div>
          </template>

          <!-- Hysteria 2 端点凭据 -->
          <template v-if="form.protocol === 'hysteria'">
            <div class="form-grid-2">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.hy2_address" class="font-mono-num" placeholder="如 hy2.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="UDP 端口 Port">
                <el-input-number v-model="form.hy2_port" :min="1" :max="65535" class="font-mono-num" style="width: 100%" />
              </el-form-item>
              <el-form-item label="Auth 认证密码" class="col-span-2">
                <el-input v-model="form.hy2_password" show-password class="font-mono-num" placeholder="Hysteria 2 服务端连接密钥" />
              </el-form-item>
            </div>
          </template>

          <!-- SOCKS5 凭据 -->
          <template v-if="form.protocol === 'socks'">
            <div class="form-grid-2">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.socks_address" class="font-mono-num" placeholder="如 socks.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.socks_port" :min="1" :max="65535" class="font-mono-num" style="width: 100%" />
              </el-form-item>
              <el-form-item label="认证用户名 (User，选填)">
                <el-input v-model="form.socks_user" placeholder="留空表示无密码认证" />
              </el-form-item>
              <el-form-item label="认证密码 (Password，选填)">
                <el-input v-model="form.socks_password" show-password placeholder="认证密码" />
              </el-form-item>
              <div class="compact-switch-card col-span-2">
                <div class="switch-card-info">
                  <div class="switch-card-title">启用 TLS 加密 (SOCKS5 Over TLS)</div>
                  <div class="switch-card-sub">开启后在 SOCKS 握手之上套用 TLS 加密隧道</div>
                </div>
                <el-switch v-model="form.socks_tls" />
              </div>
              <el-form-item v-if="form.socks_tls" label="TLS SNI (serverName)" class="col-span-2">
                <el-input v-model="form.socks_sni" class="font-mono-num" placeholder="留空默认使用远端地址" />
              </el-form-item>
            </div>
          </template>

          <!-- HTTP 凭据 -->
          <template v-if="form.protocol === 'http'">
            <div class="form-grid-2">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.http_address" class="font-mono-num" placeholder="如 proxy.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.http_port" :min="1" :max="65535" class="font-mono-num" style="width: 100%" />
              </el-form-item>
              <el-form-item label="认证用户名 (User，选填)">
                <el-input v-model="form.http_user" placeholder="留空表示无需认证" />
              </el-form-item>
              <el-form-item label="认证密码 (Password，选填)">
                <el-input v-model="form.http_password" show-password placeholder="认证密码" />
              </el-form-item>
              <div class="compact-switch-card col-span-2">
                <div class="switch-card-info">
                  <div class="switch-card-title">启用 HTTPS 加密 (HTTP Over TLS)</div>
                  <div class="switch-card-sub">开启后使用 HTTPS 安全代理</div>
                </div>
                <el-switch v-model="form.http_tls" />
              </div>
              <el-form-item v-if="form.http_tls" label="TLS SNI (serverName)" class="col-span-2">
                <el-input v-model="form.http_sni" class="font-mono-num" placeholder="留空默认使用远端地址" />
              </el-form-item>
            </div>
          </template>
        </div>

        <!-- 3.3 底层传输与安全加密 (VLESS 专属微卡片) -->
        <template v-if="form.protocol === 'vless'">
          <div class="x-micro-card" style="margin-bottom: 14px">
            <div class="x-micro-card-head">
              <div class="x-micro-card-title">
                <span class="x-dot emerald"></span>
                <span>底层传输与安全加密 (Transport & Security)</span>
              </div>
              <span class="x-chip emerald font-mono-num">{{ form.vless_network.toUpperCase() }} + {{ form.vless_security.toUpperCase() }}</span>
            </div>
            <div class="form-grid-2">
              <el-form-item label="传输协议 (Network)">
                <el-select v-model="form.vless_network" style="width: 100%">
                  <el-option v-for="opt in NETWORK_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
              </el-form-item>
              <el-form-item label="安全类型 (Security)">
                <el-select v-model="form.vless_security" style="width: 100%">
                  <el-option v-for="opt in SECURITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
              </el-form-item>

              <!-- WS 专属 -->
              <template v-if="form.vless_network === 'ws'">
                <el-form-item label="WS Path"><el-input v-model="form.ws_path" class="font-mono-num" placeholder="/" /></el-form-item>
                <el-form-item label="WS Host（选填）"><el-input v-model="form.ws_host" class="font-mono-num" placeholder="留空默认" /></el-form-item>
              </template>

              <!-- XHTTP 专属 -->
              <template v-if="form.vless_network === 'xhttp'">
                <el-form-item label="XHTTP Mode">
                  <el-select v-model="form.xhttp_mode" style="width: 100%">
                    <el-option label="auto" value="auto" />
                    <el-option label="packet-up" value="packet-up" />
                    <el-option label="stream-up" value="stream-up" />
                    <el-option label="stream-one" value="stream-one" />
                  </el-select>
                </el-form-item>
                <el-form-item label="XHTTP Path"><el-input v-model="form.xhttp_path" class="font-mono-num" placeholder="/" /></el-form-item>
                <el-form-item label="XHTTP Host（选填）" class="col-span-2"><el-input v-model="form.xhttp_host" class="font-mono-num" placeholder="留空默认" /></el-form-item>
              </template>

              <!-- gRPC 专属 -->
              <template v-if="form.vless_network === 'grpc'">
                <el-form-item label="gRPC Service Name"><el-input v-model="form.grpc_service_name" class="font-mono-num" placeholder="grpc" /></el-form-item>
                <el-form-item label="gRPC Authority（选填）"><el-input v-model="form.grpc_authority" class="font-mono-num" placeholder="留空默认" /></el-form-item>
              </template>

              <!-- TLS 专属 -->
              <template v-if="form.vless_security === 'tls'">
                <el-form-item label="SNI (serverName)"><el-input v-model="form.tls_server_name" class="font-mono-num" placeholder="如 example.com" /></el-form-item>
                <el-form-item label="uTLS Fingerprint">
                  <el-select v-model="form.vless_fingerprint" style="width: 100%">
                    <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                  </el-select>
                </el-form-item>
              </template>

              <!-- REALITY 专属 -->
              <template v-if="form.vless_security === 'reality'">
                <el-form-item label="目标域名 ServerName (SNI)">
                  <el-input v-model="form.reality_server_name" class="font-mono-num" placeholder="如 gateway.icloud.com" />
                </el-form-item>
                <el-form-item>
                  <template #label>
                    <div class="field-label">
                      <span>PublicKey（公钥）</span>
                      <span class="required-star">*</span>
                      <el-button size="small" text type="primary" :loading="genKeyLoading" @click="fetchRealityKeys">生成密钥对</el-button>
                    </div>
                  </template>
                  <el-input v-model="form.reality_public_key" class="font-mono-num" placeholder="x25519 对应公钥" />
                </el-form-item>
                <el-form-item>
                  <template #label>
                    <div class="field-label">
                      <span>ShortId（简短标识）</span>
                      <el-button size="small" text type="primary" @click="genShortId">随机生成</el-button>
                    </div>
                  </template>
                  <el-input v-model="form.reality_short_id" class="font-mono-num" placeholder="十六进制 shortId，如 6ba7b810" />
                </el-form-item>
                <el-form-item label="uTLS Fingerprint">
                  <el-select v-model="form.reality_fingerprint" style="width: 100%">
                    <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                  </el-select>
                </el-form-item>
                <el-form-item label="SpiderX 爬虫伪装路径" class="col-span-2">
                  <el-input v-model="form.reality_spider_x" class="font-mono-num" placeholder="如 / 或 /dl/" />
                </el-form-item>
              </template>
            </div>
          </div>
        </template>

        <!-- 3.4 Hysteria 2 TLS 与高级参数微卡片 -->
        <template v-if="form.protocol === 'hysteria'">
          <div class="x-micro-card" style="margin-bottom: 14px">
            <div class="x-micro-card-head">
              <div class="x-micro-card-title">
                <span class="x-dot cyan"></span>
                <span>Hysteria 2 TLS 与验证参数</span>
              </div>
            </div>
            <div class="form-grid-2">
              <el-form-item label="SNI (serverName)">
                <el-input v-model="form.hy2_sni" class="font-mono-num" placeholder="如 hy2.example.com" />
              </el-form-item>
              <el-form-item label="uTLS 客户端指纹">
                <el-select v-model="form.hy2_fingerprint" style="width: 100%">
                  <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                </el-select>
              </el-form-item>
              <el-form-item label="证书指纹 (Pin SHA-256)">
                <el-input v-model="form.hy2_pinned_peer_cert_sha256" class="font-mono-num" placeholder="远端证书 64位 Hex 散列" clearable />
              </el-form-item>
              <el-form-item label="CA 证书路径 (certificateFile)">
                <el-input v-model="form.hy2_ca_cert_file" class="font-mono-num" placeholder="如 /etc/ssl/certs/ca.crt" clearable />
              </el-form-item>
              <div class="compact-switch-card col-span-2">
                <div class="switch-card-info">
                  <div class="switch-card-title">允许不安全证书 (allowInsecure)</div>
                  <div class="switch-card-sub">允许跳过 TLS 服务端证书链校验（仅测试时使用）</div>
                </div>
                <el-switch v-model="form.hy2_allow_insecure" />
              </div>
            </div>
          </div>
        </template>

        <!-- 3.5 VLESS 高级 TLS 证书验签微卡片 (当 VLESS + TLS 时) -->
        <template v-if="form.protocol === 'vless' && form.vless_security === 'tls'">
          <div class="x-micro-card" style="margin-bottom: 14px; border-style: dashed;">
            <div class="x-micro-card-head">
              <div class="x-micro-card-title">
                <span class="x-dot amber"></span>
                <span>高级 TLS 证书防劫持（CA 根公钥 / 证书指纹 Pin）</span>
              </div>
              <span class="x-chip amber">Phase 60/61 扩展</span>
            </div>
            <div class="form-grid-2">
              <el-form-item label="证书指纹 (Pin SHA-256)">
                <el-input v-model="form.tls_pinned_peer_cert_sha256" class="font-mono-num" placeholder="远端证书 64位 Hex 散列" clearable />
              </el-form-item>
              <el-form-item label="CA 证书路径 (certificateFile)">
                <el-input v-model="form.tls_ca_cert_file" class="font-mono-num" placeholder="如 /etc/ssl/certs/xpanel_ca.crt" clearable />
              </el-form-item>
              <div class="compact-switch-card">
                <div class="switch-card-info">
                  <div class="switch-card-title">禁用系统根证书库 (disableSystemRoot)</div>
                  <div class="switch-card-sub">仅信任内联 CA 证书，杜绝第三方系统 CA 假冒</div>
                </div>
                <el-switch v-model="form.tls_disable_system_root" />
              </div>
              <div class="compact-switch-card">
                <div class="switch-card-info">
                  <div class="switch-card-title">允许不安全证书 (allowInsecure)</div>
                  <div class="switch-card-sub">生成配置时将自动转换为证书指纹验证</div>
                </div>
                <el-switch v-model="form.tls_allow_insecure" />
              </div>
            </div>
          </div>
        </template>
      </template>

      <!-- ===== 场景 4: 原生自定义 JSON ===== -->
      <template v-if="sceneMode === 'custom'">
        <div class="x-micro-card" style="margin-bottom: 14px">
          <div class="x-micro-card-head">
            <div class="x-micro-card-title">
              <span class="x-dot amber"></span>
              <span>原生 Xray 出站对象 JSON (Custom Mode)</span>
            </div>
            <span class="x-chip amber font-mono-num">RAW XRAY OBJECT</span>
          </div>
          <CodeEditor
            v-model="form.outbound_json"
            language="json"
            height="260px"
            :placeholder="OUTBOUND_JSON_PLACEHOLDER"
          />
          <div class="compact-tip" style="margin-top: 10px">
            <b>高级自定义：</b>填写完整的 Xray outbound JSON 对象。配置生成器将直接透传该对象，支持
            WireGuard、Shadowsocks、WARP 等任意原生出站协议，且会自动确保 tag 与上方设置对齐。
          </div>
        </div>
      </template>
    </el-form>

    <!-- 从链接导入弹窗 -->
    <el-dialog
      v-model="importDialogVisible"
      title="从代理链接导入"
      width="560px"
      append-to-body
    >
      <div style="font-size: 13px; color: var(--x-text-2); margin-bottom: 8px">
        粘贴节点订阅分享链接（支持 vless://, hysteria2://, hy2://, socks5://, http:// 等）：
      </div>
      <el-input
        v-model="importLinkText"
        type="textarea"
        :rows="5"
        placeholder="vless://... 或 hysteria2://..."
      />
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <el-button @click="importDialogVisible = false">取消</el-button>
          <el-button type="primary" :disabled="!importLinkText.trim()" @click="doImportLink">
            解析并填入表单
          </el-button>
        </div>
      </template>
    </el-dialog>

    <template #footer>
      <div style="display: flex; justify-content: space-between; align-items: center">
        <span class="muted" style="font-size: 12px">保存后主控自动编译配置并推送到服务器</span>
        <div style="display: flex; gap: 10px">
          <el-button @click="visible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="save">
            <el-icon><Check /></el-icon>&nbsp;保存出站
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.scene-selector-container {
  margin-bottom: 14px;
}
.scene-selector-label {
  font-size: 11px;
  font-weight: 700;
  color: var(--x-text-muted, #64748b);
  text-transform: uppercase;
  margin-bottom: 8px;
  letter-spacing: 0.5px;
}
.scene-card-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}
@media (max-width: 768px) {
  .scene-card-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
.scene-option-card {
  background: var(--x-card-bg, #fff);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 8px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 72px;

  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
    transform: translateY(-1px);
  }
  &.active {
    background: var(--x-primary-soft, rgba(59, 130, 246, 0.08));
    border: 1.5px solid var(--x-primary, #3b82f6);
    .scene-title {
      color: var(--x-primary, #3b82f6);
    }
  }
  &.disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
}
.scene-card-head {
  display: flex;
  align-items: center;
  gap: 6px;
}
.scene-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--x-text-1, #0f172a);
}
.scene-desc {
  font-size: 10.5px;
  color: var(--x-text-muted, #64748b);
  margin-top: 4px;
  line-height: 1.35;
}
.scene-badge {
  font-size: 10px;
  color: var(--x-text-3, #94a3b8);
  margin-top: 6px;
}

/* 规范业务分组微卡片 */
.x-micro-card {
  background: var(--x-card-bg, #fff);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 8px;
  padding: 14px 16px;
  transition: border-color 0.15s ease;
  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
  }
}
.x-micro-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px dashed var(--x-border, #e2e8f0);
}
.x-micro-card-title {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--x-text-1, #0f172a);
}
.x-sub-label {
  font-size: 11px;
  font-weight: normal;
  color: var(--x-text-muted, #64748b);
  margin-left: 4px;
}
.card-head-action {
  display: flex;
  align-items: center;
  gap: 8px;
}
.action-label {
  font-size: 12px;
  color: var(--x-text-2, #334155);
}

.x-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  &.blue { background: #3b82f6; box-shadow: 0 0 6px rgba(59, 130, 246, 0.5); }
  &.emerald { background: #10b981; box-shadow: 0 0 6px rgba(16, 185, 129, 0.5); }
  &.purple { background: #8b5cf6; box-shadow: 0 0 6px rgba(139, 92, 246, 0.5); }
  &.amber { background: #f59e0b; box-shadow: 0 0 6px rgba(245, 158, 11, 0.5); }
  &.indigo { background: #6366f1; box-shadow: 0 0 6px rgba(99, 102, 241, 0.5); }
  &.cyan { background: #06b6d4; box-shadow: 0 0 6px rgba(6, 182, 212, 0.5); }
}

.x-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 10.5px;
  font-weight: 500;
  line-height: 1.35;
  border: 1px solid transparent;
  &.blue { background: rgba(59, 130, 246, 0.1); color: #3b82f6; border-color: rgba(59, 130, 246, 0.25); }
  &.emerald { background: rgba(16, 185, 129, 0.1); color: #10b981; border-color: rgba(16, 185, 129, 0.25); }
  &.purple { background: rgba(139, 92, 246, 0.1); color: #8b5cf6; border-color: rgba(139, 92, 246, 0.25); }
  &.amber { background: rgba(245, 158, 11, 0.1); color: #f59e0b; border-color: rgba(245, 158, 11, 0.25); }
}

.form-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 16px;
}
@media (max-width: 640px) {
  .form-grid-2 {
    grid-template-columns: 1fr;
  }
}
.col-span-2 {
  grid-column: 1 / -1;
}

.field-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 500;
}
.required-star {
  color: var(--el-color-danger, #ef4444);
  font-weight: bold;
}
.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 13px;
  height: 13px;
  border-radius: 50%;
  background: var(--x-border, #e2e8f0);
  color: var(--x-text-3, #94a3b8);
  font-size: 10px;
  cursor: help;
}

.compact-switches-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
@media (max-width: 640px) {
  .compact-switches-grid {
    grid-template-columns: 1fr;
  }
}
.compact-switch-card {
  background: var(--x-bg, #f8fafc);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 6px;
  padding: 8px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.switch-card-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.switch-card-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--x-text-1, #0f172a);
}
.switch-card-sub {
  font-size: 10.5px;
  color: var(--x-text-muted, #64748b);
}

.compact-tip {
  font-size: 11.5px;
  color: var(--x-text-2, #334155);
  line-height: 1.5;
  background: var(--x-primary-soft, rgba(59, 130, 246, 0.08));
  border-left: 3px solid var(--x-primary, #3b82f6);
  padding: 8px 12px;
  border-radius: 0 6px 6px 0;
}
.field-hint {
  font-size: 11px;
  color: var(--x-text-muted, #64748b);
  line-height: 1.4;
}
.tls-verify-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.muted {
  color: var(--x-text-3, #94a3b8);
}
</style>
