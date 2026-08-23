<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Check, MagicStick, Refresh, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
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

// ===== Phase T：连接方式（仅 vless）——手动配置 / 引用落地入站 =====
const refMode = ref(false)
const refServerId = ref<number>(0)
const refInboundId = ref<number>(0)
const servers = ref<ServerItem[]>([])
const allInbounds = ref<InboundItem[]>([])

// 选中服务器的可引用入站（relay 优先展示）
const refInboundOptions = computed(() => {
  const list = allInbounds.value.filter((i) => i.server_id === refServerId.value && i.enabled)
  const sorted = [...list].sort((a, b) => {
    const rank = (t?: string) => (t === 'relay' ? 0 : t === 'user' ? 1 : 2)
    return rank(a.type) - rank(b.type)
  })
  return sorted
})

function inboundLabel(i: InboundItem) {
  const typeLabel = i.type === 'relay' ? '转发' : '用户'
  let net = '—'
  try {
    const s = JSON.parse(i.stream_settings || '{}')
    net = `${s.network || 'tcp'}/${s.security || 'none'}`
  } catch { /* ignore */ }
  return `${i.tag}（${typeLabel} · vless+${net}）`
}

async function loadRefTargets() {
  try {
    const [s, i] = await Promise.all([getServers(), getInbounds()])
    if (s.data.code === 0) servers.value = s.data.data.items
    if (i.data.code === 0) allInbounds.value = i.data.data.items
  } catch { /* 引用选择器加载失败不阻塞 */ }
}

// ===== 协议选项（去掉 socks/vmess） =====
const PROTOCOL_OPTIONS = [
  { label: 'freedom (直连)', value: 'freedom' },
  { label: 'blackhole (阻断)', value: 'blackhole' },
  { label: 'vless (代理链)', value: 'vless' },
]

// ===== 表单 =====
const form = reactive({
  tag: '',
  protocol: 'freedom' as string,
  enabled: true,
  priority: 0,
  remark: '',
  send_through: '',
  // freedom 子字段
  domain_strategy: 'AsIs' as string,
  block_private: true,
  block_cn: false,
  redirect: '',
  // blackhole 子字段
  response_type: 'none' as string,
  // vless 子字段
  vless_address: '',
  vless_port: 443,
  vless_uuid: '',
  vless_flow: '' as string,
  vless_encryption: 'none' as string,
  vless_network: 'tcp' as string,
  vless_security: 'none' as string,
  vless_fingerprint: 'chrome' as string,
  // vless 传输层
  ws_path: '/' as string,
  ws_host: '' as string,
  xhttp_mode: 'auto' as string,
  xhttp_path: '/' as string,
  xhttp_host: '' as string,
  grpc_service_name: 'grpc' as string,
  grpc_authority: '' as string,
  grpc_multi_mode: false,
  // vless 安全层
  tls_server_name: '' as string,
  tls_allow_insecure: false,
  reality_server_name: '' as string,
  reality_public_key: '' as string,
  reality_short_id: '' as string,
  reality_fingerprint: 'chrome' as string,
  reality_spider_x: '/' as string,
})

const FLOW_OPTIONS = [
  { label: '无', value: '' },
  { label: 'xtls-rprx-vision', value: 'xtls-rprx-vision' },
]

