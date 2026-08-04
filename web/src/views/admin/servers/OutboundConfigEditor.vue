<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { Check } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  createServerOutbound,
  updateServerOutbound,
  type OutboundPayload,
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

const form = reactive<OutboundPayload>({
  tag: '',
  protocol: 'freedom',
  settings_json: '',
  stream_settings_json: '',
  send_through: '',
  enabled: true,
  priority: 0,
  remark: '',
})

const jsonError = ref('')

const PROTOCOL_OPTIONS = [
  { label: 'freedom（直连）', value: 'freedom' },
  { label: 'blackhole（黑洞）', value: 'blackhole' },
  { label: 'socks（Socks 代理）', value: 'socks' },
  { label: 'vmess（Vmess 代理）', value: 'vmess' },
  { label: 'vless（Vless 代理）', value: 'vless' },
]

const PROTOCOL_TEMPLATES: Record<string, string> = {
  freedom: JSON.stringify({ domainStrategy: 'UseIP' }, null, 2),
  blackhole: JSON.stringify({ response: { type: 'none' } }, null, 2),
  socks: JSON.stringify(
    {
      servers: [
        { address: '127.0.0.1', port: 1080, users: [{ user: 'user', pass: 'pass' }] },
      ],
    },
    null,
    2,
  ),
  vmess: JSON.stringify(
    {
      vnext: [
        {
          address: 'example.com',
          port: 443,
          users: [{ id: 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx', alterId: 0, email: 'user@example.com', security: 'auto' }],
        },
      ],
    },
    null,
    2,
  ),
  vless: JSON.stringify(
    {
      vnext: [
        {
          address: 'example.com',
          port: 443,
          users: [
            {
              id: 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx',
              flow: 'xtls-rprx-vision',
              encryption: 'none',
              level: 0,
              email: 'user@example.com',
            },
          ],
        },
      ],
    },
    null,
    2,
  ),
}

const DEFAULT_STREAM_TEMPLATE = JSON.stringify(
  {
    network: 'tcp',
    security: 'none',
    sockopt: { tcpFastOpen: true },
  },
  null,
  2,
)

watch(
  () => form.protocol,
  (p) => {
    if (props.outbound) return
    if (PROTOCOL_TEMPLATES[p]) {
      form.settings_json = PROTOCOL_TEMPLATES[p]
    }
  },
)

function validateJson(text: string, label: string): boolean {
  if (!text || !text.trim()) return true
  try {
    JSON.parse(text)
    return true
  } catch {
    jsonError.value = `${label}不是合法 JSON`
    return false
  }
}

onMounted(() => {
  if (props.outbound) {
    form.tag = props.outbound.tag
    form.protocol = props.outbound.protocol
    form.settings_json = props.outbound.settings_json || ''
    form.stream_settings_json = props.outbound.stream_settings_json || ''
    form.send_through = props.outbound.send_through || ''
    form.enabled = props.outbound.enabled
    form.priority = props.outbound.priority
    form.remark = props.outbound.remark
  } else {
    form.tag = ''
    form.protocol = 'freedom'
    form.settings_json = PROTOCOL_TEMPLATES.freedom
  }
  visible.value = true
})

async function save() {
  jsonError.value = ''
  if (!form.tag?.trim()) {
    ElMessage.warning('请填写标签（tag）')
    return
  }
  if (!form.protocol) {
    ElMessage.warning('请选择协议')
    return
  }
  if (!validateJson(form.settings_json ?? '', 'Settings JSON')) return
  if (!validateJson(form.stream_settings_json ?? '', 'Stream Settings JSON')) return

  saving.value = true
  try {
    const payload: OutboundPayload = {
      tag: form.tag.trim(),
      protocol: form.protocol,
      settings_json: form.settings_json?.trim() || '',
      stream_settings_json: form.stream_settings_json?.trim() || '',
      send_through: form.send_through?.trim() || '',
      enabled: form.enabled ?? true,
      priority: form.priority ?? 0,
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

function onClosed() {
  emit('close')
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="outbound ? '编辑出站规则' : '新增出站规则'"
    width="720px"
    @closed="onClosed"
  >
    <el-form label-position="top">
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="协议 Protocol">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option v-for="opt in PROTOCOL_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="标签 Tag">
          <el-input v-model="form.tag" placeholder="如 direct / proxy-socks / blocked" />
        </el-form-item>
        <el-form-item label="发送 IP（sendThrough，选填）">
          <el-input v-model="form.send_through" placeholder="如 1.2.3.4" />
        </el-form-item>
        <el-form-item label="优先级 Priority">
          <el-input-number v-model="form.priority" :min="0" style="width: 100%" />
        </el-form-item>
      </div>

      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="选填" />
        </el-form-item>
      </div>

      <div class="sec-title">Settings JSON（协议参数，切换协议时自动填充模板）</div>
      <el-input
        v-model="form.settings_json"
        type="textarea"
        :rows="8"
        class="code-textarea"
        placeholder='{"domainStrategy": "UseIP"}'
      />
      <p class="muted tip">
        freedom 如 <code>{"domainStrategy":"UseIP"}</code>；blackhole 如
        <code>{"response":{"type":"none"}}</code>；socks 如
        <code>{"servers":[{"address":"127.0.0.1","port":1080,"users":[]}]}</code>
      </p>

      <div class="sec-title">Stream Settings JSON（传输层，选填）</div>
      <el-input
        v-model="form.stream_settings_json"
        type="textarea"
        :rows="6"
        class="code-textarea"
        placeholder='{"network":"tcp","security":"none","sockopt":{"tcpFastOpen":true}}'
      />

      <el-alert
        v-if="jsonError"
        type="error"
        :closable="false"
        :title="jsonError"
        style="margin-top: 10px"
      />
      <el-alert
        v-else
        type="info"
        :closable="false"
        show-icon
        title="保存后将自动重新生成该节点的 Xray 配置并推送（节点离线时保留，上线自动补推）"
        style="margin-top: 10px"
      />
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
  padding-top: 6px;
  border-top: 1px dashed var(--x-border);
  color: var(--x-primary);
}
.muted {
  color: var(--x-text-3);
}
.tip {
  font-size: 12px;
  margin: 6px 0 0;
}
.code-textarea {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.5;
}
</style>
