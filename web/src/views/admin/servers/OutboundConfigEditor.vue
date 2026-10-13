<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, MagicStick, Refresh, CopyDocument, Download, Share } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import TipIcon from '@/components/base/TipIcon.vue'
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
    if (inb.protocol === 'mixed') {
      form.protocol = 'socks'
    } else {
      form.protocol = inb.protocol
    }
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
    const s: Record<string, any> = {
      network: 'hysteria',
      security: 'tls',
      tlsSettings: {
        serverName: form.hy2_sni || undefined,
        allowInsecure: form.hy2_allow_insecure || false,
      },
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
    const target = allInbounds.value.find((i) => i.id === ob.inbound_ref)
    if (target) {
      refServerId.value = target.server_id
      form.protocol = target.protocol === 'mixed' ? 'socks' : target.protocol
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

      if (stream.tlsSettings) { form.tls_server_name = stream.tlsSettings.serverName || ''; form.tls_allow_insecure = !!stream.tlsSettings.allowInsecure }
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
      <!-- 场景模式选择卡片 -->
      <div class="scene-selector-box" style="margin-bottom: 16px">
        <div class="box-title" style="margin-bottom: 8px">出站场景模式</div>
        <el-radio-group
          v-model="sceneMode"
          :disabled="isSystemReserved"
          size="default"
          style="width: 100%; display: flex; flex-wrap: wrap; gap: 8px"
          @change="onSceneChange"
        >
          <el-radio-button value="basic">基础出口 (Freedom / Blackhole)</el-radio-button>
          <el-radio-button value="inbound_ref">引用落地入站 (拓扑连线中转)</el-radio-button>
          <el-radio-button value="proxy">外部代理节点 (VLESS / Hy2 / Socks / HTTP)</el-radio-button>
          <el-radio-button value="custom">原生自定义 JSON</el-radio-button>
        </el-radio-group>
      </div>

      <!-- 基础公共字段 -->
      <div class="card-box" style="margin-bottom: 14px">
        <div class="box-title">公共元数据</div>
        <div class="form-grid">
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
            <el-input v-model="form.tag" :disabled="isSystemReserved" placeholder="如 proxy-out / via-japan" />
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
            <el-input v-model="form.send_through" placeholder="留空使用系统默认 IP" />
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
            <el-input-number v-model="form.priority" :min="0" style="width: 100%" />
          </el-form-item>

          <el-form-item label="备注说明">
            <el-input v-model="form.remark" placeholder="选填，如 默认直连出口 / 日本落地中转" />
          </el-form-item>
        </div>

        <div class="toggle-card" style="margin-top: 8px">
          <div class="toggle-info">
            <span class="toggle-title">启用此出站规则</span>
            <span class="toggle-sub">开启后写入服务器 Xray 配置中；停用则不参与流量转发</span>
          </div>
          <el-switch v-model="form.enabled" />
        </div>
      </div>

      <!-- ===== 场景 1: 基础出口 (Freedom / Blackhole) ===== -->
      <template v-if="sceneMode === 'basic'">
        <div class="card-box" style="margin-bottom: 14px">
          <div class="box-title">基础协议选择</div>
          <div class="form-grid">
            <el-form-item label="出口协议" style="grid-column: 1 / -1">
              <el-select v-model="form.protocol" :disabled="isSystemReserved" style="width: 100%">
                <el-option v-for="opt in BASIC_PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>
          </div>

          <!-- Freedom 专属参数 -->
          <template v-if="form.protocol === 'freedom'">
            <div class="form-grid" style="margin-top: 8px">
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
              <el-form-item label="透明代理重定向 (redirect)">
                <el-input v-model="form.redirect" placeholder="如 127.0.0.1:3366（选填）" />
              </el-form-item>
            </div>

            <!-- 屏蔽内网私有 IP Toggle -->
            <div class="toggle-card inner-toggle" style="margin-top: 8px">
              <div class="toggle-info">
                <span class="toggle-title">
                  屏蔽内网私有 IP (block_private)
                  <TipIcon content="开启后拦截访问 10.x / 172.16.x / 192.168.x 等局域网私有段（命中即断开）。关闭则显式放行私网。" />
                </span>
              </div>
              <el-switch v-model="form.block_private" />
            </div>

            <!-- 阻止回国流量 Toggle -->
            <div class="toggle-card inner-toggle" style="margin-top: 8px">
              <div class="toggle-info">
                <span class="toggle-title">
                  阻断回国流量 (block_cn)
                  <TipIcon content="阻断走本出站的大陆 IP 段 (geoip:cn)，防止海外服务器被滥用回国。" />
                </span>
              </div>
              <el-switch v-model="form.block_cn" />
            </div>
          </template>

          <!-- Blackhole 专属参数 -->
          <template v-if="form.protocol === 'blackhole'">
            <div class="form-grid" style="margin-top: 8px">
              <el-form-item label="阻断响应类型" style="grid-column: 1 / -1">
                <el-select v-model="form.response_type" style="width: 100%">
                  <el-option label="none（直接断开 TCP 连接，静默丢弃）" value="none" />
                  <el-option label="http（返回 HTTP 403 Forbidden）" value="http" />
                </el-select>
              </el-form-item>
            </div>
          </template>
        </div>
      </template>

      <!-- ===== 场景 2: 引用落地入站 (InboundRef 拓扑连线) ===== -->
      <template v-if="sceneMode === 'inbound_ref'">
        <div class="card-box" style="margin-bottom: 14px">
          <div class="box-title">引用目标落地入站</div>
          <div class="form-grid">
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
          </div>
          <div class="tip-banner" style="margin-top: 10px">
            <b>拓扑中继说明：</b>系统已支持跨服务器引用 <b>VLESS、Hysteria 2、Mixed (SOCKS/HTTP)</b> 落地入站！
            在此处绑定目标落地入站（或保存后在拓扑画布中拖拽端点连线），配置生成器将自动打通对称机间凭据并防双计，无需手动维护密钥。
          </div>
        </div>
      </template>

      <!-- ===== 场景 3: 外部代理节点 (Proxy Node) ===== -->
      <template v-if="sceneMode === 'proxy'">
        <div class="card-box" style="margin-bottom: 14px">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
            <div class="box-title" style="margin-bottom: 0">代理协议与订阅流转</div>
            <div style="display: flex; gap: 8px">
              <el-button size="small" type="primary" plain @click="openImportDialog">
                <el-icon><Download /></el-icon>&nbsp;从代理链接导入
              </el-button>
              <el-button size="small" text @click="exportNodeLink">
                <el-icon><Share /></el-icon>&nbsp;生成分享链接
              </el-button>
            </div>
          </div>

          <div class="form-grid">
            <el-form-item label="客户端代理协议" style="grid-column: 1 / -1">
              <el-select v-model="form.protocol" style="width: 100%">
                <el-option v-for="opt in PROXY_PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>
          </div>

          <!-- 3.1 VLESS 配置表单 -->
          <template v-if="form.protocol === 'vless'">
            <div class="box-title" style="margin-top: 8px">远端服务器凭据</div>
            <div class="form-grid">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.vless_address" placeholder="如 proxy.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.vless_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
              <el-form-item label="UUID（用户 ID）">
                <el-input v-model="form.vless_uuid" placeholder="xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" />
              </el-form-item>
              <el-form-item label="Flow（流控）">
                <el-select v-model="form.vless_flow" style="width: 100%" clearable placeholder="无流控">
                  <el-option v-for="opt in FLOW_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
              </el-form-item>
            </div>

            <div class="box-title" style="margin-top: 14px">传输与安全协议</div>
            <div class="form-grid">
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
            </div>

            <!-- WS -->
            <div v-if="form.vless_network === 'ws'" class="form-grid" style="margin-top: 6px">
              <el-form-item label="WS Path"><el-input v-model="form.ws_path" placeholder="/" /></el-form-item>
              <el-form-item label="WS Host（选填）"><el-input v-model="form.ws_host" placeholder="留空默认" /></el-form-item>
            </div>

            <!-- XHTTP -->
            <div v-if="form.vless_network === 'xhttp'" class="form-grid" style="margin-top: 6px">
              <el-form-item label="XHTTP Mode">
                <el-select v-model="form.xhttp_mode" style="width: 100%">
                  <el-option label="auto" value="auto" />
                  <el-option label="packet-up" value="packet-up" />
                  <el-option label="stream-up" value="stream-up" />
                  <el-option label="stream-one" value="stream-one" />
                </el-select>
              </el-form-item>
              <el-form-item label="XHTTP Path"><el-input v-model="form.xhttp_path" placeholder="/" /></el-form-item>
              <el-form-item label="XHTTP Host（选填）" style="grid-column: 1 / -1"><el-input v-model="form.xhttp_host" placeholder="留空默认" /></el-form-item>
            </div>

            <!-- gRPC -->
            <div v-if="form.vless_network === 'grpc'" class="form-grid" style="margin-top: 6px">
              <el-form-item label="gRPC Service Name"><el-input v-model="form.grpc_service_name" placeholder="grpc" /></el-form-item>
              <el-form-item label="gRPC Authority（选填）"><el-input v-model="form.grpc_authority" placeholder="留空默认" /></el-form-item>
            </div>

            <!-- TLS -->
            <div v-if="form.vless_security === 'tls'" class="form-grid" style="margin-top: 6px">
              <el-form-item label="SNI (serverName)"><el-input v-model="form.tls_server_name" placeholder="如 example.com" /></el-form-item>
              <el-form-item label="uTLS Fingerprint">
                <el-select v-model="form.vless_fingerprint" style="width: 100%">
                  <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                </el-select>
              </el-form-item>
              <div class="toggle-card inner-toggle" style="grid-column: 1 / -1">
                <div class="toggle-info">
                  <span class="toggle-title">
                    跳过证书校验 (allowInsecure)
                    <TipIcon type="warn" content="允许使用自签名或域名不匹配的证书（生产环境慎用）。" />
                  </span>
                </div>
                <el-switch v-model="form.tls_allow_insecure" />
              </div>
            </div>

            <!-- REALITY -->
            <div v-if="form.vless_security === 'reality'" class="form-grid" style="margin-top: 6px">
              <el-form-item label="SNI (serverName)"><el-input v-model="form.reality_server_name" placeholder="如 www.apple.com" /></el-form-item>
              <el-form-item label="uTLS Fingerprint">
                <el-select v-model="form.reality_fingerprint" style="width: 100%">
                  <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                </el-select>
              </el-form-item>
              <el-form-item label="Short ID">
                <div style="display: flex; gap: 8px; width: 100%">
                  <el-input v-model="form.reality_short_id" placeholder="abcdef0123456789" />
                  <el-button @click="genShortId"><el-icon><Refresh /></el-icon></el-button>
                </div>
              </el-form-item>
              <el-form-item label="SpiderX 路径"><el-input v-model="form.reality_spider_x" placeholder="/" /></el-form-item>
              <el-form-item label="Public Key（服务端公钥）" style="grid-column: 1 / -1">
                <el-input v-model="form.reality_public_key" placeholder="x25519 公钥 (32 字节 base64url 格式)" />
              </el-form-item>
              <div style="grid-column: 1 / -1; display: flex; gap: 8px">
                <el-button size="small" type="primary" plain :loading="genKeyLoading" @click="fetchRealityKeys">
                  <el-icon><MagicStick /></el-icon>&nbsp;生成 x25519 密钥
                </el-button>
                <el-button v-if="form.reality_public_key" size="small" text @click="copyText(form.reality_public_key, '公钥')">
                  <el-icon><CopyDocument /></el-icon>&nbsp;复制公钥
                </el-button>
              </div>
            </div>
          </template>

          <!-- 3.2 Hysteria 2 配置表单 -->
          <template v-if="form.protocol === 'hysteria'">
            <div class="box-title" style="margin-top: 8px">Hysteria 2 远端与凭据</div>
            <div class="form-grid">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.hy2_address" placeholder="如 hy2.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.hy2_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
              <el-form-item label="认证密码 (Password)" style="grid-column: 1 / -1">
                <el-input v-model="form.hy2_password" show-password placeholder="目标 Hysteria 2 节点的认证密码" />
              </el-form-item>
              <el-form-item label="TLS SNI (serverName)">
                <el-input v-model="form.hy2_sni" placeholder="留空默认使用远端地址" />
              </el-form-item>
              <el-form-item label="uTLS Fingerprint">
                <el-select v-model="form.hy2_fingerprint" style="width: 100%">
                  <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                </el-select>
              </el-form-item>
              <div class="toggle-card inner-toggle" style="grid-column: 1 / -1">
                <div class="toggle-info">
                  <span class="toggle-title">
                    跳过证书校验 (allowInsecure)
                    <TipIcon type="warn" content="允许使用自签名或域名不匹配的证书。" />
                  </span>
                </div>
                <el-switch v-model="form.hy2_allow_insecure" />
              </div>
            </div>
          </template>

          <!-- 3.3 SOCKS5 配置表单 -->
          <template v-if="form.protocol === 'socks'">
            <div class="box-title" style="margin-top: 8px">SOCKS5 代理服务器凭据</div>
            <div class="form-grid">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.socks_address" placeholder="如 socks.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.socks_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
              <el-form-item label="认证用户名 (User，选填)">
                <el-input v-model="form.socks_user" placeholder="留空表示无密码认证" />
              </el-form-item>
              <el-form-item label="认证密码 (Password，选填)">
                <el-input v-model="form.socks_password" show-password placeholder="认证密码" />
              </el-form-item>
              <div class="toggle-card inner-toggle" style="grid-column: 1 / -1">
                <div class="toggle-info">
                  <span class="toggle-title">启用 TLS 加密 (SOCKS5 Over TLS)</span>
                </div>
                <el-switch v-model="form.socks_tls" />
              </div>
              <el-form-item v-if="form.socks_tls" label="TLS SNI" style="grid-column: 1 / -1">
                <el-input v-model="form.socks_sni" placeholder="留空默认使用远端地址" />
              </el-form-item>
            </div>
          </template>

          <!-- 3.4 HTTP 配置表单 -->
          <template v-if="form.protocol === 'http'">
            <div class="box-title" style="margin-top: 8px">HTTP / HTTPS 代理凭据</div>
            <div class="form-grid">
              <el-form-item label="远端地址 Address">
                <el-input v-model="form.http_address" placeholder="如 proxy.example.com 或 1.2.3.4" />
              </el-form-item>
              <el-form-item label="远端端口 Port">
                <el-input-number v-model="form.http_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
              <el-form-item label="认证用户名 (User，选填)">
                <el-input v-model="form.http_user" placeholder="留空表示无需认证" />
              </el-form-item>
              <el-form-item label="认证密码 (Password，选填)">
                <el-input v-model="form.http_password" show-password placeholder="认证密码" />
              </el-form-item>
              <div class="toggle-card inner-toggle" style="grid-column: 1 / -1">
                <div class="toggle-info">
                  <span class="toggle-title">启用 HTTPS 加密 (HTTP Over TLS)</span>
                </div>
                <el-switch v-model="form.http_tls" />
              </div>
              <el-form-item v-if="form.http_tls" label="TLS SNI" style="grid-column: 1 / -1">
                <el-input v-model="form.http_sni" placeholder="留空默认使用远端地址" />
              </el-form-item>
            </div>
          </template>
        </div>
      </template>

      <!-- ===== 场景 4: 原生自定义 JSON ===== -->
      <template v-if="sceneMode === 'custom'">
        <div class="card-box" style="margin-bottom: 14px">
          <div class="box-title">原生 Xray 出站对象 JSON</div>
          <CodeEditor
            v-model="form.outbound_json"
            language="json"
            height="260px"
            :placeholder="OUTBOUND_JSON_PLACEHOLDER"
          />
          <div class="tip-banner" style="margin-top: 10px">
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
.scene-selector-box {
  background: var(--x-card-bg, #fff);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px 14px;
}
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}
.field-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  font-weight: 500;
}
.required-star {
  color: var(--el-color-danger);
  font-weight: bold;
}
.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 13px;
  height: 13px;
  border-radius: 50%;
  background: var(--x-border);
  color: var(--x-text-3);
  font-size: 10px;
  cursor: help;
}
.card-box {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px 14px;
}
.box-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--x-primary);
  margin-bottom: 8px;
}
.toggle-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 10px 14px;
  &.inner-toggle {
    background: var(--x-card-bg, #fff);
  }
}
.toggle-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.toggle-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--x-text-1);
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.toggle-sub {
  font-size: 11.5px;
  color: var(--x-text-3);
}
.tip-banner {
  font-size: 12px;
  color: var(--x-text-2);
  line-height: 1.5;
  background: rgba(var(--x-primary-rgb, 59, 130, 246), 0.06);
  border-left: 3px solid var(--x-primary);
  padding: 6px 10px;
  border-radius: 0 4px 4px 0;
}
.muted {
  color: var(--x-text-3);
}
</style>
