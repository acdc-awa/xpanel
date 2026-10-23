<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  CsAlert,
  CsButton,
  CsContainer,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
  CsSegmentedControl,
  CsSelect,
  CsToggle,
  ElMessage,
} from '@/components/cloudscape'
import CodeEditor from '@/components/CodeEditor.vue'
import {
  createServerOutbound,
  getInbounds,
  getServerOutbounds,
  getServers,
  updateServerOutbound,
  type ServerItem,
  type ServerOutbound,
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

// 出站三种形态：基础出口 / 引用入站 / 自定义 JSON。
// 引用入站只存 inbound_ref（vnext/传输/安全/凭据由生成器按目标入站拼装）；
// 自定义 JSON 只存 outbound_json（原样透传）。旧版「外部代理节点」表单已下线。
type OutboundMode = 'basic' | 'inbound_ref' | 'custom'

const visible = ref(false)
const saving = ref(false)
const mode = ref<OutboundMode>('basic')
const legacyPayload = ref(false)

const isSystemReserved = computed(() => props.outbound?.tag === 'direct' || props.outbound?.tag === 'blocked')

const modeOptions = [
  { id: 'basic', text: '基础出口' },
  { id: 'inbound_ref', text: '引用入站' },
  { id: 'custom', text: '自定义 JSON' },
]

const BASIC_PROTOCOL_OPTIONS = [
  { label: 'freedom（自主直连出口）', value: 'freedom' },
  { label: 'blackhole（黑洞阻断出口）', value: 'blackhole' },
]
const DOMAIN_STRATEGY_OPTIONS = [
  { label: 'AsIs（原样）', value: 'AsIs' },
  { label: 'UseIP（优先解析为 IP）', value: 'UseIP' },
  { label: 'UseIPv4', value: 'UseIPv4' },
  { label: 'UseIPv6', value: 'UseIPv6' },
]
const RESPONSE_TYPE_OPTIONS = [
  { label: 'none（静默丢弃）', value: 'none' },
  { label: 'http（返回 HTTP 403）', value: 'http' },
]

// 只有注册了客户端投影的协议能作为引用目标（当前为 VLESS / Hysteria 2）。
const REF_TARGET_PROTOCOLS = ['vless', 'hysteria']

const servers = ref<ServerItem[]>([])
const allInbounds = ref<InboundItem[]>([])
const refServerId = ref(0)
const refInboundId = ref(0)

const refServerOptions = computed(() => [
  { label: '（稍后在拓扑画布中连线）', value: 0 },
  ...servers.value
    .filter((s) => s.id !== props.serverId)
    .map((s) => ({ label: s.name, value: s.id })),
])

const refInboundOptions = computed(() => {
  if (!refServerId.value) return [{ label: '（请先选择目标服务器）', value: 0 }]
  return [
    { label: '（稍后在拓扑画布中连线）', value: 0 },
    ...allInbounds.value
      .filter(
        (i) =>
          i.server_id === refServerId.value &&
          i.enabled &&
          REF_TARGET_PROTOCOLS.includes(i.protocol) &&
          i.type !== 'tunnel',
      )
      .map((i) => ({ label: inboundLabel(i), value: i.id })),
  ]
})

const refTarget = computed(() => allInbounds.value.find((i) => i.id === refInboundId.value) || null)
const refTargetServer = computed(() => servers.value.find((s) => s.id === refServerId.value) || null)
const refTargetMissing = computed(() => refInboundId.value > 0 && !refTarget.value)

function inboundLabel(i: InboundItem) {
  let net = 'tcp/none'
  try {
    const s = JSON.parse(i.stream_settings || '{}')
    net = `${s.network || 'tcp'}/${s.security || 'none'}`
  } catch {
    /* 解析失败按默认展示 */
  }
  return `${i.tag} :${i.port}（${i.protocol} + ${net}）`
}

const form = reactive({
  tag: '',
  protocol: 'freedom',
  enabled: true,
  priority: 0 as number | null,
  remark: '',
  send_through: '',
  // 基础出口
  domain_strategy: 'AsIs',
  redirect: '',
  block_private: true,
  block_cn: false,
  response_type: 'none',
  // 自定义 JSON
  outbound_json: '',
})

const OUTBOUND_JSON_PLACEHOLDER = `{
  "protocol": "vless",
  "settings": {
    "vnext": [{
      "address": "203.0.113.10",
      "port": 443,
      "users": [{
        "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
        "encryption": "none",
        "flow": "xtls-rprx-vision"
      }]
    }]
  },
  "streamSettings": {
    "network": "tcp",
    "security": "reality",
    "realitySettings": {
      "serverName": "www.example.com",
      "publicKey": "REALITY_PUBLIC_KEY",
      "shortId": "0123abcd",
      "fingerprint": "chrome"
    }
  }
}`

async function loadRefTargets() {
  try {
    const [s, i] = await Promise.all([getServers(), getInbounds()])
    if (s.data.code === 0) servers.value = s.data.data.items
    if (i.data.code === 0) allInbounds.value = i.data.data.items
  } catch {
    /* 目标列表加载失败不阻塞弹窗；保存时后端仍会校验引用 */
  }
}

/**
 * 画布（/admin/topology）只给出站行下发轻量字段，不含 settings_json / outbound_json。
 * 直接拿它回填会让基础出口按默认值展示、自定义 JSON 退化成引用形态，保存即覆盖真实配置；
 * 所以行里没有载荷列时按 id 回查一次完整出站。出站列表页传的已是完整行，不会多一次请求。
 */
const loadedOutbound = ref<ServerOutbound | null>(null)

async function loadFullOutbound() {
  const ob = props.outbound
  if (!ob?.id) return
  // 类型上 settings_json 是必填，这里按运行时值判断「调用方是否真的传了载荷列」
  const raw = ob as unknown as Record<string, unknown>
  if (raw.settings_json !== undefined || raw.outbound_json !== undefined) return
  try {
    const { data } = await getServerOutbounds(props.serverId)
    if (data.code === 0) {
      loadedOutbound.value = data.data.items.find((o) => o.id === ob.id) ?? null
    }
  } catch {
    /* 回查失败退回轻量行：载荷列缺失时保存会被后端形状校验拦下，不会静默覆盖 */
  }
}

function parseExisting() {
  const ob = loadedOutbound.value ?? props.outbound
  if (!ob) return

  form.tag = ob.tag
  form.protocol = ob.protocol
  form.enabled = ob.enabled
  form.priority = ob.priority ?? 0
  form.remark = ob.remark || ''
  form.send_through = ob.send_through || ''

  // 1. 自定义 JSON
  if (ob.outbound_json) {
    mode.value = 'custom'
    form.outbound_json = ob.outbound_json
    return
  }

  // 2. 引用入站
  if (ob.inbound_ref) {
    mode.value = 'inbound_ref'
    refInboundId.value = ob.inbound_ref
    const target = allInbounds.value.find((i) => i.id === ob.inbound_ref)
    if (target) {
      refServerId.value = target.server_id
      form.protocol = target.protocol
    }
    return
  }

  // 3. 基础出口
  if (ob.protocol === 'freedom' || ob.protocol === 'blackhole') {
    mode.value = 'basic'
    try {
      const settings = ob.settings_json ? JSON.parse(ob.settings_json) : {}
      if (form.protocol === 'freedom') {
        form.domain_strategy = settings.domainStrategy || 'AsIs'
        form.block_private =
          Array.isArray(settings.finalRules) &&
          settings.finalRules.some(
            (r: any) => r.action === 'block' && Array.isArray(r.ip) && r.ip.includes('geoip:private'),
          )
        form.block_cn = !!settings.block_cn
        form.redirect = settings.redirect || ''
      } else {
        form.response_type = settings.response?.type || 'none'
      }
    } catch {
      /* 坏数据按默认展示，保存时重建 */
    }
    return
  }

  // 4. 旧版外部代理行（正常应在启动迁移时折叠为 outbound_json；这里只兜底坏数据/跳过项）
  if (ob.settings_json || ob.stream_settings_json) {
    legacyPayload.value = true
    mode.value = 'custom'
    form.outbound_json = composeLegacyJSON(ob)
    return
  }

  // 5. 引用草稿（非基础协议 + 无载荷）
  mode.value = 'inbound_ref'
}

function composeLegacyJSON(ob: ServerOutbound): string {
  const item: Record<string, any> = {}
  try {
    const parsed = JSON.parse(ob.settings_json || '{}')
    if (parsed && typeof parsed === 'object' && ('protocol' in parsed || 'settings' in parsed)) {
      Object.assign(item, parsed)
    } else {
      item.settings = parsed
    }
  } catch {
    /* 坏 JSON 交给用户重写 */
  }
  try {
    if (ob.stream_settings_json) item.streamSettings = JSON.parse(ob.stream_settings_json)
  } catch {
    /* 坏 JSON 交给用户重写 */
  }
  if (!item.protocol) item.protocol = ob.protocol
  return JSON.stringify(item, null, 2)
}

function onModeChange(detail: { selectedId: string | number }) {
  mode.value = String(detail.selectedId) as OutboundMode
  if (mode.value === 'basic' && form.protocol !== 'freedom' && form.protocol !== 'blackhole') {
    form.protocol = 'freedom'
  }
}

function onBasicProtocolChange(v: any) {
  form.protocol = String(v)
}

function onRefServerChange(v: any) {
  refServerId.value = Number(v) || 0
  refInboundId.value = 0
}

function onRefInboundChange(v: any) {
  refInboundId.value = Number(v) || 0
  const target = refTarget.value
  if (target) form.protocol = target.protocol
}

function buildBasicSettingsJSON(): string {
  if (form.protocol === 'blackhole') {
    return JSON.stringify({ response: { type: form.response_type } })
  }
  const s: Record<string, any> = { domainStrategy: form.domain_strategy }
  const redirect = form.redirect.trim()
  if (redirect) s.redirect = redirect
  if (form.block_cn) s.block_cn = true
  const rules: Record<string, any>[] = []
  if (form.block_private) rules.push({ action: 'block', ip: ['geoip:private'], blockDelay: '0' })
  rules.push({ action: 'allow' })
  s.finalRules = rules
  return JSON.stringify(s)
}

function validateCustomJSON(): Record<string, any> | null {
  const raw = form.outbound_json.trim()
  if (!raw) {
    ElMessage.warning('请填写自定义 Outbound JSON')
    return null
  }
  let obj: any
  try {
    obj = JSON.parse(raw)
  } catch (e: any) {
    ElMessage.error(`JSON 语法错误: ${e.message}`)
    return null
  }
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
    ElMessage.warning('Outbound JSON 必须为合法的 JSON 对象 {...}')
    return null
  }
  if (typeof obj.protocol !== 'string' || !obj.protocol.trim()) {
    ElMessage.warning('Outbound JSON 必须包含非空 protocol 字段')
    return null
  }
  return obj
}

