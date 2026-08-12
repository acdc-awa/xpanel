<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Check, MagicStick, Refresh, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  createServerOutbound,
  updateServerOutbound,
  getXrayKeys,
  type ServerOutbound,
} from '@/api/admin'
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

// ===== 协议选项（去掉 socks/vmess） =====
const PROTOCOL_OPTIONS = [
  { label: 'freedom（直连）', value: 'freedom' },
  { label: 'blackhole（黑洞/屏蔽）', value: 'blackhole' },
  { label: 'vless（VLESS 代理链）', value: 'vless' },
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
  { label: 'none（无加密）', value: 'none' },
  { label: 'tls（TLS 加密）', value: 'tls' },
  { label: 'reality（伪装防封）', value: 'reality' },
]

const FINGERPRINT_OPTIONS = [
  'chrome', 'firefox', 'safari', 'edge', '360', 'qq', 'random', 'randomized',
]

const DOMAIN_STRATEGY_OPTIONS = [
  { label: 'AsIs（保持原样）', value: 'AsIs' },
  { label: 'UseIP（解析后连接）', value: 'UseIP' },
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

  try {
    const settings = props.outbound.settings_json ? JSON.parse(props.outbound.settings_json) : {}
    if (form.protocol === 'freedom') {
      form.domain_strategy = settings.domainStrategy || 'AsIs'
      form.block_private = Array.isArray(settings.finalRules) && settings.finalRules.some((r: any) => r.action === 'block' && r.ip?.includes?.('geoip:private'))
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
  parseExisting()
  visible.value = true
})

async function save() {
  if (!form.tag?.trim()) { ElMessage.warning('请填写出站标签'); return }
  if (form.protocol === 'vless' && (!form.vless_address || !form.vless_uuid)) {
    ElMessage.warning('VLESS 出站需填写远端地址和 UUID')
    return
  }
  saving.value = true
  try {
    const payload = {
      tag: form.tag.trim(),
      protocol: form.protocol,
      settings_json: buildSettingsJSON(),
      stream_settings_json: buildStreamJSON(),
      send_through: form.send_through?.trim() || '',
      enabled: form.enabled,
      priority: form.priority,
      remark: form.remark || '',
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
    :title="outbound ? '编辑出站规则' : '新增出站规则'"
    width="700px"
    @closed="onClosed"
  >
    <el-form label-position="top">
      <!-- ===== 基本配置 ===== -->
      <div class="sec-title">基本配置</div>
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="协议 Protocol">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option v-for="opt in PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="标签 Tag（必填）">
          <el-input v-model="form.tag" placeholder="如 direct / proxy-out / blocked" />
        </el-form-item>
        <el-form-item label="发送 IP（sendThrough，选填）">
          <el-input v-model="form.send_through" placeholder="指定出口 IP，如 1.2.3.4" />
        </el-form-item>
        <el-form-item label="优先级 Priority">
          <el-input-number v-model="form.priority" :min="0" style="width: 100%" />
        </el-form-item>
      </div>

      <!-- ===== Freedom 参数 ===== -->
      <template v-if="form.protocol === 'freedom'">
        <div class="sec-title">Freedom 直连参数</div>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="域名策略 domainStrategy">
            <el-select v-model="form.domain_strategy" style="width: 100%">
              <el-option v-for="opt in DOMAIN_STRATEGY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="屏蔽内网 IP（推荐开启）">
            <el-switch v-model="form.block_private" active-text="阻止访问内网" />
          </el-form-item>
          <el-form-item label="透明代理 redirect（选填）">
            <el-input v-model="form.redirect" placeholder="如 127.0.0.1:3366" />
          </el-form-item>
        </div>
        <p class="muted tip">开启"屏蔽内网"后生成 finalRules，禁止访问 10.x/172.16.x/192.168.x 等私有 IP 段。</p>
      </template>

      <!-- ===== Blackhole 参数 ===== -->
      <template v-if="form.protocol === 'blackhole'">
        <div class="sec-title">Blackhole 黑洞参数</div>
        <el-form-item label="响应类型 response.type">
          <el-select v-model="form.response_type" style="width: 200px">
            <el-option label="none（直接断开）" value="none" />
            <el-option label="http（返回 403）" value="http" />
          </el-select>
        </el-form-item>
      </template>

      <!-- ===== VLESS 参数 ===== -->
      <template v-if="form.protocol === 'vless'">
        <!-- 远端连接 -->
        <div class="sec-title">远端连接</div>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="远端地址 Address">
            <el-input v-model="form.vless_address" placeholder="如 proxy.example.com" />
          </el-form-item>
          <el-form-item label="远端端口 Port">
            <el-input-number v-model="form.vless_port" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
          <el-form-item label="UUID（用户 ID）">
            <el-input v-model="form.vless_uuid" placeholder="xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" />
          </el-form-item>
          <el-form-item label="Flow（流控）">
            <el-select v-model="form.vless_flow" style="width: 100%" clearable>
              <el-option v-for="opt in FLOW_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="Encryption（加密方式）">
            <el-select v-model="form.vless_encryption" style="width: 100%">
              <el-option v-for="opt in ENCRYPTION_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </el-form-item>
        </div>

        <!-- 传输层 -->
        <div class="sec-title">传输层</div>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="传输协议 Network">
            <el-select v-model="form.vless_network" style="width: 100%">
              <el-option v-for="opt in NETWORK_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="安全类型 Security">
            <el-select v-model="form.vless_security" style="width: 100%">
              <el-option v-for="opt in SECURITY_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </el-form-item>
        </div>

        <!-- 传输参数 -->
        <template v-if="form.vless_network === 'ws'">
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="WS Path"><el-input v-model="form.ws_path" placeholder="/" /></el-form-item>
            <el-form-item label="WS Host（选填）"><el-input v-model="form.ws_host" placeholder="留空默认" /></el-form-item>
          </div>
        </template>
        <template v-if="form.vless_network === 'xhttp'">
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="XHTTP Mode">
              <el-select v-model="form.xhttp_mode" style="width: 100%">
                <el-option label="auto" value="auto" />
                <el-option label="packet-up" value="packet-up" />
                <el-option label="stream-up" value="stream-up" />
                <el-option label="stream-one" value="stream-one" />
              </el-select>
            </el-form-item>
            <el-form-item label="XHTTP Path"><el-input v-model="form.xhttp_path" placeholder="/" /></el-form-item>
            <el-form-item label="XHTTP Host（选填）"><el-input v-model="form.xhttp_host" placeholder="留空默认" /></el-form-item>
          </div>
        </template>
        <template v-if="form.vless_network === 'grpc'">
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="gRPC Service Name"><el-input v-model="form.grpc_service_name" placeholder="grpc" /></el-form-item>
            <el-form-item label="gRPC Authority（选填）"><el-input v-model="form.grpc_authority" placeholder="留空默认" /></el-form-item>
            <el-form-item label="Multi Mode"><el-switch v-model="form.grpc_multi_mode" /></el-form-item>
          </div>
        </template>

        <!-- 安全层参数 -->
        <template v-if="form.vless_security === 'tls'">
          <div class="sec-title">TLS 参数</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="SNI (serverName)">
              <el-input v-model="form.tls_server_name" placeholder="如 example.com" />
            </el-form-item>
            <el-form-item label="uTLS Fingerprint">
              <el-select v-model="form.vless_fingerprint" style="width: 100%">
                <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
              </el-select>
            </el-form-item>
            <el-form-item label="allowInsecure（跳过证书校验）">
              <el-switch v-model="form.tls_allow_insecure" />
            </el-form-item>
          </div>
        </template>
        <template v-if="form.vless_security === 'reality'">
          <div class="sec-title">REALITY 参数</div>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
            <el-form-item label="SNI (serverName)">
              <el-input v-model="form.reality_server_name" placeholder="如 www.apple.com" />
            </el-form-item>
            <el-form-item label="Fingerprint">
              <el-select v-model="form.reality_fingerprint" style="width: 100%">
                <el-option v-for="fp in FINGERPRINT_OPTIONS" :key="fp" :label="fp" :value="fp" />
              </el-select>
            </el-form-item>
            <el-form-item label="Short ID">
              <div style="display: flex; gap: 8px; width: 100%">
                <el-input v-model="form.reality_short_id" placeholder="abcdef0123456789" />
                <el-button @click="genShortId"><el-icon><Refresh /></el-icon>&nbsp;随机</el-button>
              </div>
            </el-form-item>
            <el-form-item label="SpiderX 路径">
              <el-input v-model="form.reality_spider_x" placeholder="/" />
            </el-form-item>
            <el-form-item label="Public Key（服务端公钥）">
              <el-input v-model="form.reality_public_key" placeholder="x25519 公钥" />
            </el-form-item>
          </div>
          <div style="margin-top: 8px; display: flex; gap: 10px; align-items: center">
            <el-button type="primary" plain size="small" :loading="genKeyLoading" @click="fetchRealityKeys">
              <el-icon><MagicStick /></el-icon>&nbsp;一键生成 x25519 密钥对
            </el-button>
            <el-button v-if="form.reality_public_key" size="small" text @click="copyText(form.reality_public_key, '公钥')">
              <el-icon><CopyDocument /></el-icon>&nbsp;复制公钥
            </el-button>
          </div>
        </template>
      </template>

      <!-- ===== 通用 ===== -->
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; margin-top: 4px">
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" placeholder="选填" /></el-form-item>
      </div>

      <el-alert type="info" :closable="false" show-icon
        title="保存后将自动重新生成该节点的 Xray 配置并推送（节点离线时保留，上线自动补推）"
        style="margin-top: 10px" />
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">
        <el-icon><Check /></el-icon>&nbsp;保存
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.sec-title {
  font-weight: 600;
  font-size: 13px;
  margin: 6px 0 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--x-border);
  color: var(--x-primary);
}
.muted { color: var(--x-text-3); }
.tip { font-size: 12px; margin: 6px 0 0; }
</style>