const ENCRYPTION_OPTIONS = [
  { label: 'none', value: 'none' },
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

// ===== JSON 构建 =====
function buildSettingsJSON(): string {
  switch (form.protocol) {
    case 'freedom': {
      const s: Record<string, any> = { domainStrategy: form.domain_strategy }
      if (form.redirect) s.redirect = form.redirect
      if (form.block_cn) s.block_cn = true
      if (form.block_private) {
        s.finalRules = [
          { action: 'block', ip: ['geoip:private'] },
          { action: 'allow' },
        ]
      }
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
    default:
      return '{}'
  }
}

function buildStreamJSON(): string {
  if (form.protocol !== 'vless') return ''
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
    s.tlsSettings = t
    if (form.vless_fingerprint) s.fingerprint = form.vless_fingerprint
  } else if (form.vless_security === 'reality') {
    // 出站 REALITY 公钥标准字段为 password（publicKey 是兼容旧名，见 01 号文档 §2.2）
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

// ===== 从已有 outbound 解析回表单 =====
function parseExisting() {
  if (!props.outbound) return
  form.tag = props.outbound.tag
  form.protocol = props.outbound.protocol
  form.enabled = props.outbound.enabled
  form.priority = props.outbound.priority
  form.remark = props.outbound.remark
  form.send_through = props.outbound.send_through || ''

  // Phase T：引用模式回填
  const ob = props.outbound
  if (ob?.inbound_ref) {
    refMode.value = true
    refInboundId.value = ob.inbound_ref
    const target = allInbounds.value.find((i) => i.id === ob.inbound_ref)
    if (target) refServerId.value = target.server_id
  }

  try {
    const settings = props.outbound.settings_json ? JSON.parse(props.outbound.settings_json) : {}
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
    }
  } catch { /* ignore parse errors */ }

  try {
    const stream = props.outbound.stream_settings_json ? JSON.parse(props.outbound.stream_settings_json) : {}
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
  if (form.protocol === 'vless' && !refMode.value && (!form.vless_address || !form.vless_uuid)) {
    ElMessage.warning('手动模式需填写远端地址和 UUID，或切换为「引用落地入站 / 画布拖线」')
    return
  }
  saving.value = true
  try {
    const payload = {
      tag: form.tag.trim(),
      protocol: form.protocol,
      settings_json: refMode.value ? '' : buildSettingsJSON(),
      stream_settings_json: refMode.value ? '' : buildStreamJSON(),
      send_through: form.send_through?.trim() || '',
      enabled: form.enabled,
      priority: form.priority,
      remark: form.remark || '',
      // Phase T：引用模式只发 inbound_ref（vnext/streamSettings 由生成器自动构造）
      inbound_ref: refMode.value ? (refInboundId.value || undefined) : undefined,
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

const isSystemReserved = computed(() => props.outbound?.tag === 'direct' || props.outbound?.tag === 'blocked')

async function copyText(text: string, label: string) {
  try { await navigator.clipboard.writeText(text); ElMessage.success(`${label}已复制`) }
  catch { ElMessage.warning('复制失败') }
}
const activeTab = ref('basic')
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="outbound ? `编辑出站规则 · ${outbound.tag}` : '新增出站规则'"
    width="720px"
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
      <el-tabs v-model="activeTab" class="editor-tabs">
        <!-- ===== TAB 1: 基础配置 ===== -->
        <el-tab-pane label="基础配置" name="basic">
          <div class="form-grid">
            <el-form-item>
              <template #label>
                <div class="field-label">
                  <span>协议 Protocol</span>
                  <el-tooltip content="freedom 用于节点自主连接外部互联网；blackhole 用于黑洞阻断；vless 用于跨节点中转代理链。" placement="top">
                    <span class="help-icon">?</span>
                  </el-tooltip>
                </div>
              </template>
              <el-select v-model="form.protocol" :disabled="isSystemReserved" style="width: 100%">
                <el-option v-for="opt in PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>

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

            <el-form-item label="备注说明" style="grid-column: 1 / -1">
              <el-input v-model="form.remark" placeholder="选填，如 默认直连出口 / 日本落地中转" />
            </el-form-item>
          </div>

          <!-- 启用出站 Toggle 卡片 -->
          <div class="toggle-card" style="margin-top: 4px">
            <div class="toggle-info">
              <span class="toggle-title">启用此出站规则</span>
              <span class="toggle-sub">开启后写入节点 Xray 配置中；停用则不参与流量转发</span>
            </div>
            <el-switch v-model="form.enabled" />
          </div>

          <!-- Freedom 专属参数 -->
          <template v-if="form.protocol === 'freedom'">
            <div class="card-box" style="margin-top: 14px">
              <div class="box-title">Freedom 直连参数</div>
              <div class="form-grid">
                <el-form-item label="域名解析策略 domainStrategy">
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
                  <span class="toggle-title">屏蔽内网私有 IP (block_private)</span>
                  <span class="toggle-sub">自动注入规则禁止访问 10.x / 172.16.x / 192.168.x 等局域网私有段</span>
                </div>
                <el-switch v-model="form.block_private" />
              </div>

              <!-- 阻止回国流量 Toggle -->
              <div class="toggle-card inner-toggle" style="margin-top: 8px">
                <div class="toggle-info">
                  <span class="toggle-title">阻止回国流量 (block_cn)</span>
                  <span class="toggle-sub">自动注入规则阻断访问大陆域名与 IP (geosite:cn / geoip:cn)，防止海外节点被滥用回国</span>
                </div>
                <el-switch v-model="form.block_cn" />
              </div>
            </div>
          </template>

          <!-- Blackhole 专属参数 -->
          <template v-if="form.protocol === 'blackhole'">
            <div class="card-box" style="margin-top: 14px">
              <div class="box-title">Blackhole 黑洞参数</div>
              <div class="form-grid">
                <el-form-item label="阻断响应类型" style="grid-column: 1 / -1">
                  <el-select v-model="form.response_type" style="width: 100%">
                    <el-option label="none（直接断开 TCP 连接，静默丢弃）" value="none" />
                    <el-option label="http（返回 HTTP 403 Forbidden）" value="http" />
                  </el-select>
                </el-form-item>
              </div>
            </div>
          </template>
        </el-tab-pane>

        <!-- ===== TAB 2: VLESS 远端与安全（仅 vless 展示） ===== -->
        <el-tab-pane v-if="form.protocol === 'vless'" label="远端与传输安全" name="vless_remote">
          <!-- 连接方式切换 -->
          <div style="margin-bottom: 14px">
            <el-radio-group v-model="refMode">
              <el-radio-button :value="false">手动配置远端 (Manual)</el-radio-button>
              <el-radio-button :value="true">引用落地入站 (InboundRef 拓扑)</el-radio-button>
            </el-radio-group>
          </div>

          <!-- 引用落地入站模式 -->
          <template v-if="refMode">
            <div class="card-box">
              <div class="box-title">引用目标落地服务器</div>
              <div class="form-grid">
                <el-form-item label="目标落地服务器（选填）">
                  <el-select v-model="refServerId" style="width: 100%" clearable placeholder="留空可在画布中拖线连接" @change="refInboundId = 0">
                    <el-option :value="0" label="（稍后在拓扑画布中拖线连接）" />
                    <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
                  </el-select>
                </el-form-item>
                <el-form-item label="目标落地入站（选填）">
                  <el-select v-model="refInboundId" style="width: 100%" clearable placeholder="留空可在画布中拖线连接">
                    <el-option :value="0" label="（稍后在拓扑画布中拖线连接）" />
                    <el-option v-for="i in refInboundOptions" :key="i.id" :label="inboundLabel(i)" :value="i.id" />
                  </el-select>
                </el-form-item>
              </div>
              <div class="tip-banner" style="margin-top: 10px">
                提示：在此处选择落地入站，或直接留空保存并在<b>拓扑画布中拖拽端点连线</b>，系统将自动绑定落地并生成合法中转凭据。
              </div>
            </div>
          </template>

          <!-- 手动配置模式 -->
          <template v-else>
            <div class="card-box">
              <div class="box-title">远端节点凭据</div>
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
            </div>

            <div class="card-box" style="margin-top: 12px">
              <div class="box-title">传输与安全协议</div>
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

              <!-- WebSocket 动态参数 -->
              <div v-if="form.vless_network === 'ws'" class="form-grid" style="margin-top: 10px">
                <el-form-item label="WS Path"><el-input v-model="form.ws_path" placeholder="/" /></el-form-item>
                <el-form-item label="WS Host（选填）"><el-input v-model="form.ws_host" placeholder="留空默认" /></el-form-item>
              </div>

              <!-- XHTTP 动态参数 -->
              <div v-if="form.vless_network === 'xhttp'" class="form-grid" style="margin-top: 10px">
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

              <!-- gRPC 动态参数 -->
              <div v-if="form.vless_network === 'grpc'" class="form-grid" style="margin-top: 10px">
                <el-form-item label="gRPC Service Name"><el-input v-model="form.grpc_service_name" placeholder="grpc" /></el-form-item>
                <el-form-item label="gRPC Authority（选填）"><el-input v-model="form.grpc_authority" placeholder="留空默认" /></el-form-item>
              </div>

              <!-- TLS 动态参数 -->
              <div v-if="form.vless_security === 'tls'" class="form-grid" style="margin-top: 10px">
                <el-form-item label="SNI (serverName)"><el-input v-model="form.tls_server_name" placeholder="如 example.com" /></el-form-item>
                <el-form-item label="uTLS Fingerprint">
                  <el-select v-model="form.vless_fingerprint" style="width: 100%">
                    <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
                  </el-select>
                </el-form-item>
                <div class="toggle-card inner-toggle" style="grid-column: 1 / -1">
                  <div class="toggle-info">
                    <span class="toggle-title">跳过证书校验 (allowInsecure)</span>
                    <span class="toggle-sub">允许使用自签名或域名不匹配的证书（生产环境慎用）</span>
                  </div>
                  <el-switch v-model="form.tls_allow_insecure" />
                </div>
              </div>

              <!-- REALITY 动态参数 -->
              <div v-if="form.vless_security === 'reality'" class="form-grid" style="margin-top: 10px">
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
                  <el-input v-model="form.reality_public_key" placeholder="x25519 公钥" />
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
            </div>
          </template>
        </el-tab-pane>
      </el-tabs>
    </el-form>

    <template #footer>
      <div style="display: flex; justify-content: space-between; align-items: center">
        <span class="muted" style="font-size: 12px">保存后主控自动编译配置并推送到节点</span>
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
.editor-tabs {
  margin-top: -8px;
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