async function save() {
  if (!form.tag?.trim()) {
    ElMessage.warning('请填写出站标签')
    return
  }
  const lowerTag = form.tag.trim().toLowerCase()
  if (!props.outbound && (lowerTag === 'direct' || lowerTag === 'blocked')) {
    ElMessage.warning('direct 与 blocked 为系统内置预留出站，请使用其他 Tag')
    return
  }

  if (mode.value === 'custom') {
    if (!validateCustomJSON()) return
  }
  if (mode.value === 'inbound_ref' && refInboundId.value > 0) {
    const target = refTarget.value
    if (!target) {
      ElMessage.warning('目标入站已失效，请重新选择或在拓扑画布中改线')
      return
    }
    if (target.server_id === props.serverId) {
      ElMessage.warning('引用目标必须在其他服务器上（同机会形成转发环路）')
      return
    }
    if (!REF_TARGET_PROTOCOLS.includes(target.protocol)) {
      ElMessage.warning('引用目标仅支持 VLESS / Hysteria 2 入站')
      return
    }
  }

  const payload: any = {
    tag: form.tag.trim(),
    send_through: form.send_through?.trim() || '',
    enabled: form.enabled,
    priority: form.priority ?? 0,
    remark: form.remark || '',
  }

  if (mode.value === 'basic') {
    payload.protocol = form.protocol
    payload.inbound_ref = 0
    payload.outbound_json = ''
    payload.settings_json = buildBasicSettingsJSON()
    payload.stream_settings_json = ''
  } else if (mode.value === 'inbound_ref') {
    payload.protocol = refTarget.value?.protocol || form.protocol || 'vless'
    payload.inbound_ref = refInboundId.value || 0
    payload.outbound_json = ''
    payload.settings_json = ''
    payload.stream_settings_json = ''
  } else {
    payload.protocol = 'custom'
    payload.inbound_ref = 0
    payload.outbound_json = form.outbound_json.trim()
    payload.settings_json = ''
    payload.stream_settings_json = ''
  }

  saving.value = true
  try {
    const { data } = props.outbound
      ? await updateServerOutbound(props.serverId, props.outbound.id, payload)
      : await createServerOutbound(props.serverId, payload)

    if (data.code === 0) {
      ElMessage.success(props.outbound ? '出站已更新' : '出站已创建')
      emit('saved')
      handleClose()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

function handleClose() {
  visible.value = false
  emit('close')
}

onMounted(async () => {
  await Promise.all([loadRefTargets(), loadFullOutbound()])
  parseExisting()
  visible.value = true
})
</script>

<template>
  <CsModal
    :visible="visible"
    :header="outbound ? `编辑出站 · ${outbound.tag}` : '新增出站'"
    size="large"
    scrollable
    @close="handleClose"
  >
    <CsForm variant="embedded" label-position="top">
      <CsAlert v-if="isSystemReserved" type="info" header="系统内置核心出站">
        该出站的 Tag 与模式已锁定，可调整内部策略、优先级与备注。
      </CsAlert>
      <CsAlert v-if="legacyPayload" type="warning" header="旧版外部代理配置" style="margin-top: 12px">
        该行仍是旧版「外部代理节点」的 settings/stream 存储。已尽力预填为 JSON，请检查字段后保存，保存即完成转换。
      </CsAlert>

      <CsContainer header="公共元数据" style="margin-top: 16px">
        <template #actions>
          <div class="meta-toggle">
            <span class="cs-text-secondary">启用此出站</span>
            <CsToggle v-model="form.enabled" aria-label="启用此出站" />
          </div>
        </template>
        <div class="form-grid-2">
          <CsFormItem label="出站标签 Tag" required description="该出站在本服务器上的唯一标识，如 proxy-out / via-japan。">
            <CsInput
              v-model="form.tag"
              :disabled="isSystemReserved"
              class="mono"
              placeholder="如 proxy-out / via-japan"
            />
          </CsFormItem>
          <CsFormItem label="发送 IP (sendThrough)" description="多 IP 服务器可指定出口源地址，留空使用系统默认。">
            <CsInput v-model="form.send_through" class="mono" placeholder="留空使用系统默认 IP" />
          </CsFormItem>
          <CsFormItem label="优先级 Priority" description="数值越小，在 Xray outbounds 数组里越靠前。">
            <CsInputNumber v-model="form.priority" :min="0" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="备注说明">
            <CsInput v-model="form.remark" placeholder="选填，如 默认直连出口 / 日本落地中转" />
          </CsFormItem>
        </div>
      </CsContainer>

      <CsContainer header="出站形态" style="margin-top: 16px">
        <CsSegmentedControl
          :model-value="mode"
          :options="modeOptions"
          :full-width="true"
          @change="onModeChange"
        />
        <p class="mode-hint cs-text-secondary">
          基础出口直接生成 freedom / blackhole；引用入站由目标入站自动拼装；自定义 JSON 原样交给 Xray。
        </p>
      </CsContainer>

      <!-- 基础出口 -->
      <CsContainer v-if="mode === 'basic'" header="基础出口策略" style="margin-top: 16px">
        <div class="form-grid-2">
          <CsFormItem label="出口协议">
            <CsSelect
              :model-value="form.protocol"
              :options="BASIC_PROTOCOL_OPTIONS"
              :disabled="isSystemReserved"
              @update:model-value="onBasicProtocolChange"
            />
          </CsFormItem>
        </div>

        <template v-if="form.protocol === 'freedom'">
          <div class="form-grid-2">
            <CsFormItem label="域名解析策略 domainStrategy">
              <CsSelect
                :model-value="form.domain_strategy"
                :options="DOMAIN_STRATEGY_OPTIONS"
                @update:model-value="(v: any) => (form.domain_strategy = String(v))"
              />
            </CsFormItem>
            <CsFormItem label="redirect 目标" description="把流量重定向到指定地址（host:port）；留空表示不重定向。">
              <CsInput v-model="form.redirect" class="mono" placeholder="留空 = 不重定向" />
            </CsFormItem>
          </div>
          <div class="switch-row">
            <CsToggle v-model="form.block_private" label="屏蔽内网 / 保留网段（geoip:private）" />
            <CsToggle v-model="form.block_cn" label="阻断回国（geoip:cn）" />
          </div>
        </template>

        <template v-else>
          <CsFormItem label="黑洞响应方式" description="none 静默丢弃；http 返回 403，便于客户端快速失败。">
            <CsSelect
              :model-value="form.response_type"
              :options="RESPONSE_TYPE_OPTIONS"
              @update:model-value="(v: any) => (form.response_type = String(v))"
            />
          </CsFormItem>
        </template>
      </CsContainer>

      <!-- 引用入站 -->
      <CsContainer v-if="mode === 'inbound_ref'" header="引用落地入站" style="margin-top: 16px">
        <div class="form-grid-2">
          <CsFormItem label="目标服务器" description="引用必须跨服务器；同机会形成转发环路。">
            <CsSelect
              :model-value="refServerId"
              :options="refServerOptions"
              filterable
              @update:model-value="onRefServerChange"
            />
          </CsFormItem>
          <CsFormItem label="目标入站" description="仅列出已启用、支持客户端投影的 VLESS / Hysteria 2 入站。">
            <CsSelect
              :model-value="refInboundId"
              :options="refInboundOptions"
              filterable
              :disabled="!refServerId"
              @update:model-value="onRefInboundChange"
            />
          </CsFormItem>
        </div>

        <CsAlert
          :type="refTarget ? 'success' : refTargetMissing ? 'warning' : 'info'"
          :header="refTarget ? '已连线' : refTargetMissing ? '目标入站不可用' : '待连线'"
        >
          <template v-if="refTarget">
            已引用「{{ refTargetServer?.name || '服务器#' + refTarget.server_id }} / {{ refTarget.tag }}」。vnext
            地址、端口、UUID、传输与安全参数由主控按目标入站自动拼装，机间凭据自动注入落地入站，无需手填。
          </template>
          <template v-else-if="refTargetMissing">
            当前引用的入站已不在可引用列表（可能被停用/删除，或不是 VLESS / Hysteria 2）。请重新选择目标。
          </template>
          <template v-else>
            留空保存后，到「路由 → 拓扑画布」把本出站拖到目标服务器的入站即可；也可以直接在上方选择目标。
          </template>
        </CsAlert>
      </CsContainer>

      <!-- 自定义 JSON -->
      <CsContainer v-if="mode === 'custom'" header="自定义 Xray 出站 JSON" style="margin-top: 16px">
        <CodeEditor
          v-model="form.outbound_json"
          language="json"
          height="320px"
          :placeholder="OUTBOUND_JSON_PLACEHOLDER"
        />
        <CsAlert type="info" header="原样透传" style="margin-top: 12px">
          面板只覆盖出站 <code>tag</code> 与 <code>sendThrough</code>，并为 VLESS 缺省补
          <code>encryption: "none"</code>，其余字段不做改写。字段语法与版本兼容性由 Xray 校验兜底；
          请按当前内核（26.10.21）的字段名填写。
        </CsAlert>
      </CsContainer>
    </CsForm>

    <template #footer>
      <div class="modal-footer-row">
        <CsButton variant="link" @click="handleClose">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">保存</CsButton>
      </div>
    </template>
  </CsModal>
</template>

<style scoped>
.form-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.meta-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
}
.switch-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 28px;
  margin-top: 4px;
}
.mode-hint {
  margin: 10px 0 0;
  font-size: 13px;
}
.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-variant-numeric: tabular-nums;
}
.modal-footer-row {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
:deep(.cs-form-item) {
  margin-bottom: 16px;
}
@media (max-width: 720px) {
  .form-grid-2 {
    grid-template-columns: 1fr;
  }
}
</style>
